package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MCP protocol types (simplified for HTTP transport)
type MCPToolsListResponse struct {
	Tools []MCPTool `json:"tools"`
}

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type MCPToolCallRequest struct {
	Arguments map[string]any `json:"arguments,omitempty"`
}

type MCPToolCallResponse struct {
	Content []MCPContent `json:"content"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// HandleMCPListTools implements GET /mcp/v1/tools
func (s *Server) HandleMCPListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var tools []MCPTool

	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()

	// Built-in tools
	for name := range s.registry.handlers {
		tools = append(tools, MCPTool{
			Name:        name,
			Description: getToolDescription(name),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": getToolParameters(name),
				"required":   getToolRequired(name),
			},
		})
	}

	// External tools
	for name, cfg := range s.registry.external {
		desc := cfg.Description
		if desc == "" {
			desc = fmt.Sprintf("External tool: %s", name)
		}
		tools = append(tools, MCPTool{
			Name:        name,
			Description: desc,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		})
	}

	response := MCPToolsListResponse{Tools: tools}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleMCPCallTool implements POST /mcp/v1/tools/{toolName}
func (s *Server) HandleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract tool name from path
	path := strings.TrimPrefix(r.URL.Path, "/mcp/v1/tools/")
	toolName := path

	var req MCPToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body for tools with no arguments
		req.Arguments = map[string]any{}
	}

	ctx := r.Context()
	result, err := s.registry.Invoke(ctx, toolName, req.Arguments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Wrap result in MCP content format
	resultJSON, _ := json.Marshal(result)
	response := MCPToolCallResponse{
		Content: []MCPContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
