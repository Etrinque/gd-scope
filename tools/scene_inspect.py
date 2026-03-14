#!/usr/bin/env python3
"""
scene_inspect.py - Example external tool for gd-scope.
Contract: JSON on stdin → JSON on stdout. Exit 0 = success, 1 = error.
"""
import json, sys, re

args = json.load(sys.stdin)
path = args.get("path", "")
if not path:
    print(json.dumps({"error": "missing 'path' argument"}))
    sys.exit(1)

try:
    content = open(path).read()
except OSError as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(1)

print(json.dumps({
    "path": path,
    "node_names": re.findall(r'name="([^"]+)"', content),
    "node_types": list(set(re.findall(r'type="([^"]+)"', content))),
    "line_count": content.count("\n"),
}))
