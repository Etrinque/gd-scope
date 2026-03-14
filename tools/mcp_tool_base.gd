# mcp_tool_base.gd
# Base class for all GDScript MCP tools.
#
# Inherit from this and override _run(args: Dictionary) -> Dictionary.
# The base class handles all the JSON I/O and error wrapping.
#
# Usage:
#   extends "res://../../tools/mcp_tool_base.gd"  (adjust path as needed)
#   func _run(args: Dictionary) -> Dictionary:
#       return { "result": "hello" }
#
extends SceneTree

func _initialize() -> void:
	var user_args := OS.get_cmdline_user_args()
	if user_args.is_empty():
		_exit_error("no arguments provided - expected JSON as first user arg")
		return

	var json := JSON.new()
	var parse_err := json.parse(user_args[0])
	if parse_err != OK:
		_exit_error("invalid JSON input: " + json.get_error_message())
		return

	var args: Dictionary = json.get_data()
	var result := _run(args)
	print(JSON.stringify(result))
	quit(0)

# Override this in your tool.
func _run(_args: Dictionary) -> Dictionary:
	return {"error": "_run() not implemented"}

func _exit_error(msg: String) -> void:
	print(JSON.stringify({"error": msg}))
	quit(1)
