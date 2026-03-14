# Tool Creation Guide

How to extend gd-scope with custom tools.

---

## Tool Types

gd-scope supports three types of custom tools:

| Type | Language | Use Case | Performance |
|------|----------|----------|-------------|
| **Built-in** | Go | Core functionality | Instant |
| **External** | Any | Custom logic, portable | ~10-50ms overhead |
| **GDScript** | GDScript | Godot-specific, engine API | ~100-200ms overhead |

---

## External Tools (Python, Rust, Node.js, etc.)

External tools are standalone executables that communicate via JSON.

### Contract

```
stdin:  JSON object with arguments
stdout: JSON object with result
exit 0: success
exit non-zero: error (stderr becomes error message)
```

### Example: Python Tool

**tools/my_tool.py:**
```python
#!/usr/bin/env python3
import json
import sys

# Read arguments from stdin
args = json.load(sys.stdin)
path = args.get("path", "")

if not path:
    print(json.dumps({"error": "missing 'path' argument"}))
    sys.exit(1)

# Do your work
with open(path, 'r') as f:
    lines = f.readlines()

# Return result
result = {
    "path": path,
    "line_count": len(lines)
}
print(json.dumps(result))
sys.exit(0)
```

**tools/my_tool.json:**
```json
{
  "name": "count_lines",
  "description": "Count lines in a file. Args: path (string).",
  "type": "external",
  "command": "./tools/my_tool.py"
}
```

**Make executable:**
```bash
chmod +x tools/my_tool.py
```

**Reload:**
```bash
curl -X POST http://localhost:3333/mcp/v1/tools/reload_tools
```

### Example: Rust Tool

**tools/rust_tool/Cargo.toml:**
```toml
[package]
name = "rust_tool"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
```

**tools/rust_tool/src/main.rs:**
```rust
use serde::{Deserialize, Serialize};
use std::io::{self, Read};

#[derive(Deserialize)]
struct Args {
    path: String,
}

#[derive(Serialize)]
struct Result {
    path: String,
    exists: bool,
    size: u64,
}

fn main() -> io::Result<()> {
    // Read JSON from stdin
    let mut input = String::new();
    io::stdin().read_to_string(&mut input)?;
    
    let args: Args = serde_json::from_str(&input)?;
    
    // Check file
    let metadata = std::fs::metadata(&args.path)?;
    
    // Return result
    let result = Result {
        path: args.path,
        exists: true,
        size: metadata.len(),
    };
    
    println!("{}", serde_json::to_string(&result)?);
    Ok(())
}
```

**Build:**
```bash
cd tools/rust_tool
cargo build --release
cp target/release/rust_tool ../file_check
```

**tools/file_check.json:**
```json
{
  "name": "file_check",
  "description": "Check if file exists and get size. Args: path (string).",
  "type": "external",
  "command": "./tools/file_check"
}
```

---

## GDScript Tools

GDScript tools have full access to the Godot engine API via headless mode.

### Base Class

All GDScript tools extend `mcp_tool_base.gd`:

**tools/mcp_tool_base.gd:**
```gdscript
extends SceneTree

func _init():
    var cmdline_args = OS.get_cmdline_user_args()
    var json_str = cmdline_args[0]
    var json = JSON.new()
    var parse_result = json.parse(json_str)
    var args = json.get_data()
    
    # Normalize res:// paths
    if args.has("path") and args.path is String:
        if not args.path.begins_with("res://"):
            args.path = "res://" + args.path
    
    var result = _run(args)
    print(JSON.stringify(result))
    quit(0)

func _run(args: Dictionary) -> Dictionary:
    return {"error": "not implemented"}
```

### Example: Scene Validator

**tools/validate_scene.gd:**
```gdscript
extends "res://tools/mcp_tool_base.gd"

func _run(args: Dictionary) -> Dictionary:
    var path: String = args.get("path", "")
    if path.is_empty():
        return {"error": "missing 'path' argument"}
    
    if not ResourceLoader.exists(path):
        return {"error": "scene not found"}
    
    var scene := ResourceLoader.load(path, "PackedScene")
    if scene == null:
        return {"error": "not a valid scene"}
    
    var warnings := []
    var errors := []
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
                        "issue": "missing script: " + str(script_path)
                    })
    
    return {
        "path": path,
        "valid": errors.is_empty(),
        "warnings": warnings,
        "errors": errors,
        "node_count": state.get_node_count()
    }
```

**tools/validate_scene.json:**
```json
{
  "name": "validate_scene",
  "description": "Validate a scene file for common issues. Args: path (string).",
  "type": "gdscript",
  "script": "./tools/validate_scene.gd"
}
```

### GDScript API Access

GDScript tools can use the full Godot API:

```gdscript
# Load resources
var scene = ResourceLoader.load("res://scene.tscn")
var texture = ResourceLoader.load("res://icon.png")

# Access file system
var dir = DirAccess.open("res://")
dir.list_dir_begin()
var file_name = dir.get_next()

# Use engine features
var regex = RegEx.new()
regex.compile("[0-9]+")

# Access project settings
var setting = ProjectSettings.get_setting("application/config/name")
```

---

## Built-in Tools (Go)

Built-in tools are compiled into the gd-scope binary.

### Adding a Built-in Tool

**1. Create handler function:**

Add to `tools_custom.go` (create if needed):
```go
package main

import "context"

func (r *Registry) myCustomTool(ctx context.Context, args map[string]any) (any, error) {
    path, _ := args["path"].(string)
    
    // Your logic here
    result := map[string]any{
        "path": path,
        "result": "value",
    }
    
    return result, nil
}
```

**2. Register in registry.go:**

Add to `NewRegistry()`:
```go
func NewRegistry(cfg *Config) (*Registry, error) {
    // ... existing code ...
    
    r.handlers = map[string]ToolHandler{
        // ... existing handlers ...
        "my_custom_tool": r.myCustomTool,
    }
    
    return r, nil
}
```

**3. Add metadata in tool_schemas.go:**

```go
func getToolDescription(name string) string {
    descriptions := map[string]string{
        // ... existing ...
        "my_custom_tool": "Description of what it does. Args: path (string).",
    }
    // ...
}

func getToolParameters(name string) map[string]any {
    schemas := map[string]map[string]any{
        // ... existing ...
        "my_custom_tool": {
            "path": map[string]any{
                "type": "string",
                "description": "Path to file",
            },
        },
    }
    // ...
}

func getToolRequired(name string) []string {
    required := map[string][]string{
        // ... existing ...
        "my_custom_tool": {"path"},
    }
    // ...
}
```

**4. Rebuild:**
```bash
go build -o gd-scope .
```

---

## Tool Best Practices

### Error Handling

Always return errors in this format:
```json
{"error": "descriptive error message"}
```

Never exit with 0 if there's an error.

### Input Validation

Check all required arguments:
```python
if not args.get("path"):
    return {"error": "missing required argument: path"}
```

### Path Handling

Normalize paths:
```python
path = args.get("path", "")
if not path.startswith("res://") and not path.startswith("/"):
    path = "res://" + path
```

### Output Size

Keep output reasonable (<1MB). For large results, provide summaries:
```json
{
  "total_lines": 10000,
  "summary": "First 100 lines...",
  "truncated": true
}
```

### Timeouts

External tools have 30s timeout (configurable). Make sure your tool:
- Completes quickly
- Handles partial results
- Can be interrupted

### Testing

Test tools independently:
```bash
echo '{"path":"test.txt"}' | ./tools/my_tool.py
```

---

## Tool Configuration Options

### External Tool Config

```json
{
  "name": "tool_name",
  "description": "What it does. Args: arg1 (type), arg2 (type).",
  "type": "external",
  "command": "./tools/tool_script",
  
  // Optional: override global timeout
  "timeout_seconds": 60
}
```

### GDScript Tool Config

```json
{
  "name": "tool_name",
  "description": "What it does.",
  "type": "gdscript",
  "script": "./tools/tool.gd",
  
  // Optional: use different Godot binary
  "godot_bin": "/opt/godot4/godot"
}
```

---

## Examples Gallery

See the `tools/` directory for complete examples:

- `scene_deep_inspect.gd` - Deep scene parsing with ResourceLoader
- `scan_dependencies.gd` - Find res:// references with RegEx
- `scene_inspect.py` - Python text-based scene parser

---

For more info, see [API-REFERENCE.md](API-REFERENCE.md)
