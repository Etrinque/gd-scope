# scene_deep_inspect.gd
# Uses Godot's own scene loader to inspect a .tscn file properly.
# This is what makes GDScript tools valuable - real engine API access.
#
# Args:   { "path": "res://scenes/Player.tscn" }
# Returns node tree with types, groups, and script paths.
#
extends "res://tools/mcp_tool_base.gd"

func _run(args: Dictionary) -> Dictionary:
	var path: String = args.get("path", "")
	if path.is_empty():
		return {"error": "missing 'path' argument"}

	# Normalize: accept filesystem paths and res:// paths
	if not path.begins_with("res://") and not path.begins_with("/"):
		path = "res://" + path

	if not ResourceLoader.exists(path):
		return {"error": "scene not found: " + path}

	var packed: PackedScene = ResourceLoader.load(path, "PackedScene")
	if packed == null:
		return {"error": "could not load as PackedScene: " + path}

	var state := packed.get_state()
	var nodes := []

	for i in range(state.get_node_count()):
		var node_info := {
			"name":   state.get_node_name(i),
			"type":   state.get_node_type(i),
			"path":   str(state.get_node_path(i)),
			"groups": state.get_node_groups(i),
			"properties": {}
		}

		# Extract node properties (position, script, etc.)
		for j in range(state.get_node_property_count(i)):
			var prop_name := state.get_node_property_name(i, j)
			var prop_val  := state.get_node_property_value(i, j)
			# Convert non-serialisable types to strings
			if prop_val is Resource:
				node_info["properties"][prop_name] = prop_val.resource_path
			elif typeof(prop_val) in [TYPE_VECTOR2, TYPE_VECTOR3, TYPE_COLOR, TYPE_RECT2]:
				node_info["properties"][prop_name] = str(prop_val)
			else:
				node_info["properties"][prop_name] = prop_val

		nodes.append(node_info)

	return {
		"path": path,
		"node_count": state.get_node_count(),
		"nodes": nodes,
	}
