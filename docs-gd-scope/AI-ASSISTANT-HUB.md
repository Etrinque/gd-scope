# AI Assistant Hub + Ollama + gd-scope Integration Guide

**Complete guide for connecting AI Assistant Hub to gd-scope through Ollama**

This guide shows you how to set up the complete integration so your AI assistants in Godot can access powerful project-aware tools.

---

## Table of Contents

- [Understanding the Integration](#understanding-the-integration)
- [Prerequisites](#prerequisites)
- [Part 1: Install Ollama](#part-1-install-ollama)
- [Part 2: Install and Configure gd-scope](#part-2-install-and-configure-gd-scope)
- [Part 3: Install AI Assistant Hub](#part-3-install-ai-assistant-hub)
- [Part 4: Configure AI Assistant Hub to Use gd-scope](#part-4-configure-ai-assistant-hub-to-use-gd-scope)
- [Part 5: Using the Complete System](#part-5-using-the-complete-system)
- [Part 6: Advanced Configuration](#part-6-advanced-configuration)
- [Troubleshooting](#troubleshooting)
- [Example Workflows](#example-workflows)

---

## Understanding the Integration

### The Complete Flow

```
┌─────────────────────┐
│      User           │  "Find all code related to player movement"
│  (You in Godot)     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  AI Assistant Hub   │  Godot Plugin - Chat interface in editor
│  (Godot Plugin)     │  Can READ and WRITE code
└──────────┬──────────┘
           │ Sends prompt
           ▼
┌─────────────────────┐
│      Ollama         │  LLM with function calling
│  (Local LLM API)    │  Decides when to use tools
└──────────┬──────────┘
           │ Calls MCP tools when needed
           ▼
┌─────────────────────┐
│    gd-scope        │  MCP Server - Provides project data
│   (MCP Server)      │  • Read files
└─────────────────────┘  • Parse scenes
                         • Search docs
                         • Semantic search
                         • And more...
```

### Why This Matters

**Without gd-scope:** AI Assistant Hub can only see the code you paste into the chat or select in the editor.

**With gd-scope:** The AI can:
- List all scenes in your project
- Read any script or scene file
- Parse `.tscn` files to understand node hierarchies
- Search documentation for Godot APIs
- Find code semantically ("show me enemy AI code")
- Understand your entire project structure

### What Each Component Does

**AI Assistant Hub (Godot Plugin)**
- Lives inside Godot editor
- Provides chat interface
- Can write code directly into scripts
- Manages conversation with Ollama
- Repository: https://github.com/FlamxGames/godot-ai-assistant-hub

**Ollama (LLM Runtime)**
- Runs large language models locally
- Provides function calling capability
- Acts as MCP client when calling tools
- Download: https://ollama.com

**gd-scope (MCP Server)**
- Provides tools for reading/analyzing Godot projects
- Sandboxes access to your project directory
- Enables semantic search with embeddings
- This is what you're building

---

## Prerequisites

- **Godot 4.3+** (AI Assistant Hub requirement)
- **Go 1.22+** (to build gd-scope)
- **4-8GB RAM** for Ollama models
- **~500MB disk space** for models

---

## Part 1: Install Ollama

### Installation

**macOS:**
```bash
brew install ollama
```

**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**Windows:**
Download installer from [https://ollama.com/download](https://ollama.com/download)

### Start Ollama

**macOS/Linux:**
```bash
ollama serve
```

Keep this running, or run in background:
```bash
nohup ollama serve > /dev/null 2>&1 &
```

**Windows:**
Runs as a service automatically after installation.

### Verify Ollama is Running

```bash
curl http://127.0.0.1:11434/api/tags

# Should return: {"models":[]}
```

### Pull Models

You need TWO types of models:

**1. Chat Model (for AI Assistant Hub conversations):**
```bash
# Good for coding (choose based on your RAM)
ollama pull qwen2.5-coder:7b    # 4GB RAM - fast, good for coding
ollama pull deepseek-coder-v2:16b  # 8GB RAM - excellent for coding
ollama pull codestral:22b       # 16GB RAM - very powerful

# Or general purpose
ollama pull llama3.2:3b         # 2GB RAM - fast, general
ollama pull mistral:7b          # 4GB RAM - good balance
```

**2. Embedding Model (for gd-scope semantic search):**
```bash
ollama pull nomic-embed-text    # 274MB - required for semantic search
```

### Verify Models

```bash
ollama list

# Should show both your chat model and nomic-embed-text
```

---

## Part 2: Install and Configure gd-scope

### Build gd-scope

```bash
# Clone or download the gd-scope code
cd gd-scope
go mod tidy
go build -o gd-scope .
```

### Create Configuration

Create `mcp.json` in your **Godot project root**:

```json
{
  "project_root": ".",
  "docs_dir": "docs",
  "tools_dir": "tools",
  "addr": ":3333",
  
  "ollama_url": "http://127.0.0.1:11434",
  "embed_model": "nomic-embed-text"
}
```

**Important:** Put this file in your Godot project directory, not next to the gd-scope binary.

### Test gd-scope

```bash
cd /path/to/your/godot/project
/path/to/gd-scope
```

You should see:
```
INFO: loaded 12 tools
INFO: semantic search enabled (ollama connected)
INFO: transport: HTTP :3333
```

Press Ctrl+C to stop for now.

---

## Part 3: Install AI Assistant Hub

### Download and Install

**Method 1: Godot Asset Library (Recommended)**

1. Open Godot
2. Go to **AssetLib** tab
3. Search for "AI Assistant Hub"
4. Click **Download** then **Install**
5. Restart Godot

**Method 2: Manual Installation**

1. Download from [https://github.com/FlamxGames/godot-ai-assistant-hub](https://github.com/FlamxGames/godot-ai-assistant-hub)
2. Extract the `addons/ai_assistant_hub` folder
3. Copy it to your project's `addons/` directory: `res://addons/ai_assistant_hub/`
4. Enable the plugin: **Project > Project Settings > Plugins**
5. Check the box next to "AI Assistant Hub"

### Verify Installation

You should see a new **AI Hub** tab in Godot's bottom panel.

---

## Part 4: Configure AI Assistant Hub to Use gd-scope

### Step 1: Configure Ollama Connection in AI Assistant Hub

1. Click the **AI Hub** tab in Godot's bottom panel
2. You'll see a setup wizard or settings panel
3. Configure the Ollama connection:
   - **API Type:** Select "Ollama"
   - **API URL:** `http://127.0.0.1:11434`
   - **Model:** Select the chat model you downloaded (e.g., `qwen2.5-coder:7b`)

### Step 2: Configure MCP Tools (gd-scope)

AI Assistant Hub can connect to MCP servers. Here's how to add gd-scope:

**If AI Assistant Hub has MCP configuration UI:**

1. Look for **MCP Servers** or **Tools** settings in AI Assistant Hub
2. Add a new MCP server:
   - **Name:** `gd-scope`
   - **URL:** `http://127.0.0.1:3333` (or whatever port you configured)
   - **Type:** `HTTP` or `MCP-HTTP`

**If configuring via Ollama's MCP support:**

Some Ollama setups support MCP tool registration. Check Ollama documentation for:
```bash
# Add MCP server to Ollama
ollama mcp add godot http://127.0.0.1:3333
```

### Step 3: Start gd-scope

Before using AI Assistant Hub with MCP tools, make sure gd-scope is running:

```bash
cd /path/to/your/godot/project
/path/to/gd-scope
```

Keep this terminal open while you work in Godot.

### Step 4: Create Your First Assistant

In AI Assistant Hub:

1. Click **"New Assistant Type"** or similar button
2. Configure your assistant:
   - **Name:** "Project Assistant" or "Godot Helper"
   - **Model:** Select your Ollama model (e.g., `qwen2.5-coder:7b`)
   - **System Prompt:** (Optional) Customize the assistant's behavior
   
Example system prompt:
```
You are an expert Godot game developer assistant. You have access to tools 
that let you read and analyze the user's Godot project. Use these tools 
proactively to help answer questions about their code, scenes, and project 
structure. When asked to find or explain code, always use the available 
tools to read the actual files.
```

3. Save the assistant
4. Click the assistant button to start a new chat

---

## Part 5: Using the Complete System

### Available Tools (What gd-scope Provides)

When you ask the AI a question, it can automatically use these tools:

**Filesystem:**
- `read_file` - Read any project file
- `list_files` - List all files in directories

**Godot Project:**
- `project_info` - Parse project.godot (version, autoloads, settings)
- `list_scenes` - Find all .tscn files
- `read_scene` - Parse scene node hierarchy and properties
- `list_scripts` - Find all .gd and .cs files

**Documentation:**
- `docs_versions` - List available Godot doc versions
- `docs_list` - List pages for a version
- `docs_get` - Get full documentation page
- `docs_search` - Search docs by keyword

**Semantic Search (requires Ollama + nomic-embed-text):**
- `index_project` - Embed all project files for semantic search
- `semantic_search` - Find code by natural language description

**Management:**
- `reload_tools` - Reload external tool configurations

### Example Conversations

**Example 1: Understanding Project Structure**

```
You: What scenes are in my project?

AI: [Automatically calls list_scenes tool]
    I found 15 scenes in your project:
    
    Main scenes:
    - res://scenes/main_menu.tscn
    - res://scenes/gameplay.tscn
    
    Levels:
    - res://scenes/levels/level_1.tscn
    - res://scenes/levels/level_2.tscn
    - res://scenes/levels/level_3.tscn
    
    Player:
    - res://scenes/player/player.tscn
    
    [continues listing...]
```

**Example 2: Analyzing a Scene**

```
You: What nodes are in my Player scene?

AI: [Calls read_scene with path="res://scenes/player/player.tscn"]
    The Player scene has this node hierarchy:
    
    - Player (CharacterBody2D)
      ├─ Sprite2D
      ├─ CollisionShape2D
      ├─ Camera2D
      ├─ AnimationPlayer
      ├─ HealthComponent (Node)
      │  └─ HealthBar (ProgressBar)
      └─ InputHandler (Node)
    
    The Player root node has a script attached: res://scripts/player.gd
    
    Would you like me to examine the player.gd script?
```

**Example 3: Finding Code**

```
You: Find all code related to saving and loading

AI: [Calls semantic_search with query="save load system"]
    I found several files related to saving and loading:
    
    1. scripts/save_manager.gd (89% match)
       Main save/load system implementation
       
    2. scripts/player.gd (74% match)
       Contains save_state() and load_state() methods
       
    3. scripts/game_data.gd (71% match)
       Defines the SaveGame data structure
    
    [Calls read_file on save_manager.gd]
    
    Here's the main save system:
    [Shows code with explanation]
```

**Example 4: Code Generation with Context**

```
You: Create a pickup item script that can be saved/loaded

AI: [Calls read_file on save_manager.gd to understand save format]
    [Calls docs_search for "Node2D" and "Area2D" in Godot docs]
    
    Based on your project's save system, here's a pickup item script:
    
    [AI writes the code directly into Godot's script editor]
    [Code follows your project's existing patterns]
```

**Example 5: Documentation Lookup**

```
You: How do I use move_and_slide in Godot 4.3?

AI: [Calls docs_search with version="4.3", query="move_and_slide"]
    [Calls docs_get to retrieve the CharacterBody2D documentation]
    
    In Godot 4.3, move_and_slide() is called on CharacterBody2D without 
    parameters. The velocity is now a built-in property:
    
    ```gdscript
    extends CharacterBody2D
    
    func _physics_process(delta):
        velocity.y += gravity * delta
        
        if Input.is_action_pressed("jump") and is_on_floor():
            velocity.y = JUMP_VELOCITY
        
        var direction = Input.get_axis("left", "right")
        velocity.x = direction * SPEED
        
        move_and_slide()  # No parameters needed!
    ```
    
    Key changes from Godot 3.x:
    - velocity is a property, not passed as parameter
    - No need for UP_DIRECTION parameter
    - Returns nothing (velocity is updated in place)
```

### Proactive Tool Usage

The AI should use tools automatically based on context. You can encourage this:

```
You: Review my Player script for best practices

AI: [Calls list_scripts to find player scripts]
    [Calls read_file on scripts/player.gd]
    [Calls docs_search to look up relevant Godot best practices]
    [Analyzes and provides feedback based on actual code]
```

---

## Part 6: Advanced Configuration

### Running gd-scope as a Background Service

**macOS/Linux (systemd):**

Create `/etc/systemd/system/gd-scope.service`:
```ini
[Unit]
Description=Godot MCP Server
After=network.target ollama.service

[Service]
Type=simple
User=youruser
WorkingDirectory=/path/to/your/godot/project
ExecStart=/usr/local/bin/gd-scope
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable:
```bash
sudo systemctl enable gd-scope
sudo systemctl start gd-scope
```

**macOS (launchd):**

Create `~/Library/LaunchAgents/com.gd-scope.plist`:
```xml
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
</dict>
</plist>
```

Load:
```bash
launchctl load ~/Library/LaunchAgents/com.gd-scope.plist
```

### Customizing Tool Behavior

Create custom tools by adding JSON configs to `tools/` directory:

**Example: Custom scene validator**

`tools/validate_scene.json`:
```json
{
  "name": "validate_scene",
  "description": "Validate a scene file for common issues",
  "type": "gdscript",
  "script": "./tools/validate_scene.gd"
}
```

`tools/validate_scene.gd`:
```gdscript
extends "res://tools/mcp_tool_base.gd"

func _run(args: Dictionary) -> Dictionary:
    var path: String = args.get("path", "")
    if not path.begins_with("res://"):
        path = "res://" + path
    
    var warnings := []
    var errors := []
    
    var scene := ResourceLoader.load(path, "PackedScene")
    if scene == null:
        return {"error": "Could not load scene"}
    
    var state := scene.get_state()
    
    # Check for missing scripts
    for i in range(state.get_node_count()):
        for j in range(state.get_node_property_count(i)):
            var prop_name := state.get_node_property_name(i, j)
            if prop_name == "script":
                var script_path = state.get_node_property_value(i, j)
                if script_path and not ResourceLoader.exists(str(script_path)):
                    errors.append({
                        "node": state.get_node_name(i),
                        "issue": "Missing script: " + str(script_path)
                    })
    
    return {
        "path": path,
        "valid": errors.is_empty(),
        "warnings": warnings,
        "errors": errors
    }
```

Reload tools without restarting:
```
You: reload the tools

AI: [Calls reload_tools]
    Tools reloaded. New tool available: validate_scene
```

### Adding Versioned Documentation

Create documentation for your project that the AI can search:

```bash
mkdir -p docs/4.3
```

Add markdown files:
```markdown
# My Game's Architecture

## Save System

The save system uses JSON serialization...

## Combat System

Combat damage is calculated using...
```

The AI can now search your custom docs:
```
You: How does my game's combat system work?

AI: [Calls docs_search with query="combat system"]
    [Calls docs_get to read the full page]
    [Explains based on YOUR documentation]
```

---

## Troubleshooting

### AI Assistant Hub Can't Connect to Ollama

**Symptoms:**
- "Connection refused" errors
- AI doesn't respond

**Solutions:**

1. **Check Ollama is running:**
```bash
curl http://127.0.0.1:11434/api/tags
```

2. **Verify model is downloaded:**
```bash
ollama list
# Should show your chat model
```

3. **Check AI Assistant Hub settings:**
- API URL should be `http://127.0.0.1:11434`
- Model name should match exactly from `ollama list`

### AI Doesn't Use gd-scope Tools

**Symptoms:**
- AI says "I can't access your project files"
- No tool calls visible

**Solutions:**

1. **Verify gd-scope is running:**
```bash
curl http://127.0.0.1:3333/mcp/v1/tools
# Should return JSON with tool list
```

2. **Check MCP configuration in AI Assistant Hub:**
- Make sure gd-scope URL is correct
- Restart AI Assistant Hub after configuration

3. **Test tool calling directly:**
```bash
curl -X POST http://127.0.0.1:3333/mcp/v1/tools/list_scenes \
  -H "Content-Type: application/json" \
  -d '{}'
```

4. **Check model supports function calling:**
Some models don't support function calling well. Try:
- `qwen2.5-coder` - Excellent function calling
- `llama3.1` and above - Good support
- Older models may not work

5. **Update system prompt:**
Add to your assistant's system prompt:
```
You have access to tools for reading and analyzing the Godot project.
ALWAYS use these tools when the user asks about project files, scenes,
or code. Use list_scenes to see all scenes, read_scene to examine 
scene structure, and read_file to view script contents.
```

### Semantic Search Not Working

**Symptoms:**
- `semantic_search` tool not available
- "index_project" command fails

**Solutions:**

1. **Check Ollama has embedding model:**
```bash
ollama list | grep nomic-embed-text
```

If missing:
```bash
ollama pull nomic-embed-text
```

2. **Verify mcp.json configuration:**
```json
{
  "ollama_url": "http://127.0.0.1:11434",
  "embed_model": "nomic-embed-text"
}
```

3. **Index your project first:**
```
You: Index my project for semantic search

AI: [Calls index_project]
    Indexed 47 files successfully.
```

4. **Check gd-scope logs:**
Look for:
```
INFO: semantic search enabled (ollama connected)
```

If you see:
```
WARN: ollama unreachable - semantic search disabled
```

Verify Ollama is running and the URL is correct.

### "Path Outside Sandbox" Errors

**Symptoms:**
- AI can't read files
- "path outside sandbox" errors

**Solutions:**

1. **Check project_root in mcp.json:**
```json
{
  "project_root": "/absolute/path/to/your/godot/project"
}
```

Must be absolute path to your project root (where project.godot is).

2. **Verify gd-scope is running in project directory:**
```bash
cd /path/to/your/godot/project
/path/to/gd-scope
```

3. **Check file paths are project-relative:**
File paths should be relative to project root:
- ✅ `scenes/player.tscn`
- ✅ `res://scenes/player.tscn`
- ❌ `/Users/you/projects/game/scenes/player.tscn`

---

## Example Workflows

### Workflow 1: Daily Development

**Morning setup:**
```bash
# Terminal 1
ollama serve

# Terminal 2  
cd ~/my-game
/path/to/gd-scope

# Start Godot
godot
```

**In AI Assistant Hub:**
```
You: Index my project for semantic search

AI: [Calls index_project]
    Indexed 52 files. Semantic search is now available.

You: What did I work on yesterday?

AI: [Calls semantic_search with "recent changes commits TODO"]
    [Calls read_file on relevant scripts]
    [Summarizes recent work based on TODOs and comments]
```

### Workflow 2: Refactoring

```
You: I want to move player health logic into a separate component

AI: [Calls read_file on scripts/player.gd]
    [Analyzes the health-related code]
    [Calls list_scripts to check if HealthComponent already exists]
    
    I can help you extract the health system. Currently your Player script
    has these health-related elements:
    - var health: int = 100
    - func take_damage(amount: int)
    - func heal(amount: int)
    - signal health_changed(new_health: int)
    
    Let me create a HealthComponent for you...
    
    [AI creates the component and shows you how to integrate it]
```

### Workflow 3: Learning Godot APIs

```
You: Show me examples of Area2D from my project, then explain best practices

AI: [Calls semantic_search with "Area2D collision detection"]
    [Finds your existing Area2D usage]
    
    I found 3 examples of Area2D in your project:
    1. scenes/pickups/coin.tscn - for pickup detection
    2. scenes/hazards/spike.tscn - for damage zones  
    3. scenes/player/hurt_box.tscn - for player hit detection
    
    [Shows your actual code]
    
    [Calls docs_get for Area2D documentation]
    
    Based on Godot 4.3 documentation and your usage:
    [Explains best practices using your code as examples]
```

### Workflow 4: Bug Investigation

```
You: My player sometimes falls through the floor. Help me debug this.

AI: [Calls read_scene on player scene]
    [Calls read_file on player.gd]
    [Calls read_scene on level scene]
    [Calls docs_search for "collision" and "physics"]
    
    I've analyzed your player and level setup. Here are potential issues:
    
    1. Your player's CollisionShape2D has no shape resource set
    2. The floor in level_1.tscn uses StaticBody2D but collision layer 
       is set to 0 (disabled)
    3. Your player's physics process doesn't check is_on_floor() before 
       applying gravity
    
    Let me show you the fixes needed...
    [Provides specific solutions based on actual code]
```

---

## Quick Start Checklist

- [ ] Install Ollama
- [ ] Pull chat model: `ollama pull qwen2.5-coder:7b`
- [ ] Pull embedding model: `ollama pull nomic-embed-text`
- [ ] Start Ollama: `ollama serve`
- [ ] Build gd-scope: `go build -o gd-scope .`
- [ ] Create `mcp.json` in Godot project root
- [ ] Start gd-scope: `cd project && /path/to/gd-scope`
- [ ] Install AI Assistant Hub in Godot (AssetLib)
- [ ] Configure AI Assistant Hub to connect to Ollama
- [ ] Configure AI Assistant Hub to use gd-scope tools
- [ ] Create your first assistant
- [ ] Test: "What scenes are in my project?"
- [ ] Index for semantic search: "Index my project"
- [ ] Test semantic search: "Find enemy AI code"

---

## Additional Resources

- **AI Assistant Hub:** https://github.com/FlamxGames/godot-ai-assistant-hub
- **Ollama:** https://ollama.com
- **Ollama Models:** https://ollama.com/library
- **MCP Protocol:** https://modelcontextprotocol.io
- **gd-scope GitHub:** [your-repo-url]

---

*Last updated: February 2025 | gd-scope v1.0.0*
