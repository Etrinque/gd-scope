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

// ──────────────────────────────────────────────────────────
// Ollama embedding client
// ──────────────────────────────────────────────────────────

// ollamaEmbedRequest matches the /api/embeddings endpoint schema.
// Note: this is NOT the OpenAI-compatible /api/embed endpoint.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"` // Ollama uses "prompt", not "input"
}

type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"` // top-level array, not nested under "data"
}

// embed calls the Ollama local API and returns a float64 embedding vector.
// Returns a clear error if Ollama is unreachable rather than silently returning nil.
func (r *Registry) embed(text string) ([]float64, error) {
	if r.cfg.OllamaURL == "" {
		return nil, fmt.Errorf("embed: OllamaURL not configured")
	}

	reqBody, _ := json.Marshal(ollamaEmbedRequest{
		Model:  r.cfg.EmbedModel,
		Prompt: text,
	})

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(r.cfg.OllamaURL+"/api/embeddings", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("embed: ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: ollama returned HTTP %d", resp.StatusCode)
	}

	var out ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("embed: got empty embedding from ollama (model=%s)", r.cfg.EmbedModel)
	}
	return out.Embedding, nil
}

// ──────────────────────────────────────────────────────────
// Cosine similarity
// ──────────────────────────────────────────────────────────

func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// ──────────────────────────────────────────────────────────
// index_project
// ──────────────────────────────────────────────────────────

// indexProject walks the project, reads scripts and scenes, and embeds them.
// The resulting vectors are stored in-memory for semantic_search.
func (r *Registry) indexProject(ctx context.Context, _ map[string]any) (any, error) {
	if r.cfg.OllamaURL == "" {
		return nil, fmt.Errorf("index_project: OllamaURL not configured")
	}

	var toIndex []struct{ key, text string }

	// Collect scripts
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
		toIndex = append(toIndex, struct{ key, text string }{rel, string(data)})
		return nil
	})

	var indexed int
	var entries []VectorEntry

	for _, item := range toIndex {
		// Check for context cancellation between embedding calls.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		vec, err := r.embed(item.text)
		if err != nil {
			log.Printf("index_project: embed %s: %v", item.key, err)
			continue
		}
		entries = append(entries, VectorEntry{
			Key:       item.key,
			Text:      item.text[:min(len(item.text), 200)], // store a preview only
			Embedding: vec,
		})
		indexed++
	}

	r.mu.Lock()
	r.vectors = entries
	r.mu.Unlock()

	return map[string]any{
		"indexed": indexed,
		"skipped": len(toIndex) - indexed,
		"total":   len(toIndex),
	}, nil
}

// ──────────────────────────────────────────────────────────
// semantic_search
// ──────────────────────────────────────────────────────────

type searchHit struct {
	Key     string  `json:"path"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

// semanticSearch embeds the query and returns the top-k most similar indexed entries.
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
	r.mu.RUnlock()

	if vecCount == 0 {
		return nil, fmt.Errorf("semantic_search: no indexed content - call index_project first")
	}

	qv, err := r.embed(query)
	if err != nil {
		return nil, fmt.Errorf("semantic_search: embed query: %w", err)
	}

	r.mu.RLock()
	hits := make([]searchHit, 0, len(r.vectors))
	for _, v := range r.vectors {
		hits = append(hits, searchHit{
			Key:     v.Key,
			Score:   cosine(qv, v.Embedding),
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
