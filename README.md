# gd-scope v1.0

**A unified MCP server for Godot projects with Ollama-compatible endpoints**

Give AI assistants structured access to your Godot project through MCP protocol and Ollama-compatible APIs on a single port. Semantic search available for Ollama models.

---

## ⚠️ SECURITY WARNING

- ✅ Runs on **localhost only** (127.0.0.1)
- ✅ Meant for **single-user, local development**
- ⚠️ **No authentication** - anyone on your machine can access
- ⚠️ **External tools execute with YOUR permissions** - only use trusted tools
- ⚠️ **Understand the risks** - While path sandboxed, the AI can still call any enabled/available tool

**Safe usage:**
- localhost only ✓
- Trusted tools only ✓
- Development machine ✓

**Unsafe usage:**
- Network exposure ✗
- Shared servers ✗
- Untrusted tools ✗
- Production environments ✗

---

## Key Insights

🎯 **Unified Server Architecture** - Single process, single port (3333)
🔌 **Ollama-Compatible Endpoints** - Works with AI Assistant Hub, VS Code extensions, Rider plugins
🤖 **Automatic Tool Calling** - Transparent tool execution for Ollama-based clients
📚 **Dual Protocol Support** - MCP protocol for Claude Desktop/Cursor + Ollama API for everything else

---

## Quick Start

## 1. Install Prerequisites

**Ollama:**
```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# Windows
# Download from https://ollama.com/download
```

**Go 1.22+** (if building from source):
```bash
go version  # Check version
```

---

## 2. Start Ollama

**Local:**
```bash
ollama serve
```

**Remote (ollama server):**
- Point at your remote instance
- Configure `ollama_url` in mcp.json
- Example: `"ollama_url": "http://192.168.1.100:11434"`

---

## 3. Pull Models

```bash
# Chat model (recommended for tool calling)
ollama pull qwen3 # Large
ollama pull qwen2.5-coder:14b # Best balance

# Embedding model (for semantic search)
ollama pull nomic-embed-text
```

Verify:
```bash
ollama list
```

--- 

## 4. Install in Godot Project

```bash
# Create addon directory in your Godot project
mkdir -p /path/to/your/godot/project/addons/gd-scope
cd /path/to/your/project/addons/gd-scope

```

---


## 5. Build gd-scope

```bash

# Install dependencies
go mod tidy 
# Build
go build -o gd-scope .

```

---

---

## Features

### Core Tools (Always Available)

| Tool | What it does |
|------|-------------|
| `read_file` | Read any project file |
| `list_files` | List files recursively |
| `project_info` | Parse project.godot |
| `list_scenes` | Find all .tscn files |
| `read_scene` | Parse scene hierarchy |
| `list_scripts` | Find all .gd/.cs files |
| `docs_versions` | List Godot doc versions |
| `docs_list` | List pages for a version |
| `docs_get` | Get documentation page |
| `docs_search` | Full-text doc search |

### Semantic Search (Requires Ollama + Embedding Model)

| Tool | What it does |
|------|-------------|
| `index_project` | Embed project files |
| `semantic_search` | Natural language search |

### Custom Tools

Add your own tools:
- **Python/Rust/Node** - JSON stdin/stdout
- **GDScript** - Full Godot engine API access

---

## Configuration Reference

**Minimal config (auto-detects project root):**

```json
{
  "ollama_url": "http://127.0.0.1:11434"
}
```

**Full config:**

```json
{
  "_comment_security": "Server binds to localhost only. Port configurable.",
  "addr": "127.0.0.1:3333",
  
  "_comment_ollama": "Can point to local or remote Ollama instance",
  "ollama_url": "http://127.0.0.1:11434",
  
  "project_root": ".",
  "docs_dir": "docs",
  "tools_dir": "tools",
  "external_timeout_seconds": 30,
  
  "godot_bin": "godot",
  "embed_model": "nomic-embed-text",
  "default_model": "qwen3"
}
```

### Remote Ollama Setup

**On the Ollama host (e.g., desktop):**
```bash
export OLLAMA_HOST=0.0.0.0:11434
ollama serve
```

**In gd-scope mcp.json:**
```json
{
  "ollama_url": "http://192.168.1.100:11434"
}
```

---

## Available Endpoints

### Ollama-Compatible

- `POST /api/chat` - Chat with automatic tool calling
- `POST /api/generate` - Single completion (proxied)
- `GET /api/tags` - List models (proxied)

### MCP Protocol

- `GET /mcp/v1/tools` - List available tools
- `POST /mcp/v1/tools/{name}` - Call a specific tool

### Health

- `GET /health` - Server health check

---

## Documentation

- **[QUICKSTART.md](QUICKSTART.md)** - 5-minute setup guide
- **[AI-ASSISTANT-HUB.md](AI-ASSISTANT-HUB.md)** - AI Assistant Hub integration
- **[OLLAMA-SETUP.md](OLLAMA-SETUP.md)** - Ollama installation and models
- **[TOOL-CREATION.md](TOOL-CREATION.md)** - Creating custom tools
- **[API-REFERENCE.md](API-REFERENCE.md)** - Complete API documentation

---

## Requirements

- **Go 1.22+** (for building)
- **Ollama** (for LLM and embeddings)
- **Godot 4.x** (only if using GDScript tools)
- **Min. 4-8GB VRAM** (for running Ollama models)


---

## License

MIT License - see LICENSE file for details.

---

## Credits

- Built for the Godot community
- Uses [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- Designed to work with [AI Assistant Hub](https://github.com/FlamxGames/godot-ai-assistant-hub)
- Ollama integration for local AI
