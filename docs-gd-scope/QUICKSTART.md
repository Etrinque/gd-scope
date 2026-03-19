# gd-scope Quickstart

**Get running in ~5 minutes**

---

## ⚠️ Read This First

gd-scope runs on **localhost only** for your security. External tools execute with your permissions - only use tools you trust.

---

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
# Create addon directory in your Godot project if it doesnt exist
mkdir -p /path/to/your/project/addons
# Cd into the addons dir
cd /path/to/your/project/addons
# Clone repo down into Godot project addons directory
git clone https://github.com/etrinque/gd-scope
# Cd to gd-scope directory
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

## 6. Configure (Optional)

Edit `addons/gd-scope/mcp.json`:

**Minimal (auto-detects everything):**
```json
{
  "ollama_url": "http://127.0.0.1:11434"
}
```

**With remote Ollama:**
```json
{
  "ollama_url": "http://192.168.1.100:11434"
}
```

---

## 7. Run

```bash
cd /path/to/your/project/addons/gd-scope
./gd-scope
```

You should see:
```
=== gd-scope server starting ===
Listening on 127.0.0.1:3333 (localhost only)
...
Project root: /path/to/your/project
Ready to accept connections...
```

---

## 8. Test It

```bash
# Test health
curl http://localhost:3333/health

# List tools
curl http://localhost:3333/mcp/v1/tools

# Call a tool
curl -X POST http://localhost:3333/mcp/v1/tools/project_info -d '{}'
```

---

## 9. Connect Your Client

### AI Assistant Hub (Godot Plugin)

1. Install AI Assistant Hub from AssetLib
2. Configure:
   ```
   API Type: Ollama
   API URL: http://localhost:3333
   Model: qwen3
   ```
3. Ask: "What scenes are in my project?"

### Claude Desktop

Edit config file:

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "godot": {
      "command": "/path/to/your/project/addons/gd-scope/gd-scope",
      "env": {"MCP_TRANSPORT": "stdio"}
    }
  }
}
```

Restart Claude Desktop.

---

## Common Issues

**"Ollama unreachable"**
```bash
# Check Ollama is running
curl http://127.0.0.1:11434/api/tags

# If not
ollama serve
```

**"SECURITY: Server must bind to localhost"**
- This is intentional security protection
- Server only runs on localhost (127.0.0.1)
- Port is configurable in addr field

**Semantic search not working**
```bash
# Pull embedding model
ollama pull nomic-embed-text

# Index project first
curl -X POST http://localhost:3333/mcp/v1/tools/index_project -d '{}'
```
> Or ask model via Ai-Assistant-Hub
> User Prompt: Please (re) index my project.

---

**You're ready to go!** 🚀
