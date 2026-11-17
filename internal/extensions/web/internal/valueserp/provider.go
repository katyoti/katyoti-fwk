// SPDX-License-Identifier: Apache-2.0

// Package valueserp implements the ValueSERP-backed search provider.
// API docs: https://www.valueserp.com/docs
package valueserp

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

const defaultEndpoint = "https://api.valueserp.com/search"

// Provider implements web_search via the ValueSERP API.
type Provider struct {
	apiKey   string
	location string
	num      int
	client   *http.Client
	endpoint string // overridable for tests
}

// NewProvider creates a ValueSERP provider.
func NewProvider(cfg webapi.ValueSERPConfig, location string, numResults int) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("valueserp: api_key is required (set VALUESERP_API_KEY env var)")
	}
	if location == "" {
		location = "France"
	}
	if numResults <= 0 {
		numResults = 5
	}
	return &Provider{
		apiKey:   cfg.APIKey,
		location: location,
		num:      numResults,
		client:   &http.Client{Timeout: 15 * time.Second},
		endpoint: defaultEndpoint,
	}, nil
}

// Search performs a web search via ValueSERP API.
func (p *Provider) Search(ctx context.Context, in webapi.SearchInput) (webapi.SearchOutput, error) {
	if in.Query == "" {
		return webapi.SearchOutput{}, fmt.Errorf("valueserp: query is required")
	}

	num := in.Count
	if num <= 0 {
		num = p.num
	}

	params := url.Values{
		"api_key":             {p.apiKey},
		"q":                   {in.Query},
		"location":            {p.location},
		"num":                 {strconv.Itoa(num)},
		"include_ai_overview": {"false"},
	}

	u, _ := url.Parse(p.endpoint)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("valueserp: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("valueserp: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return webapi.SearchOutput{}, fmt.Errorf("valueserp: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw valueSERPResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("valueserp: decode response: %w", err)
	}

	results := make([]webapi.SearchResult, 0, len(raw.OrganicResults))
	for _, r := range raw.OrganicResults {
		results = append(results, webapi.SearchResult{
			Title:       r.Title,
			URL:         r.Link,
			Description: r.Snippet,
		})
	}

	return webapi.SearchOutput{Results: results}, nil
}

// valueSERPResponse is the raw JSON response from the ValueSERP API.
type valueSERPResponse struct {
	OrganicResults []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic_results"`
}
