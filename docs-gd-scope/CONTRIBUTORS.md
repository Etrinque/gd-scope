# Contributors

gd-scope is built and maintained by the following people.

---

## Core

**Eric Trinque** ([@etrinque](https://github.com/etrinque))
Project creator. Architecture, Go server, Ollama integration, tool system, distribution.

---

## How to contribute

Bug reports, feature requests, and pull requests are welcome.

Before opening a PR, please:
- Run `go vet ./...` and `go test ./...` and confirm both pass
- Keep changes focused. One fix or feature per PR.
- For anything large, open an issue first so we can discuss direction before you invest the time.

External tools are one of the easiest ways to extend gd-scope without touching core Go code. If you build something useful, consider adding it to the `tools/` directory or documenting it in `TOOL-CREATION.md`. **NOTE:** All community tools must pass thorough security audit before being added to the core tools respository. A separate gd-scope/Community/Tools repository may be added as adoption dictates.

See [QUICKSTART.md](github.com/etrinque/docs-gd-scope/QUICKSTART.md) to get the project running locally.

---

## Acknowledgments

- [FlamxGames](https://github.com/FlamxGames) for the [AI Assistant Hub](https://github.com/FlamxGames/godot-ai-assistant-hub) Godot plugin that gd-scope was built to integrate with
- The [Godot Engine](https://godotengine.org) community
- The [Ollama](https://ollama.com) team for making local LLM inference practical
