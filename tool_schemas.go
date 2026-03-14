package main

import "fmt"

// Tool descriptions for Ollama function calling
// These are used to help the LLM understand when and how to use each tool

func getToolDescription(name string) string {
	descriptions := map[string]string{
		"read_file":       "Read the contents of a file from the Godot project. Use this when you need to see the actual code or content of a specific file.",
		"list_files":      "List all files in a directory recursively. Use this to explore the project structure or find files.",
		"project_info":    "Get information about the Godot project including version, autoloads, and settings. Use this to understand the project configuration.",
		"list_scenes":     "List all .tscn scene files in the project. Use this to see what scenes exist.",
		"read_scene":      "Parse a .tscn scene file and return its node hierarchy, properties, and ext_resource references. Use this to understand scene structure.",
		"list_scripts":    "List all GDScript (.gd) and C# (.cs) files in the project. Use this to find scripts.",
		"docs_versions":   "List available Godot documentation versions. Use this before searching documentation.",
		"docs_list":       "List all documentation pages available for a specific version. Use this to explore available documentation.",
		"docs_get":        "Get the full content of a specific documentation page. Use this to read Godot API documentation.",
		"docs_search":     "Search Godot documentation by keyword. Use this to find relevant documentation pages.",
		"index_project":   "Index all project files for semantic search. Call this once before using semantic_search. Requires Ollama embedding model.",
		"semantic_search": "Search project files using natural language. Use this to find code by describing what it does rather than exact keywords.",
		"reload_tools":    "Reload external tool configurations from the tools/ directory. Use this after adding new tools.",
	}

	if desc, ok := descriptions[name]; ok {
		return desc
	}
	return fmt.Sprintf("Tool: %s", name)
}

func getToolParameters(name string) map[string]any {
	schemas := map[string]map[string]any{
		"read_file": {
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file (project-relative or absolute)",
			},
		},
		"list_files": {
			"root": map[string]any{
				"type":        "string",
				"description": "Root directory to list files from (optional, defaults to project root)",
			},
		},
		"list_scenes": {
			"root": map[string]any{
				"type":        "string",
				"description": "Root directory to search for scenes (optional, defaults to project root)",
			},
		},
		"read_scene": {
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the .tscn file to parse",
			},
		},
		"list_scripts": {
			"root": map[string]any{
				"type":        "string",
				"description": "Root directory to search for scripts (optional, defaults to project root)",
			},
		},
		"docs_list": {
			"version": map[string]any{
				"type":        "string",
				"description": "Documentation version (e.g. '4.3', '4.2')",
			},
		},
		"docs_get": {
			"version": map[string]any{
				"type":        "string",
				"description": "Documentation version (e.g. '4.3')",
			},
			"page": map[string]any{
				"type":        "string",
				"description": "Page name without .md extension",
			},
		},
		"docs_search": {
			"version": map[string]any{
				"type":        "string",
				"description": "Documentation version or 'all' to search all versions",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query string",
			},
		},
		"semantic_search": {
			"query": map[string]any{
				"type":        "string",
				"description": "Natural language search query describing what code you're looking for",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (default: 5)",
			},
		},
	}

	if params, ok := schemas[name]; ok {
		return params
	}
	return map[string]any{}
}

func getToolRequired(name string) []string {
	required := map[string][]string{
		"read_file":       {"path"},
		"read_scene":      {"path"},
		"docs_list":       {"version"},
		"docs_get":        {"version", "page"},
		"docs_search":     {"version", "query"},
		"semantic_search": {"query"},
	}

	if req, ok := required[name]; ok {
		return req
	}
	return []string{}
}
