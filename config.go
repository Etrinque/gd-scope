package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is loaded from mcp.json at startup.
// All fields have sensible defaults so minimal config is needed.
type Config struct {
	// ProjectRoot is the absolute path to the Godot project.
	// Defaults to the current working directory.
	ProjectRoot string `json:"project_root"`

	// DocsDir is where versioned Godot documentation lives (docs/4.3/, docs/4.2/, …).
	// Defaults to "./docs" relative to the binary.
	DocsDir string `json:"docs_dir"`

	// ToolsDir is where external tool JSON configs are stored.
	// Defaults to "./tools".
	ToolsDir string `json:"tools_dir"`

	// OllamaURL is the base URL for the Ollama API.
	// Leave empty to disable semantic search entirely (no Ollama dependency).
	OllamaURL string `json:"ollama_url"`

	// EmbedModel is the Ollama model used for embeddings.
	// Defaults to "nomic-embed-text".
	EmbedModel string `json:"embed_model"`

	// Addr is the HTTP listen address when not using stdio transport.
	// Defaults to ":3333".
	Addr string `json:"addr"`

	// ExternalTimeout is the max seconds an external tool process may run.
	// Defaults to 30.
	ExternalTimeout int `json:"external_timeout_seconds"`

	// GodotBin is the path to the Godot executable used for gdscript tools.
	// Defaults to "godot" (assumes it is on PATH).
	// Override if your Godot binary is not on PATH, e.g. "/opt/godot/godot4".
	GodotBin string `json:"godot_bin"`

	// DefaultModel is the Ollama model to use when client doesn't specify one.
	// Only used for Ollama-compatible endpoints (/api/chat).
	// Defaults to "qwen2.5-coder:7b".
	DefaultModel string `json:"default_model"`

	// RAG configures Retrieval Augmented Generation
	RAG RAGConfig `json:"rag"`
}

// RAGConfig controls automatic context retrieval
type RAGConfig struct {
	// Enabled turns on automatic RAG for relevant queries
	Enabled bool `json:"enabled"`

	// TopK is the number of code chunks to retrieve
	TopK int `json:"top_k"`

	// AutoDetect automatically determines if query needs RAG
	// If false, RAG is used for all queries when enabled
	AutoDetect bool `json:"auto_detect"`
}

// LoadConfig reads mcp.json and fills defaults for any missing fields.
func LoadConfig(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file is fine; use all defaults.
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Re-apply defaults for anything left as zero value after parsing.
	if cfg.DocsDir == "" {
		cfg.DocsDir = "docs"
	}
	if cfg.ToolsDir == "" {
		cfg.ToolsDir = "tools"
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = "nomic-embed-text"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":3333"
	}
	if cfg.ExternalTimeout == 0 {
		cfg.ExternalTimeout = 30
	}

	// Resolve project root to absolute path now so everything downstream is consistent.
	if cfg.ProjectRoot == "" {
		cfg.ProjectRoot, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
	} else {
		cfg.ProjectRoot, err = filepath.Abs(cfg.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("abs project_root: %w", err)
		}
	}

	return cfg, nil
}

func defaultConfig() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		ProjectRoot:     cwd,
		DocsDir:         "docs",
		ToolsDir:        "tools",
		EmbedModel:      "nomic-embed-text",
		Addr:            ":3333",
		ExternalTimeout: 30,
		DefaultModel:    "qwen2.5-coder:7b",
		RAG: RAGConfig{
			Enabled:    false, // Opt-in for now
			TopK:       3,
			AutoDetect: true,
		},
		// OllamaURL intentionally empty - disables semantic search by default.
	}
}
