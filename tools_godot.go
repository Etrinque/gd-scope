package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ──────────────────────────────────────────────────────────
// project_info
// ──────────────────────────────────────────────────────────

// projectInfo parses project.godot using a simple INI-aware scanner.
// It does not depend on any INI library - the format is straightforward enough.
func (r *Registry) projectInfo(_ context.Context, _ map[string]any) (any, error) {
	ppath := filepath.Join(r.cfg.ProjectRoot, "project.godot")
	f, err := os.Open(ppath)
	if err != nil {
		return nil, fmt.Errorf("project_info: cannot open project.godot: %w", err)
	}
	defer f.Close()

	result := map[string]any{
		"path":      ppath,
		"autoloads": map[string]string{},
		"features":  []string{},
		"settings":  map[string]string{},
	}

	autoloads := map[string]string{}
	features := []string{}
	settings := map[string]string{}
	section := ""

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		// Key=value
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes from string values.
		val = strings.Trim(val, `"`)

		switch section {
		case "application":
			switch key {
			case "config/name":
				result["name"] = val
			case "config/description":
				result["description"] = val
			case "config/version":
				result["version"] = val
			}
			settings[section+"/"+key] = val
		case "":
			// Top-level keys in project.godot
			if key == "config_version" {
				result["config_version"] = val
			}
		default:
			if strings.HasPrefix(section, "autoload") {
				// Autoload entries look like: [autoload] / MyNode="*res://autoload/MyNode.gd"
				autoloads[key] = strings.TrimPrefix(val, "*")
			} else {
				settings[section+"/"+key] = val
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("project_info: scan error: %w", err)
	}

	// Extract Godot engine version from the config_version hint or feature tags.
	// project.godot files >= Godot 4 have config_version=5.
	if cv, ok := result["config_version"].(string); ok {
		switch cv {
		case "5":
			result["engine_generation"] = "Godot 4.x"
		case "4":
			result["engine_generation"] = "Godot 3.x"
		default:
			result["engine_generation"] = "unknown"
		}
	}

	result["autoloads"] = autoloads
	result["features"] = features
	result["settings"] = settings
	return result, nil
}

// ──────────────────────────────────────────────────────────
// list_scenes
// ──────────────────────────────────────────────────────────

func (r *Registry) listScenes(_ context.Context, a map[string]any) (any, error) {
	raw, _ := a["root"].(string)
	root, err := r.securePath(raw)
	if err != nil {
		return nil, err
	}

	var scenes []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".tscn") {
			rel, _ := filepath.Rel(r.cfg.ProjectRoot, p)
			scenes = append(scenes, rel)
		}
		return nil
	})

	return map[string]any{"scenes": scenes, "count": len(scenes)}, nil
}

// ──────────────────────────────────────────────────────────
// read_scene
// ──────────────────────────────────────────────────────────

// SceneNode represents one [node] entry in a .tscn file.
type SceneNode struct {
	Name     string            `json:"name"`
	Type     string            `json:"type,omitempty"`
	Parent   string            `json:"parent,omitempty"`
	Instance string            `json:"instance,omitempty"` // for instanced scenes
	Props    map[string]string `json:"props,omitempty"`
}

// ExtResource is a reference to an external file inside a .tscn.
type ExtResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Path string `json:"path"`
}

var (
	reHeader      = regexp.MustCompile(`^\[(\w+)([^\]]*)\]$`)
	reAttr        = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
	reAttrUnquote = regexp.MustCompile(`(\w+)\s*=\s*(.+)`)
)

func (r *Registry) readScene(_ context.Context, a map[string]any) (any, error) {
	raw, _ := a["path"].(string)
	p, err := r.securePath(raw)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("read_scene: %w", err)
	}
	defer f.Close()

	var nodes []SceneNode
	var resources []ExtResource
	var gd_scene map[string]string

	var currentNode *SceneNode
	var currentSection string
	var currentAttrs map[string]string

	flush := func() {
		if currentSection == "node" && currentNode != nil {
			currentNode.Props = currentAttrs
			nodes = append(nodes, *currentNode)
			currentNode = nil
		}
		if currentSection == "ext_resource" && currentAttrs != nil {
			resources = append(resources, ExtResource{
				ID:   currentAttrs["id"],
				Type: currentAttrs["type"],
				Path: currentAttrs["path"],
			})
		}
		currentAttrs = nil
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if m := reHeader.FindStringSubmatch(line); m != nil {
			flush()
			currentSection = m[1]
			currentAttrs = map[string]string{}

			// Parse inline header attributes: [node name="Foo" type="Node2D" parent="."]
			for _, am := range reAttr.FindAllStringSubmatch(m[2], -1) {
				currentAttrs[am[1]] = am[2]
			}

			switch currentSection {
			case "node":
				currentNode = &SceneNode{
					Name:   currentAttrs["name"],
					Type:   currentAttrs["type"],
					Parent: currentAttrs["parent"],
				}
				if inst, ok := currentAttrs["instance"]; ok {
					currentNode.Instance = inst
				}
				currentAttrs = map[string]string{}
			case "gd_scene":
				gd_scene = currentAttrs
				currentSection = "" // nothing more to collect
			}
			continue
		}

		// Property line inside a section
		if currentAttrs != nil {
			for _, am := range reAttrUnquote.FindAllStringSubmatch(line, 1) {
				val := strings.Trim(am[2], `"`)
				currentAttrs[am[1]] = val
			}
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read_scene scan: %w", err)
	}

	return map[string]any{
		"path":          raw,
		"gd_scene":      gd_scene,
		"nodes":         nodes,
		"node_count":    len(nodes),
		"ext_resources": resources,
	}, nil
}

// ──────────────────────────────────────────────────────────
// list_scripts
// ──────────────────────────────────────────────────────────

func (r *Registry) listScripts(_ context.Context, a map[string]any) (any, error) {
	raw, _ := a["root"].(string)
	root, err := r.securePath(raw)
	if err != nil {
		return nil, err
	}

	var scripts []map[string]string
	filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".gd" || ext == ".cs" {
			rel, _ := filepath.Rel(r.cfg.ProjectRoot, p)
			scripts = append(scripts, map[string]string{
				"path": rel,
				"lang": langFromExt(ext),
			})
		}
		return nil
	})

	return map[string]any{"scripts": scripts, "count": len(scripts)}, nil
}

func langFromExt(ext string) string {
	switch ext {
	case ".gd":
		return "GDScript"
	case ".cs":
		return "C#"
	default:
		return "unknown"
	}
}
