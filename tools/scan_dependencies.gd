# scan_dependencies.gd
# Scans GDScript files for res:// references to build a dependency map.
# Useful for AI tools that need to understand what resources a script uses.
#
# Args:   { "path": "res://scripts/Player.gd" }  (or omit for whole project)
# Returns { "dependencies": [ { "from": ..., "to": ... } ] }
#
extends "res://tools/mcp_tool_base.gd"

const RES_PATTERN := "res://[a-zA-Z0-9_./@-]+"

func _run(args: Dictionary) -> Dictionary:
	var target_path: String = args.get("path", "")
	var dependencies := []

	if not target_path.is_empty():
		_scan_file(target_path, dependencies)
	else:
		_scan_dir("res://", dependencies)

	# Deduplicate
	var seen := {}
	var unique := []
	for dep in dependencies:
		var key := dep["from"] + "|" + dep["to"]
		if not seen.has(key):
			seen[key] = true
			unique.append(dep)

	return {
		"scanned": target_path if not target_path.is_empty() else "entire project",
		"dependency_count": unique.size(),
		"dependencies": unique,
	}

func _scan_dir(dir_path: String, out: Array) -> void:
	var dir := DirAccess.open(dir_path)
	if dir == null:
		return
	dir.list_dir_begin()
	var name := dir.get_next()
	while name != "":
		if not name.begins_with("."):
			var full := dir_path.path_join(name)
			if dir.current_is_dir():
				_scan_dir(full, out)
			elif name.ends_with(".gd") or name.ends_with(".tscn"):
				_scan_file(full, out)
		name = dir.get_next()

func _scan_file(path: String, out: Array) -> void:
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		return
	var content := file.get_as_text()
	var regex := RegEx.new()
	regex.compile(RES_PATTERN)
	for match in regex.search_all(content):
		var ref := match.get_string()
		if ref != path:
			out.append({"from": path, "to": ref})
