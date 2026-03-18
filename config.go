package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
)

const configFileName = "mcp.json"

// Config is loaded from mcp.json at startup.
// All fields have sensible defaults so minimal config is needed.
type Config struct {
	// ProjectRoot is the absolute path to the Godot project.
	// Defaults to the current working directory.
	// TODO: REMOVE CWD DEFAULT. walk tree up until project.godot is found...
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
	// TODO: On start, grep for nomic-embed-text in api/tags and inform user if not present
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
	// TODO: Use os.ExecPath("godot") to find if godot is even in the PATH of the user. macOS will be postfixed with .app inform user if not on PATH
	GodotBin string `json:"godot_bin"`

	// DefaultModel is a reasonable starting model for Ollama-compatible endpoints (/api/chat).
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

// bootstrapConfig generates the mcp.json config file if the file is not found within the binary directory. Ensuring there is always a config file.
func bootstrapConfig() (*Config, error) {
	// Check if config file exists, if not create it with defaults.
	cfg := defaultConfig()

	file, err := os.Create(configFileName)
	if err != nil {
		log.Fatalf("could not create mcp.json config file: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "\t")
	if err := encoder.Encode(*cfg); err != nil {
		return &Config{}, errors.New("Error creating default config;")
	}

	return cfg, nil
}

// LoadConfig reads in mcp.json config file data, if config file is missing we bootstrap one with defaults
func LoadConfig() (*Config, error) {
	file, err := os.Open(configFileName)
	if err != nil {
		errors.Is(err, os.ErrNotExist)
		log.Printf("INFO: config file does not exist, creating now...")
		return bootstrapConfig()
	}
	defer file.Close()

	var cfg *Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return &Config{}, errors.New("Error reading config file;")
	}
	return cfg, nil
}

// Path resolution rules — all relative paths are resolved against the
// working directory at startup (i.e. the directory the gd-scope binary runs from)
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
			TopK:       5,
			AutoDetect: true,
		},
		// OllamaURL intentionally empty - disables semantic search by default.
	}
}
