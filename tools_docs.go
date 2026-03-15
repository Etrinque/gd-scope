package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// docsDir returns the absolute path to the docs directory.
func (r *Registry) docsDir() string {
	if filepath.IsAbs(r.cfg.DocsDir) {
		return r.cfg.DocsDir
	}
	return filepath.Join(r.cfg.ProjectRoot, r.cfg.DocsDir)
}

// docsVersions lists all subdirectories of docs/ that look like version folders.
// Each folder name is treated as a version string (e.g. "4.3", "4.2").
func (r *Registry) docsVersions(_ context.Context, _ map[string]any) (any, error) {
	entries, err := os.ReadDir(r.docsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"versions": []string{}, "docs_dir": r.docsDir()}, nil
		}
		return nil, fmt.Errorf("docs_versions: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	return map[string]any{
		"versions": versions,
		"docs_dir": r.docsDir(),
	}, nil
}

// docsList lists all .md files inside docs/{version}/.
func (r *Registry) docsList(_ context.Context, a map[string]any) (any, error) {
	version, err := requireString(a, "version")
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(r.docsDir(), version)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("docs_list: cannot read docs/%s: %w", version, err)
	}

	var pages []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			pages = append(pages, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	return map[string]any{"version": version, "pages": pages, "count": len(pages)}, nil
}

// docsGet returns the raw markdown content of one documentation page.
func (r *Registry) docsGet(_ context.Context, a map[string]any) (any, error) {
	version, err := requireString(a, "version")
	if err != nil {
		return nil, err
	}
	page, err := requireString(a, "page")
	if err != nil {
		return nil, err
	}

	// Sanitise page name — no path traversal via the page field.
	// Also strip any .md extension the model may have appended: docs_list
	// returns bare names (e.g. "node_tree") but the model sometimes echoes
	// them back with the extension ("node_tree.md"), which would produce
	// a double-suffixed path like "node_tree.md.md".
	page = filepath.Base(page)
	page = strings.TrimSuffix(page, ".md")
	fpath := filepath.Join(r.docsDir(), version, page+".md")

	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("docs_get: %w", err)
	}
	return map[string]any{
		"version": version,
		"page":    page,
		"content": string(data),
	}, nil
}

// docsSearch does a simple case-insensitive full-text search across docs.
// Args:
//
//	version  – specific version string, or "all" to search every version
//	query    – search string
func (r *Registry) docsSearch(_ context.Context, a map[string]any) (any, error) {
	version, _ := a["version"].(string)
	query, err := requireString(a, "query")
	if err != nil {
		return nil, err
	}
	queryLower := strings.ToLower(query)

	type Hit struct {
		Version string `json:"version"`
		Page    string `json:"page"`
		Lines   []string `json:"matching_lines"`
	}

	var versions []string
	if version == "" || version == "all" {
		entries, readErr := os.ReadDir(r.docsDir())
		if readErr != nil {
			return nil, fmt.Errorf("docs_search: list versions: %w", readErr)
		}
		for _, e := range entries {
			if e.IsDir() {
				versions = append(versions, e.Name())
			}
		}
	} else {
		versions = []string{version}
	}

	var hits []Hit
	for _, v := range versions {
		dir := filepath.Join(r.docsDir(), v)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var matchLines []string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(strings.ToLower(line), queryLower) {
					matchLines = append(matchLines, strings.TrimSpace(line))
				}
			}
			if len(matchLines) > 0 {
				hits = append(hits, Hit{
					Version: v,
					Page:    strings.TrimSuffix(e.Name(), ".md"),
					Lines:   matchLines,
				})
			}
		}
	}

	return map[string]any{
		"query":   query,
		"version": version,
		"hits":    hits,
		"count":   len(hits),
	}, nil
}

// requireString is a helper that extracts a required string arg with a clear error.
func requireString(a map[string]any, key string) (string, error) {
	v, ok := a[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required argument: %q", key)
	}
	return v, nil
}
