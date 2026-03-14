package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Ollama API types - CORRECTED for actual Ollama format
type OllamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OllamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"` // For tool result messages
	Thinking  string           `json:"thinking,omitempty"`  // For reasoning chains
}

// Ollama's actual tool call format
type OllamaToolCall struct {
	Type     string             `json:"type"`
	Function OllamaFunctionCall `json:"function"`
}

type OllamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"` // Ollama uses map directly, not JSON string
}

type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
	Tools    []OllamaTool    `json:"tools,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type OllamaTool struct {
	Type     string             `json:"type"`
	Function OllamaFunctionTool `json:"function"`
}

type OllamaFunctionTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type OllamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   OllamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

type OllamaGenerateRequest struct {
	Model  string         `json:"model"`
	Prompt string         `json:"prompt"`
	Stream bool           `json:"stream,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

type OllamaGenerateResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
}

// HandleOllamaChat implements /api/chat - Ollama-compatible chat with automatic tool calling
func (s *Server) HandleOllamaChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OllamaChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Default model if not specified
	if req.Model == "" {
		req.Model = s.config.DefaultModel
		if req.Model == "" {
			req.Model = "llama3.2"
		}
	}

	ctx := r.Context()
	response, err := s.processChatWithTools(ctx, &req)
	if err != nil {
		log.Printf("ERROR: chat processing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// DEBUG: Log the response we're sending
	log.Printf("DEBUG: Sending response - Role: %s, Content length: %d", 
		response.Message.Role, 
		len(response.Message.Content))
	log.Printf("DEBUG: Content preview (first 200 chars): %.200s", response.Message.Content)
	
	// DEBUG: Log full JSON for debugging AI Assistant Hub integration
	responseJSON, _ := json.MarshalIndent(response, "", "  ")
	log.Printf("DEBUG: Full response JSON:\n%s", string(responseJSON))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// processChatWithTools handles the tool calling loop
func (s *Server) processChatWithTools(ctx context.Context, req *OllamaChatRequest) (*OllamaChatResponse, error) {
	// Add available tools to request if not already present
	if req.Tools == nil {
		req.Tools = s.getToolSchemas()
		log.Printf("INFO: Added %d tools to request", len(req.Tools))
	}

	// CRITICAL: Add system prompt to encourage tool use
	hasSystemPrompt := false
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			hasSystemPrompt = true
			break
		}
	}

	if !hasSystemPrompt && len(req.Tools) > 0 {
		systemPrompt := `You are a helpful AI assistant with access to tools for analyzing Godot game projects.

IMPORTANT: When the user asks about their project (scenes, scripts, files, code), you MUST use the available tools to get accurate information. Do not guess or make up information about their project.

Available tools:
- list_scenes: List all .tscn scene files
- list_scripts: List all .gd/.cs script files  
- list_files: List all project files
- read_file: Read contents of a specific file
- project_info: Get project configuration and autoloads
- read_scene: Parse a scene file's node hierarchy
- semantic_search: Search code by natural language description
- And more...

WORKFLOW:
1. Call the appropriate tool(s) to get information
2. After receiving tool results, provide a COMPLETE and DETAILED explanation
3. Format your response clearly with the relevant information from the tool results
4. If the results are a list, present them in a readable format
5. Add helpful context and explanations, not just raw data

NEVER respond with just one word or a fragment. Always provide a full, helpful explanation of what you found.`

		req.Messages = append([]OllamaMessage{
			{Role: "system", Content: systemPrompt},
		}, req.Messages...)

		log.Printf("INFO: Added system prompt to encourage tool use")
	}
	
	// Set reasonable defaults for Ollama options if not provided
	if req.Options == nil {
		req.Options = map[string]any{}
	}
	if _, ok := req.Options["num_predict"]; !ok {
		req.Options["num_predict"] = 2048 // Allow longer responses
	}
	if _, ok := req.Options["temperature"]; !ok {
		req.Options["temperature"] = 0.7 // Balanced creativity
	}

	maxIterations := 10 // Prevent infinite tool calling loops
	for iteration := 0; iteration < maxIterations; iteration++ {
		log.Printf("INFO: Tool calling iteration %d/%d", iteration+1, maxIterations)

		// Call Ollama
		response, err := s.callOllama(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("ollama call: %w", err)
		}

		// Check if model wants to use tools
		if len(response.Message.ToolCalls) == 0 {
			// No more tool calls - return final response
			log.Printf("INFO: No tool calls, returning final response")
			return response, nil
		}

		log.Printf("INFO: Model requested %d tool calls", len(response.Message.ToolCalls))

		// Add assistant's message with tool calls to history
		req.Messages = append(req.Messages, response.Message)

		// Execute each tool call
		log.Printf("INFO: === EXECUTING %d TOOLS (v2.0-RESPONSE-FIX) ===", len(response.Message.ToolCalls))
		for _, toolCall := range response.Message.ToolCalls {
			log.Printf("INFO: Executing tool: %s", toolCall.Function.Name)
			
			result, err := s.executeToolCall(ctx, &toolCall)
			if err != nil {
				log.Printf("WARN: Tool %s failed: %v", toolCall.Function.Name, err)
				// Add error as tool result (Ollama format)
				req.Messages = append(req.Messages, OllamaMessage{
					Role:     "tool",
					ToolName: toolCall.Function.Name, // CORRECTED: use tool_name not tool_call_id
					Content:  fmt.Sprintf(`{"error": "%s"}`, err.Error()),
				})
				continue
			}

			// Add successful result (Ollama format)
			resultJSON, _ := json.Marshal(result)
			log.Printf("INFO: Tool %s succeeded, result size: %d bytes", toolCall.Function.Name, len(resultJSON))
			req.Messages = append(req.Messages, OllamaMessage{
				Role:     "tool",
				ToolName: toolCall.Function.Name, // CORRECTED: use tool_name
				Content:  string(resultJSON),
			})
		}
		
		// After all tool results, add a reminder to provide detailed explanation
		log.Printf("INFO: ====== ADDING FOLLOW-UP PROMPT (v2.0-RESPONSE-FIX) ======")
		// This helps prevent single-word or incomplete responses
		req.Messages = append(req.Messages, OllamaMessage{
			Role:    "user",
			Content: "Based on the tool results above, please provide a complete and detailed explanation. Format the information clearly and explain what you found.",
		})
		log.Printf("INFO: Added follow-up prompt for detailed explanation")
	}

	return nil, fmt.Errorf("max tool calling iterations (%d) reached", maxIterations)
}

// executeToolCall runs a tool via the registry
func (s *Server) executeToolCall(ctx context.Context, toolCall *OllamaToolCall) (any, error) {
	// Ollama already provides arguments as map[string]any, not JSON string
	args := toolCall.Function.Arguments
	if args == nil {
		args = map[string]any{}
	}

	result, err := s.registry.Invoke(ctx, toolCall.Function.Name, args)
	if err != nil {
		return nil, fmt.Errorf("invoke %s: %w", toolCall.Function.Name, err)
	}

	return result, nil
}

// callOllama forwards request to actual Ollama instance
func (s *Server) callOllama(ctx context.Context, req *OllamaChatRequest) (*OllamaChatResponse, error) {
	if s.config.OllamaURL == "" {
		return nil, fmt.Errorf("ollama_url not configured")
	}

	url := s.config.OllamaURL + "/api/chat"
	
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	var response OllamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &response, nil
}

// getToolSchemas converts registry tools to Ollama function calling format
func (s *Server) getToolSchemas() []OllamaTool {
	var tools []OllamaTool

	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()

	// Add built-in tools
	for name := range s.registry.handlers {
		tools = append(tools, OllamaTool{
			Type: "function",
			Function: OllamaFunctionTool{
				Name:        name,
				Description: getToolDescription(name),
				Parameters: map[string]any{
					"type":       "object",
					"properties": getToolParameters(name),
					"required":   getToolRequired(name),
				},
			},
		})
	}

	// Add external tools
	for name, cfg := range s.registry.external {
		desc := cfg.Description
		if desc == "" {
			desc = fmt.Sprintf("External tool: %s", name)
		}
		tools = append(tools, OllamaTool{
			Type: "function",
			Function: OllamaFunctionTool{
				Name:        name,
				Description: desc,
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}

	return tools
}

// HandleOllamaTags proxies /api/tags to Ollama
func (s *Server) HandleOllamaTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.config.OllamaURL == "" {
		http.Error(w, "Ollama not configured", http.StatusServiceUnavailable)
		return
	}

	url := s.config.OllamaURL + "/api/tags"
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "Ollama unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleOllamaGenerate implements /api/generate endpoint
func (s *Server) HandleOllamaGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OllamaGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if s.config.OllamaURL == "" {
		http.Error(w, "Ollama not configured", http.StatusServiceUnavailable)
		return
	}

	// Proxy to Ollama
	url := s.config.OllamaURL + "/api/generate"
	body, _ := json.Marshal(req)
	
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Ollama unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
