// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// httpEmbedder is the one plain-HTTP client for every OpenAI-shaped
// embeddings endpoint (OpenAI, Foundry, Azure OpenAI, OpenAI-compatible).
type httpEmbedder struct {
	p      resolvedProfile
	key    string
	client *http.Client
}

func (e *httpEmbedder) Name() string   { return e.p.name }
func (e *httpEmbedder) Model() string  { return e.p.model }
func (e *httpEmbedder) Dimension() int { return e.p.dimensions }

// Embed sends the texts in batches of the profile's batch_size and returns
// one vector per text, in the order of texts.
func (e *httpEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += e.p.batchSize {
		end := min(start+e.p.batchSize, len(texts))
		vectors, err := e.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vectors...)
	}
	return out, nil
}

func (e *httpEmbedder) errorf(format string, args ...any) error {
	return fmt.Errorf("embedding profile %q (%s): %s", e.p.name, e.p.typ, fmt.Sprintf(format, args...))
}

func (e *httpEmbedder) embedBatch(ctx context.Context, batch []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Input: batch, Model: e.p.model})
	if err != nil {
		return nil, e.errorf("encode request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.p.url, bytes.NewReader(body))
	if err != nil {
		return nil, e.errorf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.p.auth == AuthAPIKey {
		req.Header.Set("api-key", e.key)
	} else {
		req.Header.Set("Authorization", "Bearer "+e.key)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, e.errorf("%v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, e.errorf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, e.errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result embedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, e.errorf("decode response: %v", err)
	}
	if len(result.Data) != len(batch) {
		return nil, e.errorf("%d vectors returned for %d texts", len(result.Data), len(batch))
	}
	// The response carries each vector's input index; place it there when
	// the indices are a permutation of the batch, else keep the order sent.
	vectors := make([][]float32, len(batch))
	byIndex := true
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(batch) || vectors[d.Index] != nil {
			byIndex = false
			break
		}
		vectors[d.Index] = d.Embedding
	}
	if !byIndex {
		for i, d := range result.Data {
			vectors[i] = d.Embedding
		}
	}
	// A vector of another length than the profile declares would be indexed
	// under a false dimension, and the re-index guard trusts the declared one.
	for _, v := range vectors {
		if len(v) != e.p.dimensions {
			return nil, e.errorf("the endpoint returned vectors of %d dimensions, the profile declares %d: set its dimensions to %d", len(v), e.p.dimensions, len(v))
		}
	}
	return vectors, nil
}

type embedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}
