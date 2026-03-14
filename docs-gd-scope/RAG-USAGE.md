# RAG (Retrieval Augmented Generation) Usage

## What is RAG?

RAG automatically finds relevant code in your project and shows it to the AI when you ask questions.

**Without RAG:**
```
You: "How does my player take damage?"
AI: "Typically in Godot, you'd use a health variable..."
```
Generic answer.

**With RAG:**
```
You: "How does my player take damage?"
AI: "In your project (player.gd line 45), you have:
     func take_damage(amount):
         health -= amount
         if health <= 0: die()"
```
Specific to YOUR code.

---

## Setup

### 1. Enable RAG

Edit `mcp.json`:
```json
{
  "ollama_url": "http://127.0.0.1:11434",
  "rag": {
    "enabled": true,
    "top_k": 3,
    "auto_detect": true
  }
}
```

### 2. Index Your Project

```bash
curl -X POST http://localhost:3333/mcp/v1/tools/index_project -d '{}'
```

This creates embeddings of your code (takes 1-2 minutes for medium projects).

### 3. Ask Questions

```bash
curl -X POST http://localhost:3333/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3",
    "messages": [
      {"role": "user", "content": "How does my player move?"}
    ]
  }'
```

AI will automatically find and reference your movement code!

---

## Configuration

### `rag.enabled` (boolean)
- `true`: RAG is active
- `false`: Disabled (default)

### `rag.top_k` (integer)
- Number of code chunks to retrieve
- Default: `3`
- Range: 1-10
- Higher = more context but slower

### `rag.auto_detect` (boolean)
- `true`: Smart detection (only RAG for project-specific questions)
- `false`: RAG on every query
- Recommended: `true`

---

## Query Examples

### Project-Specific (RAG Activates)
✅ "How does my player take damage?"
✅ "Where is the enemy AI code?"
✅ "Show me the combat system"
✅ "Find code related to jumping"
✅ "What calls the heal function?"

### General (RAG Skips)
❌ "What is a good combat pattern?"
❌ "How to structure a state machine?"
❌ "Tutorial on signals"
❌ "Best practices for Godot"

---

## Logs

Watch for these in server logs:

```
INFO: RAG triggered for query: How does my player...
INFO: Injected RAG context (3 results)
```

Or:

```
INFO: RAG skipped (general question detected)
```

---

## Performance

**Indexing:**
- Small project (<100 files): ~30 seconds
- Medium project (100-500 files): ~2 minutes
- Large project (500+ files): ~5 minutes

**Query time:**
- With RAG: +200ms (semantic search)
- Without RAG: baseline

**Memory:**
- ~1KB per file indexed
- 1000 files = ~1MB RAM

---

## Troubleshooting

**"RAG search failed"**
- Index not built yet → Run `index_project` tool
- Ollama not running → Start Ollama
- Wrong embed model → Pull `nomic-embed-text`

**RAG not triggering**
- Check `rag.enabled: true` in config
- Check `ollama_url` configured
- Query might be too general (check logs)

**Wrong code retrieved**
- Try higher `top_k` (5 or 7)
- Re-index after major code changes
- Query might be ambiguous

---

## Best Practices

1. **Index after changes:** Re-run index_project when you change lots of code
2. **Specific queries:** "How does player.gd handle damage" better than "damage?"
3. **Check logs:** See what RAG is doing
4. **Adjust top_k:** More context = better answers but slower

---

## How It Works

```
1. You ask: "How does damage work?"
   ↓
2. Query → embedding vector
   ↓
3. Search vector database
   ↓
4. Find 3 most similar code chunks
   ↓
5. Inject chunks into AI context
   ↓
6. AI answers with YOUR code
```

---

## Advanced: Manual Control

Disable auto_detect to control RAG manually:

```json
{
  "rag": {
    "enabled": true,
    "top_k": 3,
    "auto_detect": false
  }
}
```

Now RAG runs on EVERY query. Use for:
- Debugging why RAG isn't triggering
- Forcing context injection
- Experimentation

---

## Updating Docs

Download official Godot docs:

```bash
cd scripts
./download_godot_docs.sh
```

This populates `docs/` with official documentation.

