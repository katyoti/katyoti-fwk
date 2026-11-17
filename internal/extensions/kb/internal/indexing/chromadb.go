// SPDX-License-Identifier: Apache-2.0

// Package indexing implements vector DB strategies for knowledge indexing.
package indexing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// ChromaDBIndexer implements KnowledgeIndexer using ChromaDB's HTTP API.
type ChromaDBIndexer struct {
	BaseURL    string // e.g. "http://localhost:8000"
	collection string
	collID     string // resolved collection UUID
}

// NewChromaDBIndexer creates a ChromaDB indexer from config.
func NewChromaDBIndexer(config map[string]any) (*ChromaDBIndexer, error) {
	baseURL := "http://localhost:8000"
	if config != nil {
		if u, ok := config["url"].(string); ok && u != "" {
			baseURL = u
		}
	}
	return &ChromaDBIndexer{BaseURL: baseURL}, nil
}

func (c *ChromaDBIndexer) Init(ctx context.Context, collection string, dimension int, meta map[string]string) error {
	c.collection = collection

	// Try to get existing collection
	resp, err := c.doJSON(ctx, "GET", "/api/v1/collections/"+collection, nil)
	if err == nil && resp != nil {
		if id, ok := resp["id"].(string); ok {
			c.collID = id
			return nil
		}
	}

	// Create collection
	body := map[string]any{
		"name":     collection,
		"metadata": meta,
	}
	resp, err = c.doJSON(ctx, "POST", "/api/v1/collections", body)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	if id, ok := resp["id"].(string); ok {
		c.collID = id
	}
	return nil
}

func (c *ChromaDBIndexer) Index(ctx context.Context, chunks []kbapi.KnowledgeChunk) error {
	if c.collID == "" {
		return fmt.Errorf("collection not initialized")
	}
	if len(chunks) == 0 {
		return nil
	}

	// ChromaDB upsert — batch up to 100 at a time
	batchSize := 100
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		ids := make([]string, len(batch))
		embeddings := make([][]float32, len(batch))
		documents := make([]string, len(batch))
		metadatas := make([]map[string]string, len(batch))

		for j, ch := range batch {
			ids[j] = ch.ID
			embeddings[j] = ch.Vector
			documents[j] = ch.Content
			metadatas[j] = ch.Metadata
		}

		body := map[string]any{
			"ids":        ids,
			"embeddings": embeddings,
			"documents":  documents,
			"metadatas":  metadatas,
		}

		_, err := c.doJSON(ctx, "POST", "/api/v1/collections/"+c.collID+"/upsert", body)
		if err != nil {
			return fmt.Errorf("upsert batch: %w", err)
		}
	}

	return nil
}

func (c *ChromaDBIndexer) Delete(ctx context.Context, filter kbapi.ChunkFilter) (int, error) {
	if c.collID == "" {
		return 0, fmt.Errorf("collection not initialized")
	}
	where := filterWhere(filter)
	if len(where) == 0 {
		return 0, fmt.Errorf("delete: no filter - it would empty the collection")
	}
	// How many it holds: their ids, nothing else.
	got, err := c.doJSON(ctx, "POST", "/api/v1/collections/"+c.collID+"/get", map[string]any{"where": where, "include": []string{}})
	if err != nil {
		return 0, err
	}
	n := 0
	if ids, ok := got["ids"].([]any); ok {
		n = len(ids)
	}
	if n == 0 {
		return 0, nil
	}
	if _, err := c.doJSON(ctx, "POST", "/api/v1/collections/"+c.collID+"/delete", map[string]any{"where": where}); err != nil {
		return 0, err
	}
	return n, nil
}

func (c *ChromaDBIndexer) Query(ctx context.Context, query kbapi.QueryRequest) ([]kbapi.QueryResult, error) {
	if c.collID == "" {
		return nil, fmt.Errorf("collection not initialized")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}

	body := map[string]any{
		"query_embeddings": [][]float32{query.Vector},
		"n_results":        limit,
		"include":          []string{"documents", "metadatas", "distances"},
	}

	if len(query.Filter) > 0 {
		body["where"] = query.Filter
	}

	resp, err := c.doJSON(ctx, "POST", "/api/v1/collections/"+c.collID+"/query", body)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	var results []kbapi.QueryResult

	// Parse ChromaDB query response
	documents, _ := resp["documents"].([]any)
	distances, _ := resp["distances"].([]any)
	metadatas, _ := resp["metadatas"].([]any)

	if len(documents) > 0 {
		docs, _ := documents[0].([]any)
		dists, _ := distances[0].([]any)
		metas, _ := metadatas[0].([]any)

		for i, doc := range docs {
			text, _ := doc.(string)
			score := 0.0
			if i < len(dists) {
				if d, ok := dists[i].(float64); ok {
					score = 1.0 - d // ChromaDB returns distances, convert to similarity
				}
			}

			var source kbapi.SourceInfo
			if i < len(metas) {
				if m, ok := metas[i].(map[string]any); ok {
					source = sourceInfoFromMeta(m)
				}
			}

			results = append(results, kbapi.QueryResult{
				Content: text,
				Score:   score,
				Source:  source,
			})
		}
	}

	return results, nil
}

func (c *ChromaDBIndexer) CollectionMeta(ctx context.Context, collection string) (map[string]string, error) {
	resp, err := c.doJSON(ctx, "GET", "/api/v1/collections/"+collection, nil)
	if err != nil {
		return nil, err
	}
	meta, _ := resp["metadata"].(map[string]any)
	result := make(map[string]string)
	for k, v := range meta {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result, nil
}

// doJSON makes an HTTP request with JSON body and returns the parsed response.
func (c *ChromaDBIndexer) doJSON(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("chromadb %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &result)
	}
	return result, nil
}

func sourceInfoFromMeta(m map[string]any) kbapi.SourceInfo {
	str := func(key string) string {
		v, _ := m[key].(string)
		return v
	}
	num := func(key string) int {
		if v, ok := m[key].(float64); ok {
			return int(v)
		}
		return 0
	}
	return kbapi.SourceInfo{
		KBName:      str("kb_name"),
		SourcePath:  str("source_path"),
		RemotePath:  str("remote_path"),
		SyncSource:  str("sync_source"),
		Part:        str("part"),
		PartIndex:   num("part_index"),
		ChunkIndex:  num("chunk_index"),
		ExtractedAt: str("extracted_at"),
	}
}
