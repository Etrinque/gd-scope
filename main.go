package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	reg, err := NewRegistry(cfg)
	if err != nil {
		log.Fatalf("registry init: %v", err)
	}

	if err := reg.Load(); err != nil {
		log.Fatalf("registry load: %v", err)
	}

	// Server instance handles both protocols
	srv := &Server{
		config:   cfg,
		registry: reg,
	}

	ctx := context.Background()

	// Check for stdio mode (for pure MCP clients like Claude Desktop)
	if os.Getenv("MCP_TRANSPORT") == "stdio" {
		log.Println("transport: stdio (MCP only)")
		mcpServer := mcp.NewServer(&mcp.Implementation{
			Name:    "gd-scope",
			Version: "1.0.0",
		}, nil)
		registerMCPTools(mcpServer, reg)
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("mcp server: %v", err)
		}
		return
	}

	// HTTP mode - supports both MCP and Ollama-compatible endpoints
	mux := http.NewServeMux()

	// MCP endpoints (for Claude Desktop, Cursor via HTTP)
	mux.HandleFunc("/mcp/v1/tools", srv.HandleMCPListTools)
	mux.HandleFunc("/mcp/v1/tools/", srv.HandleMCPCallTool)

	// Ollama-compatible endpoints (for AI Assistant Hub, VS Code, Rider)
	mux.HandleFunc("/api/chat", srv.HandleOllamaChat)
	mux.HandleFunc("/api/tags", srv.HandleOllamaTags)
	mux.HandleFunc("/api/generate", srv.HandleOllamaGenerate)

	// Server health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Status: OK"))
	})

	// TODO: Endpint for quickly viewing logs in browser in json format
	// mux.HandleFunc("/log", srv.HandleGetLogs)

	addr := cfg.Addr
	if addr == "" {
		addr = ":3333"
	}

	if srv.config != nil && srv.registry != nil {
		printStartLog(addr, cfg, reg)
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func printStartLog(addr string, cfg *Config, reg *Registry) {
	log.Printf("=== gd-scope server starting ===")
	log.Printf("Listening on %s", addr)
	log.Printf("")
	log.Printf("Available endpoints:")
	log.Printf("  MCP protocol:       http://localhost%s/mcp/v1/tools", addr)
	log.Printf("  Ollama-compatible:  http://localhost%s/api/chat", addr)
	log.Printf("  Health check:       http://localhost%s/health", addr)
	log.Printf("")
	log.Printf("Project root: %s", cfg.ProjectRoot)
	log.Printf("Loaded tools: %d built-in, %d external", len(reg.handlers), len(reg.external))

	if cfg.OllamaURL != "" {
		log.Printf("Ollama: enabled (%s)", cfg.OllamaURL)
		log.Printf("  Semantic search: available")
		log.Printf("  Default model: %s", cfg.DefaultModel)
	}

	log.Printf("")
	log.Printf("Ready to accept connections...")

}

// TODO: This is doodoo, Server is not the actual server instance it is behavior, it is told to act in a particular way and it can only act on known, particular things...
// Server holds the unified server state
type Server struct {
	config   *Config
	registry *Registry
}

// TODO: Refactor to Registry method, no reason for this to be in main entry... Split into exported method on reg git statuRegisterTools, private registerBuiltInTools, registerExternalTools
// registerMCPTools registers all tools with the MCP server (for stdio mode)
func registerMCPTools(srv *mcp.Server, reg *Registry) {
	// Helper to create tool handler that wraps registry.Invoke
	invoke := func(name string) func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
			// Convert input to map[string]any for registry
			var params map[string]any
			if input != nil {
				params, _ = input.(map[string]any)
			}
			if params == nil {
				params = map[string]any{}
			}

			result, err := reg.Invoke(ctx, name, params)
			return nil, result, err
		}
	}

	// Filesystem
	mcp.AddTool(srv, &mcp.Tool{Name: "read_file", Description: "Read a file. Args: path (string)"}, invoke("read_file"))
	mcp.AddTool(srv, &mcp.Tool{Name: "list_files", Description: "List files. Args: root (string, optional)"}, invoke("list_files"))

	// Godot project
	mcp.AddTool(srv, &mcp.Tool{Name: "project_info", Description: "Parse project.godot. No args."}, invoke("project_info"))
	mcp.AddTool(srv, &mcp.Tool{Name: "list_scenes", Description: "List .tscn files. Args: root (optional)"}, invoke("list_scenes"))
	mcp.AddTool(srv, &mcp.Tool{Name: "read_scene", Description: "Parse .tscn file. Args: path (string)"}, invoke("read_scene"))
	mcp.AddTool(srv, &mcp.Tool{Name: "list_scripts", Description: "List .gd/.cs files. Args: root (optional)"}, invoke("list_scripts"))

	// Docs
	mcp.AddTool(srv, &mcp.Tool{Name: "docs_versions", Description: "List doc versions."}, invoke("docs_versions"))
	mcp.AddTool(srv, &mcp.Tool{Name: "docs_list", Description: "List pages. Args: version (string)"}, invoke("docs_list"))
	mcp.AddTool(srv, &mcp.Tool{Name: "docs_get", Description: "Get page. Args: version, page (strings)"}, invoke("docs_get"))
	mcp.AddTool(srv, &mcp.Tool{Name: "docs_search", Description: "Search docs. Args: version, query (strings)"}, invoke("docs_search"))

	// Semantic search (only if Ollama configured)
	reg.mu.RLock()
	hasOllama := reg.cfg.OllamaURL != ""
	reg.mu.RUnlock()

	if hasOllama {
		mcp.AddTool(srv, &mcp.Tool{Name: "index_project", Description: "Index project for semantic search."}, invoke("index_project"))
		mcp.AddTool(srv, &mcp.Tool{Name: "semantic_search", Description: "Semantic search. Args: query (string), top_k (int, optional)"}, invoke("semantic_search"))
	}

	// Management
	mcp.AddTool(srv, &mcp.Tool{Name: "reload_tools", Description: "Reload tool configs."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
			if err := reg.Load(); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"status": "ok"}, nil
		},
	)

	reg.RegisterExternalMCPTools(srv)
}
