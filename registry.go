package main

import (
	"context"
	"encoding/json"

	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolType distinguishes built-in Go handlers from external process tools.
type ToolType string

const (
	ToolBuiltin  ToolType = "builtin"
	ToolExternal ToolType = "external"
	ToolGDScript ToolType = "gdscript"
)

// ToolConfig is the schema for a JSON file inside the tools/ directory.
// Built-in tools are pre-wired in Go; external tools run an executable.
type ToolConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        ToolType `json:"type"`
	// Handler is only used for builtin tools and maps to a key in Registry.handlers.
	Handler string `json:"handler,omitempty"`
	// Command is only used for external tools. It is the path to the executable.
	Command string `json:"command,omitempty"`
	// Script is only used for gdscript tools. It is the path to the .gd file.
	Script string `json:"script,omitempty"`
	// GodotBin overrides the godot binary path for this specific tool.
	// Falls back to the config-level godot_bin if unset.
	GodotBin string `json:"godot_bin,omitempty"`
}

// ToolHandler is the function signature for all registered built-in tools.
type ToolHandler func(context.Context, map[string]any) (any, error)

// VectorEntry is one indexed document for semantic search.
type VectorEntry struct {
	Key       string
	Text      string
	Embedding []float64
}

// Registry holds all tool configurations and their handlers.
// It is safe for concurrent use.
type Registry struct {
	cfg      *Config
	mu       sync.RWMutex
	external map[string]ToolConfig // external tools loaded from JSON
	vectors  []VectorEntry
	handlers map[string]ToolHandler // built-in name → function
}

// NewRegistry constructs a Registry and wires up all built-in tool handlers.
func NewRegistry(cfg *Config) (*Registry, error) {
	r := &Registry{
		cfg:      cfg,
		external: map[string]ToolConfig{},
	}

	// Wire built-in handlers. These are always available regardless of JSON configs.
	r.handlers = map[string]ToolHandler{
		// Filesystem
		"read_file":  r.readFile,
		"list_files": r.listFiles,
		// Godot
		"project_info": r.projectInfo,
		"list_scenes":  r.listScenes,
		"read_scene":   r.readScene,
		"list_scripts": r.listScripts,
		// Docs
		"docs_versions": r.docsVersions,
		"docs_list":     r.docsList,
		"docs_get":      r.docsGet,
		"docs_search":   r.docsSearch,
		// Semantic search (handlers always registered; availability gated on cfg.OllamaURL)
		"index_project":   r.indexProject,
		"semantic_search": r.semanticSearch,
	}

	return r, nil
}

// Load reads all JSON files from the tools/ directory and stores external tool configs.
// It replaces the previous external tool set and clears stale vector entries for
// tools that no longer exist.
// Load is safe to call at runtime for hot-reload.
func (r *Registry) Load() error {
	entries, err := os.ReadDir(r.cfg.ToolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// tools/ dir missing is non-fatal; there may just be no external tools.
			return nil
		}
		return fmt.Errorf("read tools dir: %w", err)
	}

	loaded := map[string]ToolConfig{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		fpath := filepath.Join(r.cfg.ToolsDir, e.Name())
		data, err := os.ReadFile(fpath)
		if err != nil {
			log.Printf("warn: skip %s: %v", fpath, err)
			continue
		}
		var cfg ToolConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("warn: parse %s: %v", fpath, err)
			continue
		}
		if cfg.Name == "" {
			log.Printf("warn: %s has no name, skipping", fpath)
			continue
		}
		if cfg.Type == ToolExternal && cfg.Command == "" {
			log.Printf("warn: %s is external but has no command, skipping", fpath)
			continue
		}
		if cfg.Type == ToolGDScript && cfg.Script == "" {
			log.Printf("warn: %s is gdscript but has no script path, skipping", fpath)
			continue
		}
		loaded[cfg.Name] = cfg
	}

	r.mu.Lock()
	r.external = loaded
	// Purge vector entries for tools that no longer exist.
	kept := r.vectors[:0]
	for _, v := range r.vectors {
		if _, ok := loaded[v.Key]; ok {
			kept = append(kept, v)
		}
	}
	r.vectors = kept
	r.mu.Unlock()

	log.Printf("registry: loaded %d external tool(s)", len(loaded))
	return nil
}

// Invoke dispatches a tool call by name.
// Built-in tools take priority over external ones with the same name.
func (r *Registry) Invoke(ctx context.Context, name string, args map[string]any) (any, error) {
	if h, ok := r.handlers[name]; ok {
		return h(ctx, args)
	}

	r.mu.RLock()
	cfg, ok := r.external[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown tool: %q", name)
	}

	switch cfg.Type {
	case ToolExternal:
		return runExternal(ctx, cfg.Command, args, r.cfg.ExternalTimeout)
	case ToolGDScript:
		godotBin := cfg.GodotBin
		if godotBin == "" {
			godotBin = r.cfg.GodotBin
		}
		if godotBin == "" {
			godotBin = "godot" // assume it is on PATH
		}
		return runGDScript(ctx, godotBin, cfg.Script, args, r.cfg.ExternalTimeout)
	default:
		return nil, fmt.Errorf("unsupported tool type: %q", cfg.Type)
	}
}

// RegisterExternalMCPTools adds MCP tool entries for every external tool currently loaded.
// Call this once at startup after Load().
func (r *Registry) RegisterExternalMCPTools(srv *mcp.Server) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, cfg := range r.external {
		// Capture loop vars.
		n := name
		desc := cfg.Description
		if desc == "" {
			desc = fmt.Sprintf("External tool: %s", n)
		}
		mcp.AddTool[any, any](srv, &mcp.Tool{Name: n, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
				// Convert input to map[string]any
				var params map[string]any
				if input != nil {
					params, _ = input.(map[string]any)
				}
				if params == nil {
					params = map[string]any{}
				}

				result, err := r.Invoke(ctx, n, params)
				return nil, result, err
			},
		)
	}
}
