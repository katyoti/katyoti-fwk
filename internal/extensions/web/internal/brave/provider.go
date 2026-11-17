// SPDX-License-Identifier: Apache-2.0

// Package brave implements the Brave Search-backed search provider.
package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

const defaultEndpoint = "https://api.search.brave.com/res/v1/web/search"

// Provider implements web_search via the Brave Search API.
type Provider struct {
	apiKey   string
	timeout  time.Duration
	count    int
	client   *http.Client
	endpoint string // overridable for tests; default: defaultEndpoint
}

// NewProvider creates a Brave Search provider from config.
func NewProvider(cfg webapi.BraveConfig) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("brave: api_key is required (set BRAVE_SEARCH_API_KEY env var)")
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	count := cfg.ResultCount
	if count <= 0 {
		count = 10
	}
	return &Provider{
		apiKey:   cfg.APIKey,
		timeout:  timeout,
		count:    count,
		client:   &http.Client{Timeout: timeout},
		endpoint: defaultEndpoint,
	}, nil
}

// Search performs a web search via Brave Search API.
func (p *Provider) Search(ctx context.Context, in webapi.SearchInput) (webapi.SearchOutput, error) {
	if in.Query == "" {
		return webapi.SearchOutput{}, fmt.Errorf("brave: query is required")
	}

	count := in.Count
	if count <= 0 {
		count = p.count
	}

	// Build request URL.
	u, _ := url.Parse(p.endpoint)
	q := u.Query()
	q.Set("q", in.Query)
	q.Set("count", strconv.Itoa(count))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("brave: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("brave: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return webapi.SearchOutput{}, fmt.Errorf("brave: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("brave: decode response: %w", err)
	}

	results := make([]webapi.SearchResult, 0, len(raw.Web.Results))
	for _, r := range raw.Web.Results {
		results = append(results, webapi.SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}

	return webapi.SearchOutput{Results: results}, nil
}

// braveResponse is the raw JSON response from the Brave Search API.
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}
