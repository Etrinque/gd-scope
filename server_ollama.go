// server_ollama.go — Ollama-compatible HTTP API for the gd-scope server.
//
// # Role in the system
//
// This file is the only layer that speaks the Ollama wire protocol. It sits
// between two parties that must never be aware of each other:
//
//   - The AI Assistant Hub plugin (GDScript, FlamxGames/godot-ai-assistant-hub),
//     which sends standard Ollama /api/chat requests and expects a plain JSON
//     response whose Message.Content it renders directly in the editor UI.
//
//   - The real Ollama instance (local or remote), which performs the actual
//     inference and returns NDJSON streams.
//
// gd-scope intercepts every /api/chat request, injects tool schemas, runs a
// multi-turn tool-calling loop, and returns a single completed response — all
// transparent to the hub.
//
// # Request lifecycle
//
//  1. Hub sends POST /api/chat  (stream:false, with its own system message)
//  2. HandleOllamaChat decodes the request and calls processChatWithTools
//  3. processChatWithTools injects tool schemas + appends the tool addendum
//     to the hub's existing system message (preserving the bot's persona)
//  4. The tool loop calls callOllama → readOllamaStream repeatedly until the
//     model produces a plain text response (no tool calls)
//  5. Each tool call is dispatched to executeToolCall → registry.Invoke
//  6. The final response is returned to the hub as a single JSON object
//
// # Streaming note
//
// The hub's HTTPRequest node buffers the entire body before parsing, so it
// can only handle non-streaming (single JSON object) responses. Internally,
// however, gd-scope always streams to Ollama (stream:true) and assembles the
// complete response — this avoids a known Ollama bug where stream:false drops
// tool_calls on some builds.
//
// # Key design decisions recorded here
//
//   - Two-system-message problem: we append to the hub's existing system
//     message rather than prepend a second one — two system messages confuse
//     small models and override the user's persona configuration.
//
//   - Greeting bypass: the hub's first turn is always a greeting prompt; we
//     skip tool injection on that turn to prevent null-named tool call JSON.
//
//   - Text tool call recovery: smaller models often write tool invocations as
//     plain JSON in the content field instead of structured tool_calls. We
//     scan the content for any JSON object that matches a registered tool name
//     and promote it to a proper tool call so the loop can handle it normally.
//
//   - res:// path normalization: models correctly use Godot's res:// URI
//     scheme in tool arguments, but filepath.Join mishandles it. We strip
//     the prefix before every registry invoke.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ─── Wire types ──────────────────────────────────────────────────────────────
//
// These structs mirror the Ollama JSON API exactly. They are used both to
// decode incoming requests from the hub and to encode outgoing requests to
// Ollama. Field names and json tags must not be changed without verifying
// compatibility with both the hub (ollama_api.gd) and Ollama itself.

// OllamaMessage is a single turn in a conversation.
// The hub sends role:"system", role:"user", and role:"assistant" messages.
// Tool results are sent back as role:"tool" (Ollama format, not OpenAI format —
// there is no tool_call_id or tool_name field).
type OllamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OllamaToolCall `json:"tool_calls,omitempty"`
	// Thinking is populated by reasoning models (deepseek-r1, qwen3-thinking).
	// The hub's ResponseCleaner strips <think>...</think> tags from Content
	// before display, so thinking content never reaches the user directly.
	Thinking string `json:"thinking,omitempty"`
}

// OllamaToolCall is the structured tool invocation emitted by the model when
// it decides to call a function. Ollama uses a flat {type, function} envelope.
type OllamaToolCall struct {
	Type     string             `json:"type"`     // always "function"
	Function OllamaFunctionCall `json:"function"` // name + arguments
}

// OllamaFunctionCall holds the name and decoded arguments of a single tool call.
// Arguments is a free-form map because each tool defines its own parameter schema.
type OllamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// OllamaChatRequest is the body sent to POST /api/chat.
// The hub always sends stream:false; gd-scope reads that flag but ignores it
// when forwarding to Ollama (we always use stream:true internally).
type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	// Stream is omitempty so the hub's stream:false is preserved when
	// reflecting the request, but see internalChatRequest for why we don't
	// use this struct for outgoing Ollama calls.
	Stream  bool           `json:"stream,omitempty"`
	Tools   []OllamaTool   `json:"tools,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// OllamaTool is the JSON Schema wrapper sent in the tools array to tell
// Ollama which functions the model may call.
type OllamaTool struct {
	Type     string             `json:"type"`     // always "function"
	Function OllamaFunctionTool `json:"function"` // schema for one tool
}

// OllamaFunctionTool describes a single callable function in JSON Schema style.
// Parameters follows the standard {"type":"object","properties":{...},"required":[...]} shape.
type OllamaFunctionTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// OllamaChatResponse is the body returned from POST /api/chat (non-streaming).
// In streaming mode Ollama sends many of these as NDJSON; we assemble them into
// one before responding to the hub.
type OllamaChatResponse struct {
	Model      string        `json:"model"`
	CreatedAt  string        `json:"created_at"`
	Message    OllamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason,omitempty"`
}

// OllamaGenerateRequest is the body for POST /api/generate (completion, not chat).
// gd-scope proxies this endpoint unchanged — it is not used for tool calling.
type OllamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// internalChatRequest is the struct gd-scope uses when forwarding requests to
// the real Ollama instance. It is separate from OllamaChatRequest for one
// critical reason: Stream has no omitempty tag, so the field is always written
// to the JSON payload.
//
// If we used OllamaChatRequest (which has Stream omitempty), setting
// Stream=false would omit the field entirely, and Ollama would default to
// streaming — causing readOllamaStream to work incorrectly. With this struct
// we always send stream:true explicitly so Ollama streams and we accumulate.
type internalChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"` // no omitempty — must be written
	Tools    []OllamaTool    `json:"tools,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

// HandleOllamaChat is the main entry point for POST /api/chat.
//
// It decodes the request from the hub, runs it through the tool-calling loop,
// and returns a single completed response. The hub always sends stream:false,
// but we support streaming clients (curl, other tools) by writing NDJSON if
// the original request had stream:true.
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

	if req.Model == "" {
		req.Model = s.config.DefaultModel
		if req.Model == "" {
			req.Model = "llama3.2"
		}
	}

	// Save before processChatWithTools mutates req (it appends tool results
	// and may alter Messages in place).
	clientWantsStream := req.Stream

	ctx := r.Context()
	response, err := s.processChatWithTools(ctx, &req)
	if err != nil {
		log.Printf("ERROR: chat processing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("DEBUG: Final content length: %d", len(response.Message.Content))
	log.Printf("DEBUG: Content preview: %.200s", response.Message.Content)

	w.Header().Set("Content-Type", "application/json")

	if clientWantsStream {
		// Non-hub streaming client (curl, etc.): write the full content as a
		// single content chunk then the done:true terminator. This is valid
		// NDJSON that streaming clients can accumulate normally.
		flusher, canFlush := w.(http.Flusher)
		writeStreamChunk(w, response.Model, response.Message.Content, false)
		if canFlush {
			flusher.Flush()
		}
		writeStreamChunk(w, response.Model, "", true)
		if canFlush {
			flusher.Flush()
		}
		return
	}

	// Hub path: single JSON object. The hub's read_response() does one
	// JSON.parse() on the entire body, so this must not be NDJSON.
	json.NewEncoder(w).Encode(response)
}

// HandleOllamaTags proxies GET /api/tags directly to Ollama unchanged.
// The hub calls this to populate its model selector dropdown.
func (s *Server) HandleOllamaTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.config.OllamaURL == "" {
		http.Error(w, "Ollama not configured", http.StatusServiceUnavailable)
		return
	}
	resp, err := http.Get(s.config.OllamaURL + "/api/tags")
	if err != nil {
		http.Error(w, "Ollama unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleOllamaGenerate proxies POST /api/generate directly to Ollama unchanged.
// This is the completion (non-chat) endpoint. gd-scope does not intercept or
// augment generate requests — no tool calling, no system prompt injection.
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
	body, _ := json.Marshal(req)
	resp, err := http.Post(s.config.OllamaURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Ollama unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// writeStreamChunk writes a single NDJSON line in the Ollama streaming format.
// done=false: a content token chunk. done=true: the terminating sentinel with done_reason:"stop".
func writeStreamChunk(w http.ResponseWriter, model, content string, done bool) {
	chunk := OllamaChatResponse{
		Model:   model,
		Message: OllamaMessage{Role: "assistant", Content: content},
		Done:    done,
	}
	if done {
		chunk.DoneReason = "stop"
	}
	b, _ := json.Marshal(chunk)
	w.Write(b)
	w.Write([]byte("\n"))
}

// ─── Tool-calling loop ────────────────────────────────────────────────────────

// isHubGreeting reports whether the last user message is the AI Assistant Hub's
// standard opening prompt: "In one short sentence say hello and introduce yourself by name."
//
// Why this matters: the hub always sends this as the first user turn before any
// project-related conversation. If we inject 16 tool schemas on this turn,
// small models (llama3.1:8b, qwen2.5-coder:7b) become confused and emit a
// null-named tool call JSON blob instead of a greeting. Detecting the greeting
// lets us bypass tool injection entirely for that one turn.
func isHubGreeting(msgs []OllamaMessage) bool {
	if len(msgs) == 0 {
		return false
	}
	last := strings.ToLower(strings.TrimSpace(msgs[len(msgs)-1].Content))
	return strings.Contains(last, "say hello") && strings.Contains(last, "introduce yourself")
}

// processChatWithTools runs the full tool-calling loop for a single user turn.
//
// The loop works as follows:
//
//  1. Inject tool schemas into the request (skipped on greeting turns).
//  2. Augment the system message with tool usage instructions.
//  3. Call Ollama and read the streamed response.
//  4. If the model returned tool_calls: execute each tool, append the results
//     as role:"tool" messages, and loop back to step 3.
//  5. If the model returned only text (no tool_calls): return that text as
//     the final response.
//
// The loop runs at most maxIterations times. If that limit is hit (e.g. a model
// that calls tools indefinitely), the last assistant message seen is returned
// rather than an error — so the hub always gets a displayable response.
//
// Text tool call recovery: some smaller models write tool invocations as plain
// JSON text in the content field rather than structured tool_calls. After step 3,
// if tool_calls is empty, parseTextToolCall scans the content for any JSON object
// that looks like a tool invocation and promotes it to a proper tool call so the
// loop handles it identically to structured calls.
func (s *Server) processChatWithTools(ctx context.Context, req *OllamaChatRequest) (*OllamaChatResponse, error) {
	// Greeting bypass — see isHubGreeting for the full rationale.
	if isHubGreeting(req.Messages) {
		log.Printf("INFO: Greeting turn detected — skipping tool injection")
		return s.callOllama(ctx, req)
	}

	// Inject tool schemas if the caller hasn't already provided them.
	// The hub never sends a tools array, so this always fires for hub requests.
	if req.Tools == nil {
		req.Tools = s.getToolSchemas()
		log.Printf("INFO: Added %d tools to request", len(req.Tools))
	}

	// System prompt augmentation.
	//
	// The hub always sends a system message at messages[0] with the form:
	//   "<AIAssistantResource.ai_description>\nYour name is <bot_name>."
	// This defines the assistant's persona, name, and any custom instructions
	// the user configured in their AIAssistantResource.
	//
	// We MUST NOT replace or prepend a second system message — that creates
	// conflicting instructions and causes small models to lose their configured
	// name and personality.
	//
	// Instead, we append toolAddendum() to the existing system message so the
	// model retains its full persona and also learns about tool calling.
	//
	// When there is no system message (direct API calls, curl tests, etc.),
	// we inject a minimal standalone prompt via buildSystemPrompt().
	if len(req.Tools) > 0 {
		if idx := systemMessageIndex(req.Messages); idx >= 0 {
			// Augment the hub's existing system message in-place.
			// This modifies the local request copy only — the hub's
			// conversation history is not affected.
			req.Messages[idx].Content += s.toolAddendum()
			log.Printf("INFO: Appended tool addendum to hub system message")
		} else {
			// No system message — inject our own.
			req.Messages = append([]OllamaMessage{
				{Role: "system", Content: s.buildSystemPrompt()},
			}, req.Messages...)
			log.Printf("INFO: Injected standalone system prompt (no hub prompt detected)")
		}
	}

	// Apply default inference options if the hub didn't provide them.
	// The hub only sends options when use_custom_temperature is enabled.
	if req.Options == nil {
		req.Options = map[string]any{}
	}
	if _, ok := req.Options["num_predict"]; !ok {
		req.Options["num_predict"] = 2048
	}
	if _, ok := req.Options["temperature"]; !ok {
		req.Options["temperature"] = 0.7
	}

	const maxIterations = 10
	var lastAssistantResponse *OllamaChatResponse

	for iteration := 0; iteration < maxIterations; iteration++ {
		log.Printf("INFO: Iteration %d/%d", iteration+1, maxIterations)

		response, err := s.callOllama(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("ollama call: %w", err)
		}

		if response.Message.Role == "assistant" {
			lastAssistantResponse = response
		}

		// Text tool call recovery: if the model wrote a tool invocation as
		// plain JSON text (e.g. {"name":"list_scenes","parameters":{}})
		// instead of structured tool_calls, promote it to a real tool call
		// so the rest of this loop handles it identically.
		if len(response.Message.ToolCalls) == 0 {
			if tc := parseTextToolCall(response.Message.Content, s.registry); tc != nil {
				log.Printf("INFO: Text-based tool call detected in content: %s", tc.Function.Name)
				response.Message.ToolCalls = []OllamaToolCall{*tc}
				response.Message.Content = ""
			} else {
				// No tool calls — model produced a final text response.
				log.Printf("INFO: No tool calls — returning final response")
				return response, nil
			}
		}

		log.Printf("INFO: Model requested %d tool call(s)", len(response.Message.ToolCalls))

		// Append the model's tool-call message to the conversation so the
		// next iteration has full context.
		req.Messages = append(req.Messages, response.Message)

		// Execute each requested tool and append its result as a role:"tool"
		// message. Multiple tool calls in a single response are each appended
		// separately.
		for _, toolCall := range response.Message.ToolCalls {
			name := toolCall.Function.Name
			log.Printf("INFO: Executing tool: %s", name)

			result, err := s.executeToolCall(ctx, &toolCall)
			var content string
			if err != nil {
				log.Printf("WARN: Tool %s failed: %v", name, err)
				// Include an instruction in the error payload so the model
				// does not hallucinate file contents or node hierarchies when
				// the tool fails.
				content = fmt.Sprintf(
					`{"error":%q,"instruction":"The tool failed. Do NOT guess or fabricate file contents or node hierarchies. Tell the user exactly what failed. If the path looks wrong, suggest they verify the filename using list_scenes or list_files first."}`,
					err.Error())
			} else {
				b, _ := json.Marshal(result)
				content = string(b)
				log.Printf("INFO: Tool %s -> %d bytes", name, len(content))
			}

			// Ollama tool result wire format: role:"tool" with a content string.
			// This is NOT the OpenAI format — there is no tool_call_id and no
			// tool_name field. Do not add them; Ollama will reject or ignore them.
			req.Messages = append(req.Messages, OllamaMessage{
				Role:    "tool",
				Content: content,
			})
		}
		// Do NOT append a synthetic user message here. Doing so restarts the
		// model's deliberation and causes infinite tool-calling loops.
	}

	// Iteration cap reached. Return the last assistant message we saw rather
	// than an HTTP 500 — the hub must always receive a displayable response.
	if lastAssistantResponse != nil {
		log.Printf("WARN: Max iterations reached — returning last assistant message")
		return lastAssistantResponse, nil
	}
	return nil, fmt.Errorf("max tool-calling iterations (%d) reached with no assistant response", maxIterations)
}

// ─── Ollama communication ─────────────────────────────────────────────────────

// callOllama sends req to Ollama with stream:true and returns a fully assembled
// OllamaChatResponse once the stream is complete.
//
// We always use stream:true for outbound calls regardless of what the hub
// requested, for two reasons:
//
//  1. Some Ollama builds silently ignore stream:false on tool-call responses
//     and stream anyway; a single json.Decode() on that body reads only the
//     first token and loses everything else.
//
//  2. Tool call payloads from Ollama always arrive as a complete JSON object
//     in one chunk even when streaming, so there is no assembly complexity.
//
// The assembled response is identical to what Ollama would have returned with
// stream:false if that had worked correctly.
func (s *Server) callOllama(ctx context.Context, req *OllamaChatRequest) (*OllamaChatResponse, error) {
	if s.config.OllamaURL == "" {
		return nil, fmt.Errorf("ollama_url not configured")
	}

	// Use internalChatRequest (not OllamaChatRequest) to ensure stream:true
	// is always written to the payload — OllamaChatRequest has omitempty on
	// Stream which would silently omit it when false.
	internal := internalChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   true,
		Tools:    req.Tools,
		Options:  req.Options,
	}

	body, err := json.Marshal(internal)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.config.OllamaURL+"/api/chat", bytes.NewReader(body))
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
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}

	return readOllamaStream(resp.Body)
}

// readOllamaStream consumes an Ollama NDJSON chat stream and assembles a single
// complete OllamaChatResponse by accumulating content tokens across all chunks.
//
// Ollama stream format — one JSON object per line:
//
//	{"model":"...","message":{"role":"assistant","content":"Hello"},"done":false}
//	{"model":"...","message":{"role":"assistant","content":" world"},"done":false}
//	{"model":"...","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
//
// Special cases:
//   - Tool calls: arrive as a complete tool_calls array in a single chunk.
//     There is no partial-token streaming for tool calls.
//   - Thinking content: accumulated separately from content (used by
//     deepseek-r1, qwen3-thinking etc.) and stored in Message.Thinking.
//   - Buffer: set to 4 MB per line to handle large tool result responses
//     that the model may echo back.
func readOllamaStream(body io.Reader) (*OllamaChatResponse, error) {
	var (
		assembled OllamaChatResponse
		sb        strings.Builder
	)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("WARN: skipping unparseable stream line: %s", line)
			continue
		}

		assembled.Model = chunk.Model
		assembled.CreatedAt = chunk.CreatedAt

		// Accumulate content tokens. Most turns produce many small chunks.
		sb.WriteString(chunk.Message.Content)

		// Role is only set on the first chunk; keep the first non-empty value.
		if assembled.Message.Role == "" && chunk.Message.Role != "" {
			assembled.Message.Role = chunk.Message.Role
		}

		// Tool calls arrive complete in one chunk — overwrite, don't append.
		if len(chunk.Message.ToolCalls) > 0 {
			assembled.Message.ToolCalls = chunk.Message.ToolCalls
		}

		// Thinking content (reasoning models) accumulated across chunks.
		if chunk.Message.Thinking != "" {
			assembled.Message.Thinking += chunk.Message.Thinking
		}

		if chunk.Done {
			assembled.Done = true
			assembled.DoneReason = chunk.DoneReason
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ollama stream: %w", err)
	}

	assembled.Message.Content = sb.String()

	// Guard: Ollama omits role on done:true sentinel chunks; default to assistant.
	if assembled.Message.Role == "" {
		assembled.Message.Role = "assistant"
	}

	log.Printf("DEBUG: Stream assembled — role:%s content:%d chars tool_calls:%d",
		assembled.Message.Role, len(assembled.Message.Content), len(assembled.Message.ToolCalls))

	return &assembled, nil
}

// ─── Tool dispatch ────────────────────────────────────────────────────────────

// executeToolCall dispatches a single tool call to the registry.
//
// Before invoking the registry, it applies two normalization steps:
//  1. Intercepts the synthetic "list_tools" call (not a real registry entry)
//     and returns the live set of tool names directly.
//  2. Strips "res://" prefixes from all string arguments via normalizeToolArgs.
func (s *Server) executeToolCall(ctx context.Context, toolCall *OllamaToolCall) (any, error) {
	name := toolCall.Function.Name

	// Synthetic list_tools handler.
	//
	// The model sometimes calls "list_tools" to introspect what it can do.
	// This is not a real registry entry — we synthesize a response from the
	// live registry so the model sees an accurate, up-to-date list.
	if name == "list_tools" {
		s.registry.mu.RLock()
		var names []string
		for n := range s.registry.handlers {
			names = append(names, n)
		}
		for n := range s.registry.external {
			names = append(names, n)
		}
		s.registry.mu.RUnlock()
		log.Printf("INFO: Synthetic list_tools — returning %d tool names", len(names))
		return map[string]any{"tools": names, "count": len(names)}, nil
	}

	args := toolCall.Function.Arguments
	if args == nil {
		args = map[string]any{}
	}
	args = normalizeToolArgs(args)
	return s.registry.Invoke(ctx, name, args)
}

// normalizeToolArgs strips the Godot res:// URI scheme from all string arguments.
//
// Models correctly use Godot's res:// conventions (e.g. res://scenes/player.tscn)
// when specifying paths. However, filepath.Join(projectRoot, "res://scenes/player.tscn")
// collapses the double slash and leaves a literal "res:" directory component in
// the resulting path, causing all file operations to fail with "no such file".
//
// Stripping the prefix here — centrally, before every registry invoke — fixes
// path resolution for all tools that accept path arguments (read_file, read_scene,
// list_scripts, list_scenes, list_files, etc.) without requiring each tool
// handler to know about the Godot URI scheme.
func normalizeToolArgs(args map[string]any) map[string]any {
	norm := make(map[string]any, len(args))
	for k, v := range args {
		if str, ok := v.(string); ok {
			norm[k] = strings.TrimPrefix(str, "res://")
		} else {
			norm[k] = v
		}
	}
	return norm
}

// getToolSchemas builds the tools array to inject into each Ollama request.
//
// It reads both the built-in handler registry and the external tool configs,
// returning an OllamaTool (JSON Schema wrapper) for each. This array is what
// tells the model which functions it may call and what arguments each expects.
//
// Built-in tool schemas come from getToolDescription / getToolParameters /
// getToolRequired in tool_schemas.go. External tool schemas use the description
// and parameter spec from the tool's JSON config file in the tools/ directory.
func (s *Server) getToolSchemas() []OllamaTool {
	var tools []OllamaTool

	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()

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

// ─── Text tool call recovery ──────────────────────────────────────────────────
//
// Smaller models (llama3.1:8b, qwen2.5-coder:7b) sometimes respond with a tool
// invocation written as plain JSON text inside the content field rather than
// using the structured tool_calls array. This happens when the model understands
// it should call a tool but fails to produce the correct response format.
//
// Example of what the model emits:
//
//	"To answer this, I would call: {\"name\":\"list_scripts\",\"parameters\":{}}"
//
// extractJSONObjects + parseTextToolCall together handle this case by scanning
// the content for any JSON object that matches a known tool name and promoting
// it to a proper OllamaToolCall so the main loop can handle it identically to
// a structured call.

// extractJSONObjects scans content for all top-level JSON objects and returns
// each one as a parsed map. It handles JSON embedded in surrounding prose by
// looking for '{' characters and attempting to parse a balanced object from
// each position. Unparseable or partial objects are silently skipped.
func extractJSONObjects(content string) []map[string]any {
	var results []map[string]any
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		// Walk forward tracking brace depth to find the matching close brace.
		depth := 0
		for j := i; j < len(content); j++ {
			switch content[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var m map[string]any
					if err := json.Unmarshal([]byte(content[i:j+1]), &m); err == nil {
						results = append(results, m)
					}
					goto nextChar
				}
			}
		}
	nextChar:
	}
	return results
}

// parseTextToolCall searches content for a JSON object that looks like a tool
// invocation referencing a registered tool, and returns it as an OllamaToolCall.
//
// Returns nil if:
//   - content is empty or contains no JSON objects
//   - no JSON object matches a registered tool name
//   - the tool name found is not in the registry (prevents acting on arbitrary JSON)
//
// Supported JSON formats emitted by different models:
//
//	{"name":"tool","arguments":{...}}    ← Format 1  (most structured models)
//	{"name":"tool","parameters":{...}}   ← Format 1b (qwen2.5-coder variant)
//	{"tool":"tool","parameters":{...}}   ← Format 2  (some llama variants)
//	{"function":{"name":"tool",...}}     ← Format 3  (OpenAI-style leakage)
func parseTextToolCall(content string, reg *Registry) *OllamaToolCall {
	if len(strings.TrimSpace(content)) == 0 {
		return nil
	}

	objects := extractJSONObjects(content)
	if len(objects) == 0 {
		return nil
	}

	for _, raw := range objects {
		var name string
		var args map[string]any
		hasArgs := false

		// Format 1 / 1b: {"name":"tool","arguments":{}} or {"name":"tool","parameters":{}}
		name, _ = raw["name"].(string)
		if name != "" {
			if a, ok := raw["arguments"].(map[string]any); ok {
				args, hasArgs = a, true
			} else if p, ok := raw["parameters"].(map[string]any); ok {
				args, hasArgs = p, true
			} else {
				// Name present but no known args key — proceed with empty args.
				hasArgs = true
			}
		}

		// Format 2: {"tool":"tool","parameters":{...}}
		if name == "" {
			name, _ = raw["tool"].(string)
			args, hasArgs = raw["parameters"].(map[string]any)
		}

		// Format 3: {"function":{"name":"tool","arguments":{...}}}
		if name == "" {
			if fn, ok := raw["function"].(map[string]any); ok {
				name, _ = fn["name"].(string)
				args, hasArgs = fn["arguments"].(map[string]any)
			}
		}

		if name == "" {
			continue
		}
		if !hasArgs || args == nil {
			args = map[string]any{}
		}

		// Allow list_tools through even though it is not in the registry.
		// It is handled synthetically in executeToolCall and intentionally
		// omitted from the registry to avoid appearing in tool schemas.
		isSynthetic := name == "list_tools"

		if !isSynthetic {
			// Reject anything else not in the registry — this prevents
			// arbitrary JSON blobs in the model's response being treated
			// as tool calls (e.g. fabricated paths, meta questions).
			reg.mu.RLock()
			_, isBuiltin := reg.handlers[name]
			_, isExternal := reg.external[name]
			reg.mu.RUnlock()

			if !isBuiltin && !isExternal {
				log.Printf("DEBUG: text-tool-call candidate %q not in registry — skipping", name)
				continue
			}
		}

		log.Printf("INFO: Recovered text-based tool call: %s args=%v", name, args)
		return &OllamaToolCall{
			Type: "function",
			Function: OllamaFunctionCall{
				Name:      name,
				Arguments: args,
			},
		}
	}

	// No registered tool name found in any JSON object in the content.
	return nil
}

// ─── System prompt helpers ────────────────────────────────────────────────────

// systemMessageIndex returns the index of the first system-role message in msgs,
// or -1 if none is present.
func systemMessageIndex(msgs []OllamaMessage) int {
	for i, m := range msgs {
		if m.Role == "system" {
			return i
		}
	}
	return -1
}

// buildSystemPrompt is used only when no system message is present in the
// incoming request — i.e. for direct API calls (curl, tests, non-hub clients).
//
// It must NOT hardcode a name or persona. Those belong to the user's
// AIAssistantResource configuration in the hub, and the hub always sends its
// own system message. This function is only ever reached by non-hub callers.
func (s *Server) buildSystemPrompt() string {
	return `You are a helpful AI assistant embedded in the Godot editor.` + s.toolAddendum()
}

// toolAddendum returns the block of tool-calling instructions that is appended
// to whatever system message is already present.
//
// Design constraints:
//   - Additive only: must not override persona, name, or style. The hub's
//     system message already contains those; this block adds tool knowledge.
//   - Dynamic: the tool list is built from the live registry on every call,
//     so user-defined external tools appear automatically without any changes
//     to this file. No hardcoded tool names.
//   - Compact: injected on every non-greeting turn, so it should not bloat
//     the context window unnecessarily for small models.
//
// The hub sends (as the system message):
//
//	"<AIAssistantResource.ai_description>\nYour name is <bot_name>."
//
// After our append it becomes:
//
//	"<ai_description>\nYour name is <bot_name>.\n\n## Godot project tools..."
func (s *Server) toolAddendum() string {
	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()

	var sb strings.Builder

	// Built-in tools — descriptions sourced from tool_schemas.go.
	for name := range s.registry.handlers {
		fmt.Fprintf(&sb, "\n- %s: %s", name, getToolDescription(name))
	}

	// External / user-defined tools (GDScript, Python, Rust, etc.).
	// Description comes from the "description" field in the tool's JSON config.
	// Authors should write a one-liner describing what the tool does and when
	// to use it — this feeds both the function-calling schema and this catalogue.
	for name, cfg := range s.registry.external {
		desc := cfg.Description
		if desc == "" {
			desc = fmt.Sprintf("user-defined %s tool", cfg.Type)
		}
		fmt.Fprintf(&sb, "\n- %s: %s", name, desc)
	}

	return `

## Godot project tools (injected by gd-scope)
You have tools that can read the user's actual Godot project. Always use them when the user asks about specific project files, scenes, scripts, nodes, or settings.

### When to call a tool (call without asking for permission):
- User asks what scenes/scripts/files exist → list_scenes / list_scripts / list_files
- User asks about a specific file → read_file or read_scene
- User asks you to explain, analyze, or summarize a script → call read_file FIRST, then explain
- User asks what a scene contains or what nodes it has → read_scene
- User asks about project settings, autoloads, or dependencies → project_info
- User asks to search code → semantic_search or docs_search
- Use list_tools to get the current list of all available tools at runtime

### When NOT to call a tool:
- Greetings and introductions ("hello", "who are you", "that will be all")
- Conversational acknowledgments ("ok", "thanks", "that is fine", "got it", "understood", "sounds good")
- Questions about yourself — your model name, context window, capabilities, version, or memory
- General Godot API questions that do not reference a specific file in THIS project
- Any turn where the user is just making conversation, not asking about project content

### CRITICAL rules:
- NEVER explain or describe a script's contents from memory — always call read_file first
- NEVER fabricate file contents or node hierarchies — if a tool fails, say so and suggest the user check the filename
- File paths: use bare paths like "scenes/player.tscn" or "scripts/player.gd" — do NOT prefix with res://

Available tools:` + sb.String() + `

After getting tool results, summarize clearly in plain language. Do not return raw JSON to the user.`
}
