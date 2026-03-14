package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// securePath resolves a user-supplied path against the configured project root
// and rejects anything that would escape the sandbox via traversal.
func (r *Registry) securePath(raw string) (string, error) {
	if raw == "" {
		raw = "."
	}

	// Resolve against the project root, not the process cwd.
	var abs string
	if filepath.IsAbs(raw) {
		abs = filepath.Clean(raw)
	} else {
		abs = filepath.Join(r.cfg.ProjectRoot, raw)
		abs = filepath.Clean(abs)
	}

	// The sandbox boundary is the project root with a trailing separator so that
	// a root of "/project" does not accidentally allow "/projectevil".
	boundary := r.cfg.ProjectRoot
	if !strings.HasSuffix(boundary, string(os.PathSeparator)) {
		boundary += string(os.PathSeparator)
	}

	if abs != r.cfg.ProjectRoot && !strings.HasPrefix(abs, boundary) {
		return "", fmt.Errorf("path %q is outside the project sandbox", raw)
	}

	return abs, nil
}

// readFile returns the text content of a file.
// Args:
//
//	path  (string) – project-relative or absolute (must be inside project root)
func (r *Registry) readFile(_ context.Context, a map[string]any) (any, error) {
	raw, _ := a["path"].(string)
	p, err := r.securePath(raw)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read_file: %w", err)
	}
	return map[string]any{
		"path":    raw,
		"content": string(b),
	}, nil
}

// listFiles recursively enumerates files under root.
// Args:
//
//	root  (string, optional) – project-relative directory, defaults to project root
func (r *Registry) listFiles(_ context.Context, a map[string]any) (any, error) {
	raw, _ := a["root"].(string)
	root, err := r.securePath(raw)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Log the permission error but continue walking the rest of the tree.
			return nil
		}
		if d == nil {
			return nil
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(r.cfg.ProjectRoot, p)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list_files: %w", err)
	}

	return map[string]any{
		"root":  raw,
		"files": files,
		"count": len(files),
	}, nil
}
