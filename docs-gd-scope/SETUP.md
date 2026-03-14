# Complete Setup Guide

**From zero to running in 10 minutes**

---

## Prerequisites Installation

### 1. Install Go (for building)

**macOS:**
```bash
brew install go
```

**Linux:**
```bash
# Download from https://go.dev/dl/
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

**Windows:**
Download installer from https://go.dev/dl/ and run it.

Verify:
```bash
go version
# Should show: go version go1.22.0 ...
```

### 2. Install Ollama

**macOS:**
```bash
brew install ollama
```

**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**Windows:**
Download from https://ollama.com/download

Verify:
```bash
ollama --version
```

---

## Build gd-scope

```bash
# Extract the zip
unzip gd-scope-v2.zip
cd gd-scope

# Install dependencies
go mod tidy

# Build
go build -o gd-scope .

# Verify
./gd-scope --help
```

You should now have a `gd-scope` binary (or `gd-scope.exe` on Windows).

### Optional: Install Globally

**macOS/Linux:**
```bash
sudo cp gd-scope /usr/local/bin/
```

**Windows:**
Add the gd-scope directory to your PATH environment variable.

---

## Configure Ollama

### Start Ollama Service

**macOS/Linux:**
```bash
ollama serve
```

Keep this running in a terminal. Or run in background:

**macOS (launchd):**
```bash
cat > ~/Library/LaunchAgents/com.ollama.plist << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ollama</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ollama</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
PLIST

launchctl load ~/Library/LaunchAgents/com.ollama.plist
```

**Linux (systemd):**
```bash
sudo cat > /etc/systemd/system/ollama.service << 'SERVICE'
[Unit]
Description=Ollama Service
After=network.target

[Service]
Type=simple
User=$USER
ExecStart=/usr/local/bin/ollama serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
SERVICE

sudo systemctl enable ollama
sudo systemctl start ollama
```

**Windows:**
Ollama runs as a service automatically.

### Pull Models

```bash
# Chat model (choose based on RAM)
ollama pull qwen2.5-coder:7b     # 4GB RAM - recommended
# OR
ollama pull deepseek-coder-v2:16b  # 8GB+ RAM - more powerful

# Embedding model (for semantic search)
ollama pull nomic-embed-text
```

Verify:
```bash
ollama list
# Should show both models
```

Test Ollama:
```bash
curl http://127.0.0.1:11434/api/tags
# Should return JSON with model list
```

---

## Configure gd-scope

### Option 1: Minimal Config

Navigate to your Godot project root:
```bash
cd /path/to/your/godot/project
```

Create `mcp.json`:
```json
{
  "project_root": ".",
  "ollama_url": "http://127.0.0.1:11434"
}
```

That's it! Defaults will be used for everything else.

### Option 2: Full Config

```json
{
  "project_root": ".",
  "docs_dir": "docs",
  "tools_dir": "tools",
  "addr": ":3333",
  "external_timeout_seconds": 30,

  "godot_bin": "godot",
  
  "ollama_url": "http://127.0.0.1:11434",
  "embed_model": "nomic-embed-text",
  "default_model": "qwen2.5-coder:7b"
}
```

---

## First Run

```bash
cd /path/to/your/godot/project
/path/to/gd-scope
```

You should see:
```
=== gd-scope server starting ===
Listening on :3333
Available endpoints:
  MCP protocol:       http://localhost:3333/mcp/v1/tools
  Ollama-compatible:  http://localhost:3333/api/chat
  Health check:       http://localhost:3333/health

Project root: /path/to/your/project
Loaded tools: 12 built-in, 0 external
Ollama integration: enabled (http://127.0.0.1:11434)
  Semantic search: available
  Default model: qwen2.5-coder:7b

Ready to accept connections...
```

### Test It

In another terminal:
```bash
# Health check
curl http://localhost:3333/health
# Should return: OK

# List tools
curl http://localhost:3333/mcp/v1/tools | jq '.tools[].name'
# Should list: read_file, list_files, project_info, etc.

# Test a tool
curl -X POST http://localhost:3333/mcp/v1/tools/project_info -d '{}'
# Should return your project info
```

---

## Client Setup

### AI Assistant Hub (Godot Plugin)

1. **Install AI Assistant Hub:**
   - Open Godot
   - Go to AssetLib
   - Search "AI Assistant Hub"
   - Install and enable

2. **Configure:**
   - Open AI Assistant Hub settings
   - API Type: Ollama
   - API URL: `http://localhost:3333`
   - Model: `qwen2.5-coder:7b`

3. **Create Assistant:**
   - Click "New Assistant Type"
   - Name: "Project Helper"
   - Save

4. **Test:**
   ```
   You: What scenes are in my project?
   
   AI: [Automatically calls list_scenes]
       I found 15 scenes in your project:
       - res://scenes/main_menu.tscn
       - res://scenes/levels/level_1.tscn
       ...
   ```

### Claude Desktop

1. **Find config file:**
   - **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
   - **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
   - **Linux:** `~/.config/Claude/claude_desktop_config.json`

2. **Edit config:**
   ```json
   {
     "mcpServers": {
       "godot": {
         "command": "/absolute/path/to/gd-scope",
         "env": {
           "MCP_TRANSPORT": "stdio"
         }
       }
     }
   }
   ```

3. **Restart Claude Desktop**

4. **Test:**
   Look for tools icon in chat. Ask "What's in my Godot project?"

### VS Code Extension (Example)

Create a simple extension:

```typescript
// extension.ts
import * as vscode from 'vscode';
import axios from 'axios';

const GODOT_MCP_URL = 'http://localhost:3333';

async function askAboutProject() {
    const prompt = await vscode.window.showInputBox({
        prompt: 'Ask about your Godot project'
    });
    
    if (!prompt) return;
    
    const response = await axios.post(`${GODOT_MCP_URL}/api/chat`, {
        model: 'qwen2.5-coder:7b',
        messages: [
            {role: 'user', content: prompt}
        ]
    });
    
    const answer = response.data.message.content;
    vscode.window.showInformationMessage(answer, { modal: true });
}

export function activate(context: vscode.ExtensionContext) {
    context.subscriptions.push(
        vscode.commands.registerCommand('gd-scope.ask', askAboutProject)
    );
}
```

---

## Enable Semantic Search

1. **Index your project:**
   
   In AI Assistant Hub:
   ```
   You: Index my project for semantic search
   
   AI: [Calls index_project]
       Indexed 47 files successfully.
   ```
   
   Or via curl:
   ```bash
   curl -X POST http://localhost:3333/mcp/v1/tools/index_project -d '{}'
   ```

2. **Use semantic search:**
   ```
   You: Find code related to enemy AI
   
   AI: [Calls semantic_search]
       I found several relevant files:
       1. scripts/enemy_ai.gd (89% match)
          - State machine implementation
       2. scripts/enemy_vision.gd (76% match)
          - Raycasting for player detection
       ...
   ```

---

## Running as a Service

### macOS (launchd)

```bash
cat > ~/Library/LaunchAgents/com.gd-scope.plist << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.gd-scope</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/gd-scope</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/path/to/your/godot/project</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/gd-scope.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/gd-scope.error.log</string>
</dict>
</plist>
PLIST

launchctl load ~/Library/LaunchAgents/com.gd-scope.plist
```

### Linux (systemd)

```bash
sudo cat > /etc/systemd/system/gd-scope.service << 'SERVICE'
[Unit]
Description=Godot MCP Server
After=network.target ollama.service

[Service]
Type=simple
User=$USER
WorkingDirectory=/path/to/your/godot/project
ExecStart=/usr/local/bin/gd-scope
Restart=on-failure

[Install]
WantedBy=multi-user.target
SERVICE

sudo systemctl enable gd-scope
sudo systemctl start gd-scope
```

Check logs:
```bash
sudo journalctl -u gd-scope -f
```

---

## Troubleshooting

### "Ollama unreachable"

```bash
# Check Ollama is running
curl http://127.0.0.1:11434/api/tags

# If not, start it
ollama serve

# Check config
cat mcp.json | grep ollama_url
# Should be: "ollama_url": "http://127.0.0.1:11434"
```

### "Cannot find project.godot"

```bash
# Check you're in the right directory
ls project.godot

# Or set absolute path in mcp.json
{
  "project_root": "/Users/you/projects/my-game"
}
```

### "Port already in use"

```bash
# Find what's using port 3333
lsof -i :3333

# Kill it
pkill gd-scope

# Or use different port
{
  "addr": ":3334"
}
```

### Semantic search not working

```bash
# Check embedding model
ollama list | grep nomic-embed-text

# If missing
ollama pull nomic-embed-text

# Index project
curl -X POST http://localhost:3333/mcp/v1/tools/index_project -d '{}'
```

### Tools not loading

```bash
# Check tools directory exists
ls tools/

# Check JSON configs
ls tools/*.json

# Reload tools
curl -X POST http://localhost:3333/mcp/v1/tools/reload_tools -d '{}'
```

---

## Next Steps

1. Read [QUICKSTART.md](QUICKSTART.md) for quick reference
2. Read [AI-ASSISTANT-HUB.md](AI-ASSISTANT-HUB.md) for full integration guide
3. Read [TOOL-CREATION.md](TOOL-CREATION.md) to create custom tools
4. Read [API-REFERENCE.md](API-REFERENCE.md) for complete API docs
5. Read [ARCHITECTURE.md](ARCHITECTURE.md) to understand internals

---

## Getting Help

- **Issues:** Check [Troubleshooting](#troubleshooting) section
- **GitHub:** https://github.com/yourname/gd-scope/issues
- **Docs:** All markdown files in this directory

---

**Enjoy building with gd-scope!** 🚀
