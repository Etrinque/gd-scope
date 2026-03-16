// tools_search.go — semantic (vector) search over the Godot project.
//
// # Overview
//
// This file implements two built-in tools:
//
//   - index_project: walks .gd / .cs / .tscn files, generates embeddings via
//     Ollama's /api/embed, and stores the result in Registry.vectors (in-memory).
//
//   - semantic_search: embeds a natural-language query and returns the top-k
//     most similar indexed documents by cosine similarity.
//
// Both tools require OllamaURL to be configured; they degrade gracefully
// (returning a descriptive error) when Ollama is unreachable.
//
// # Ollama embed API
//
// gd-scope uses the current /api/embed endpoint (introduced in Ollama 0.1.26).
// Do NOT use the legacy /api/embeddings endpoint — it is deprecated and returns
// a different response shape.
//
//	Endpoint:  POST /api/embed
//	Request:   {"model": "nomic-embed-text", "input": "text to embed"}
//	Response:  {"embeddings": [[0.1, 0.2, ...]], "model": "..."}
//
// Key differences from the old /api/embeddings endpoint:
//   - Request field is "input" (not "prompt")
//   - Response field is "embeddings" plural, and it is an array of arrays
//     (to support batch input); for a single string input the result is
//     always a one-element outer array: embeddings[0] is the vector.
//
// # Input length
//
// Both nomic-embed-text and mxbai-embed-large have an 8192-token context
// limit. Sending full file content for large files exceeds this limit and
// causes Ollama to return HTTP 500. We truncate all input to maxEmbedChars
// before embedding. The first N characters of a source file are the most
// semantically dense (imports, class declarations, key functions), so
// truncation preserves the most useful signal.
//
// # Model-specific prefixes
//
// nomic-embed-text and mxbai-embed-large are instruction-tuned embedding
// models that expect task-specific prefixes on their inputs. Without the
// correct prefix, retrieval quality degrades significantly (the model treats
// the text as a generic passage rather than a document or query).
//
// Prefix requirements:
//
//	nomic-embed-text   — documents: "search_document: "  queries: "search_query: "
//	mxbai-embed-large  — documents: (none)               queries: "Represent this sentence for searching relevant passages: "
//
// The prefix is applied in embedDoc() / embedQuery() before sending to Ollama.
// Unknown models get no prefix (safe default; correctness is model-specific).
//
// # Model consistency
//
// Embeddings from different models are not comparable — they live in different
// vector spaces with different dimensions. Searching a nomic-embed-text index
// with a query embedded by mxbai-embed-large will silently return garbage.
// We track which model was used to build the index (indexedWithModel) and warn
// in semantic_search when the current EmbedModel differs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ─── Embedding model client ───────────────────────────────────────────────────

// maxEmbedChars is the maximum number of characters sent to the embed model.
//
// nomic-embed-text and mxbai-embed-large both have 8192-token context limits.
// At an average of ~4 characters per token, 6000 characters ≈ 1500 tokens,
// leaving comfortable headroom for the model prefix and any tokenization
// overhead. Exceeding the context limit causes Ollama to return HTTP 500.
const maxEmbedChars = 6000

// indexedWithModel records which EmbedModel was active the last time
// index_project completed successfully. Protected by r.mu (same lock as
// r.vectors, since both fields describe the same index state).
//
// This is a package-level var rather than a Registry field to avoid changing
// registry.go. Registry is the only consumer in practice.
var indexedWithModel string

// ollamaEmbedRequest is the JSON body for POST /api/embed.
//
// The Go field name matches the semantic meaning (Input); the json tag matches
// the Ollama API wire format. Do not rename the json tag.
type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"` // Ollama /api/embed uses "input", not "prompt"
}

// ollamaEmbedResponse is the decoded body from POST /api/embed.
//
// Ollama returns embeddings as an array of arrays to support batch input.
// For a single-string request, Embeddings always has exactly one element.
//
// Old /api/embeddings (deprecated) returned {"embedding": [...]}.
// New /api/embed returns {"embeddings": [[...]]}.
// We decode only the new format; the old endpoint is not used.
type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"` // outer: one per input; inner: the vector
}

// embed sends text to Ollama's /api/embed and returns the embedding vector.
//
// The caller is responsible for applying any model-specific prefix and for
// truncating the text to maxEmbedChars before calling embed. This function
// is the raw transport layer; use embedDoc or embedQuery instead.
func (r *Registry) embed(text string) ([]float64, error) {
	if r.cfg.OllamaURL == "" {
		return nil, fmt.Errorf("embed: OllamaURL not configured")
	}

	reqBody, err := json.Marshal(ollamaEmbedRequest{
		Model: r.cfg.EmbedModel,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embed: marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(r.cfg.OllamaURL+"/api/embed", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("embed: ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the error body so the log shows the actual Ollama error message
		// (e.g. "context length exceeded") rather than just the status code.
		var errBody struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error != "" {
			return nil, fmt.Errorf("embed: ollama returned HTTP %d: %s", resp.StatusCode, errBody.Error)
		}
		return nil, fmt.Errorf("embed: ollama returned HTTP %d", resp.StatusCode)
	}

	var out ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}
	// Embeddings is a [][]float64; for single-string input it always has exactly
	// one element. An empty outer array means something went wrong server-side.
	if len(out.Embeddings) == 0 || len(out.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("embed: empty embeddings in response (model=%s)", r.cfg.EmbedModel)
	}
	return out.Embeddings[0], nil
}

// embedDoc embeds a document (file content) with the appropriate model-specific
// prefix and input truncation. Use this when indexing project files.
func (r *Registry) embedDoc(text string) ([]float64, error) {
	prefix := docPrefix(r.cfg.EmbedModel)
	text = truncate(text, maxEmbedChars-len(prefix))
	return r.embed(prefix + text)
}

// embedQuery embeds a search query with the appropriate model-specific prefix.
// Use this in semantic_search when embedding the user's query.
func (r *Registry) embedQuery(text string) ([]float64, error) {
	prefix := queryPrefix(r.cfg.EmbedModel)
	text = truncate(text, maxEmbedChars-len(prefix))
	return r.embed(prefix + text)
}

// docPrefix returns the instruction prefix to prepend when embedding a document
// (i.e. a project file) for the given embed model.
//
// Instruction-tuned models require task-specific prefixes to activate their
// retrieval behaviour. Without the correct prefix, recall degrades significantly.
func docPrefix(model string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(model), "nomic-embed-text"):
		// nomic-embed-text v1.5 instruction format:
		// https://huggingface.co/nomic-ai/nomic-embed-text-v1.5
		return "search_document: "
	default:
		// mxbai-embed-large and most other models do not use document prefixes.
		return ""
	}
}

// queryPrefix returns the instruction prefix to prepend when embedding a search
// query for the given embed model.
func queryPrefix(model string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(model), "nomic-embed-text"):
		return "search_query: "
	case strings.HasPrefix(strings.ToLower(model), "mxbai-embed-large"):
		// mxbai-embed-large retrieval prefix:
		// https://huggingface.co/mixedbread-ai/mxbai-embed-large-v1
		return "Represent this sentence for searching relevant passages: "
	default:
		return ""
	}
}

// truncate returns text truncated to at most n characters.
// Truncation occurs on a character boundary, not a token boundary, which is
// a reasonable approximation since embed models use byte-level tokenization.
func truncate(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[:n]
}

// ─── Vector similarity ────────────────────────────────────────────────────────

// cosine returns the cosine similarity between vectors a and b, in the range
// [-1, 1] where 1 means identical direction and 0 means orthogonal.
//
// Both nomic-embed-text and mxbai-embed-large return L2-normalized (unit)
// vectors, which means ||a|| = ||b|| = 1 and cosine similarity reduces to a
// plain dot product. The normalization step below is still computed for
// correctness with other models that may not normalize their output.
//
// Returns 0 for empty or mismatched-length vectors rather than panicking.
// A dimension mismatch at search time usually indicates that index_project
// was run with a different EmbedModel than the current one; re-index to fix.
func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	// math.Sqrt(na * nb) is one sqrt instead of two — equivalent result.
	denom := math.Sqrt(na * nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// ─── index_project ────────────────────────────────────────────────────────────

// indexProject walks the project directory, embeds all .gd, .cs, and .tscn
// files, and stores the resulting vectors in r.vectors for semantic_search.
//
// Each file is embedded as a document using embedDoc(), which applies the
// model-specific prefix and truncates to maxEmbedChars. Files that exceed
// Ollama's context window (previously a common HTTP 500 cause) are now handled
// safely by the truncation step.
//
// The model used for indexing is recorded in indexedWithModel. If the user
// changes EmbedModel in the config and calls semantic_search without
// re-indexing, a warning is logged because query and document vectors will be
// from different embedding spaces and results will be meaningless.
//
// The operation is O(n_files) Ollama calls and may take tens of seconds for
// large projects. Context cancellation is checked between files.
func (r *Registry) indexProject(ctx context.Context, _ map[string]any) (any, error) {
	if r.cfg.OllamaURL == "" {
		return nil, fmt.Errorf("index_project: OllamaURL not configured — set ollama_url in mcp.json")
	}

	// Collect all indexable files first so we can report total/skipped counts.
	type fileItem struct{ rel, text string }
	var toIndex []fileItem

	filepath.WalkDir(r.cfg.ProjectRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".gd" && ext != ".cs" && ext != ".tscn" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			log.Printf("index_project: skip %s: %v", p, err)
			return nil
		}
		rel, _ := filepath.Rel(r.cfg.ProjectRoot, p)
		toIndex = append(toIndex, fileItem{rel, string(data)})
		return nil
	})

	var entries []VectorEntry
	skipped := 0

	for _, item := range toIndex {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// embedDoc applies model-specific prefix and truncates to maxEmbedChars
		// so we never exceed the embed model's context window.
		vec, err := r.embedDoc(item.text)
		if err != nil {
			log.Printf("index_project: embed %s: %v", item.rel, err)
			skipped++
			continue
		}

		entries = append(entries, VectorEntry{
			Key:  item.rel,
			Text: truncate(item.text, 200), // preview for search result display only
			Embedding: vec,
		})
	}

	// Atomically replace the index and record which model was used.
	r.mu.Lock()
	r.vectors = entries
	indexedWithModel = r.cfg.EmbedModel
	r.mu.Unlock()

	indexed := len(entries)
	log.Printf("index_project: indexed %d files, skipped %d, model=%s",
		indexed, skipped, r.cfg.EmbedModel)

	return map[string]any{
		"indexed": indexed,
		"skipped": skipped,
		"total":   len(toIndex),
		"model":   r.cfg.EmbedModel,
	}, nil
}

// ─── semantic_search ─────────────────────────────────────────────────────────

// searchHit is one result from semantic_search.
type searchHit struct {
	Key     string  `json:"path"`    // project-relative file path
	Score   float64 `json:"score"`   // cosine similarity [0, 1]
	Preview string  `json:"preview"` // first 200 chars of the file
}

// semanticSearch embeds the query using embedQuery() and returns the top-k
// indexed files ranked by cosine similarity.
//
// Preconditions:
//   - index_project must have been called at least once since startup
//     (vectors are not persisted across restarts).
//   - The same EmbedModel must be active as when index_project ran.
//     A model change invalidates the index; re-index with the new model.
func (r *Registry) semanticSearch(_ context.Context, a map[string]any) (any, error) {
	query, err := requireString(a, "query")
	if err != nil {
		return nil, err
	}

	topK := 5
	if k, ok := a["top_k"].(float64); ok && k > 0 {
		topK = int(k)
	}

	r.mu.RLock()
	vecCount := len(r.vectors)
	cachedModel := indexedWithModel
	r.mu.RUnlock()

	if vecCount == 0 {
		return nil, fmt.Errorf("semantic_search: no indexed content — call index_project first")
	}

	// Warn if the active embed model differs from the one used to build the index.
	// Vectors from different models live in different spaces; results would be nonsense.
	if cachedModel != "" && cachedModel != r.cfg.EmbedModel {
		log.Printf("WARN: semantic_search: current EmbedModel (%s) differs from index model (%s) — re-run index_project",
			r.cfg.EmbedModel, cachedModel)
	}

	// Embed the query with the model-specific retrieval prefix.
	qv, err := r.embedQuery(query)
	if err != nil {
		return nil, fmt.Errorf("semantic_search: embed query: %w", err)
	}

	r.mu.RLock()
	hits := make([]searchHit, 0, len(r.vectors))
	for _, v := range r.vectors {
		score := cosine(qv, v.Embedding)
		if score == 0 && len(qv) != len(v.Embedding) {
			// Dimension mismatch — index was built with a different model.
			// Log once and break rather than flooding with per-entry warnings.
			log.Printf("WARN: semantic_search: query vector dim=%d but index vector dim=%d — re-run index_project with model=%s",
				len(qv), len(v.Embedding), r.cfg.EmbedModel)
			r.mu.RUnlock()
			return nil, fmt.Errorf("semantic_search: embedding dimension mismatch — re-run index_project")
		}
		hits = append(hits, searchHit{
			Key:     v.Key,
			Score:   score,
			Preview: v.Text,
		})
	}
	r.mu.RUnlock()

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	if topK < len(hits) {
		hits = hits[:topK]
	}

	return map[string]any{
		"query":   query,
		"results": hits,
		"count":   len(hits),
	}, nil
}
