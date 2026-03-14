# validate_scene.gd
# Validates a scene file for common issues (missing scripts, broken references)
extends "res://tools/mcp_tool_base.gd"

func _run(args: Dictionary) -> Dictionary:
    var path: String = args.get("path", "")
    if path.is_empty():
        return {"error": "missing 'path' argument"}
    
    if not path.begins_with("res://"):
        path = "res://" + path
    
    var warnings := []
    var errors := []
    
    if not ResourceLoader.exists(path):
        return {"error": "scene not found: " + path}
    
    var scene := ResourceLoader.load(path, "PackedScene")
    if scene == null:
        return {"error": "could not load as PackedScene"}
    
    var state := scene.get_state()
    
    # Check for missing scripts
    for i in range(state.get_node_count()):
        for j in range(state.get_node_property_count(i)):
            var prop_name := state.get_node_property_name(i, j)
            if prop_name == "script":
                var script_path = state.get_node_property_value(i, j)
                if script_path is Resource:
                    script_path = script_path.resource_path
                if script_path and not ResourceLoader.exists(str(script_path)):
                    errors.append({
                        "type": "missing_script",
                        "node": state.get_node_name(i),
                        "path": str(script_path)
                    })
    
    # Check external resources
    for i in range(state.get_connection_count()):
        var conn := state.get_connection_source(i)
        # Could check if signal sources still exist, etc.
    
    return {
        "path": path,
        "valid": errors.is_empty(),
        "warnings": warnings,
        "errors": errors,
        "node_count": state.get_node_count()
    }
