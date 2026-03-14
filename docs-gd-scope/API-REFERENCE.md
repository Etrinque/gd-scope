# gd-scope API Reference

Complete API documentation for both Ollama-compatible and MCP protocol endpoints.

---

## Base URL

Default: `http://localhost:3333`

Configurable via `addr` in `mcp.json`

---

## Ollama-Compatible Endpoints

### POST /api/chat

Chat completion with automatic tool calling.

**Request:**
```json
{
  "model": "qwen2.5-coder:7b",
  "messages": [
    {
      "role": "user",
      "content": "What scenes are in my project?"
    }
  ],
  "stream": false
}
```

**Response:**
```json
{
  "model": "qwen2.5-coder:7b",
  "created_at": "2025-02-19T10:30:00Z",
  "message": {
    "role": "assistant",
    "content": "I found 12 scenes in your project: ..."
  },
  "done": true
}
```

**Tool Calling (Automatic):**

If the model needs project data, it will automatically call tools. This is transparent to the client.

Internal flow:
1. Model requests tool: `list_scenes`
2. gd-scope executes tool
3. gd-scope sends result back to model
4. Model generates final response
5. Client receives final response

---

### POST /api/generate

Single text completion (proxied to Ollama).

**Request:**
```json
{
  "model": "qwen2.5-coder:7b",
  "prompt": "Write a GDScript function that...",
  "stream": false
}
```

**Response:**
```json
{
  "model": "qwen2.5-coder:7b",
  "created_at": "2025-02-19T10:30:00Z",
  "response": "Here's a GDScript function...",
  "done": true
}
```

---

### GET /api/tags

List available Ollama models (proxied).

**Response:**
```json
{
  "models": [
    {
      "name": "qwen2.5-coder:7b",
      "modified_at": "2025-02-19T10:00:00Z",
      "size": 4661224448
    },
    {
      "name": "nomic-embed-text:latest",
      "modified_at": "2025-02-19T09:00:00Z",
      "size": 274301184
    }
  ]
}
```

---

## MCP Protocol Endpoints

### GET /mcp/v1/tools

List all available tools.

**Response:**
```json
{
  "tools": [
    {
      "name": "read_file",
      "description": "Read a file from the Godot project...",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": {
            "type": "string",
            "description": "Path to the file"
          }
        },
        "required": ["path"]
      }
    },
    {
      "name": "list_scenes",
      "description": "List all .tscn scene files...",
      "inputSchema": {
        "type": "object",
        "properties": {
          "root": {
            "type": "string",
            "description": "Root directory (optional)"
          }
        }
      }
    }
  ]
}
```

---

### POST /mcp/v1/tools/{tool_name}

Call a specific tool.

**Example: read_file**

Request:
```json
{
  "arguments": {
    "path": "scripts/player.gd"
  }
}
```

Response:
```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"path\":\"scripts/player.gd\",\"content\":\"extends CharacterBody2D...\"}"
    }
  ]
}
```

**Example: list_scenes**

Request:
```json
{
  "arguments": {}
}
```

Response:
```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"scenes\":[\"res://scenes/main.tscn\",\"res://scenes/player.tscn\",...]}"
    }
  ]
}
```

**Example: semantic_search**

Request:
```json
{
  "arguments": {
    "query": "enemy AI behavior",
    "top_k": 3
  }
}
```

Response:
```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"query\":\"enemy AI behavior\",\"results\":[{\"path\":\"scripts/enemy_ai.gd\",\"score\":0.89,...}]}"
    }
  ]
}
```

---

## Tool Catalog

### Filesystem Tools

#### read_file
Read file contents.

**Arguments:**
- `path` (string, required): File path

**Returns:**
```json
{
  "path": "scripts/player.gd",
  "content": "extends CharacterBody2D\n..."
}
```

#### list_files
List files recursively.

**Arguments:**
- `root` (string, optional): Root directory (default: project root)

**Returns:**
```json
{
  "root": "scripts",
  "files": [
    "scripts/player.gd",
    "scripts/enemy.gd",
    ...
  ]
}
```

---

### Godot Project Tools

#### project_info
Get project configuration.

**Arguments:** None

**Returns:**
```json
{
  "name": "My Game",
  "version": "4.3.0",
  "main_scene": "res://scenes/main.tscn",
  "autoloads": {
    "GameManager": "res://autoload/game_manager.gd",
    "AudioManager": "res://autoload/audio_manager.gd"
  }
}
```

#### list_scenes
List all .tscn files.

**Arguments:**
- `root` (string, optional): Root directory

**Returns:**
```json
{
  "scenes": [
    "res://scenes/main_menu.tscn",
    "res://scenes/gameplay.tscn",
    ...
  ]
}
```

#### read_scene
Parse scene structure.

**Arguments:**
- `path` (string, required): Path to .tscn file

**Returns:**
```json
{
  "path": "res://scenes/player.tscn",
  "nodes": [
    {
      "name": "Player",
      "type": "CharacterBody2D",
      "parent": ".",
      "props": {
        "script": "res://scripts/player.gd"
      }
    },
    ...
  ]
}
```

#### list_scripts
List all script files.

**Arguments:**
- `root` (string, optional): Root directory

**Returns:**
```json
{
  "scripts": [
    "res://scripts/player.gd",
    "res://scripts/enemy.gd",
    ...
  ]
}
```

---

### Documentation Tools

#### docs_versions
List available doc versions.

**Arguments:** None

**Returns:**
```json
{
  "versions": ["4.3", "4.2", "4.1"]
}
```

#### docs_list
List pages for a version.

**Arguments:**
- `version` (string, required): Doc version

**Returns:**
```json
{
  "version": "4.3",
  "pages": ["nodes", "signals", "physics", ...]
}
```

#### docs_get
Get a documentation page.

**Arguments:**
- `version` (string, required): Doc version
- `page` (string, required): Page name (without .md)

**Returns:**
```json
{
  "version": "4.3",
  "page": "physics",
  "content": "# Physics in Godot 4.3\n..."
}
```

#### docs_search
Search documentation.

**Arguments:**
- `version` (string, required): Version or "all"
- `query` (string, required): Search query

**Returns:**
```json
{
  "version": "4.3",
  "query": "CharacterBody2D",
  "hits": [
    {
      "page": "physics",
      "matches": 3,
      "preview": "...CharacterBody2D is..."
    }
  ]
}
```

---

### Semantic Search Tools

*(Only available if Ollama is configured)*

#### index_project
Index project for semantic search.

**Arguments:** None

**Returns:**
```json
{
  "indexed": 47,
  "skipped": 2,
  "total": 49
}
```

#### semantic_search
Search by meaning.

**Arguments:**
- `query` (string, required): Natural language query
- `top_k` (integer, optional): Number of results (default: 5)

**Returns:**
```json
{
  "query": "player movement code",
  "results": [
    {
      "path": "scripts/player.gd",
      "score": 0.87,
      "preview": "extends CharacterBody2D..."
    },
    ...
  ]
}
```

---

### Management Tools

#### reload_tools
Reload external tool configs.

**Arguments:** None

**Returns:**
```json
{
  "status": "ok"
}
```

---

## Error Responses

All endpoints return errors in this format:

**HTTP 400 Bad Request:**
```json
{
  "error": "Invalid request: missing required field"
}
```

**HTTP 404 Not Found:**
```json
{
  "error": "Tool not found: unknown_tool"
}
```

**HTTP 500 Internal Server Error:**
```json
{
  "error": "Tool execution failed: ..."
}
```

**HTTP 503 Service Unavailable:**
```json
{
  "error": "Ollama unreachable"
}
```

---

## Rate Limiting

Currently no rate limiting implemented. Consider using a reverse proxy (nginx, Caddy) if exposing publicly.

---

## Authentication

Currently no authentication. gd-scope is designed for local development use. If exposing to network:

1. Use SSH tunneling
2. Add reverse proxy with auth (nginx + basic auth)
3. Use VPN

---

## Health Check

### GET /health

Check server health.

**Response:**
```
OK
```

**Status Code:** 200

---

## Client Examples

### Python

```python
import requests

# Chat
response = requests.post('http://localhost:3333/api/chat', json={
    'model': 'qwen2.5-coder:7b',
    'messages': [
        {'role': 'user', 'content': 'List my scenes'}
    ]
})
print(response.json()['message']['content'])

# Direct tool call
response = requests.post('http://localhost:3333/mcp/v1/tools/list_scenes', 
    json={'arguments': {}})
print(response.json())
```

### JavaScript

```javascript
// Chat
const response = await fetch('http://localhost:3333/api/chat', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    model: 'qwen2.5-coder:7b',
    messages: [{role: 'user', content: 'List my scenes'}]
  })
});
const data = await response.json();
console.log(data.message.content);

// Direct tool call
const toolResponse = await fetch('http://localhost:3333/mcp/v1/tools/list_scenes', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({arguments: {}})
});
console.log(await toolResponse.json());
```

### curl

```bash
# Chat
curl -X POST http://localhost:3333/api/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen2.5-coder:7b","messages":[{"role":"user","content":"List scenes"}]}'

# Direct tool call
curl -X POST http://localhost:3333/mcp/v1/tools/list_scenes -d '{}'

# With arguments
curl -X POST http://localhost:3333/mcp/v1/tools/read_file \
  -d '{"arguments":{"path":"scripts/player.gd"}}'
```

---

For implementation examples, see the client integration guides.
