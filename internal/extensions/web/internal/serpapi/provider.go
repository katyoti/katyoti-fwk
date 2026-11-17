// SPDX-License-Identifier: Apache-2.0

// Package serpapi implements the SerpAPI-backed search provider.
// Uses the official Go SDK: github.com/serpapi/serpapi-golang
package serpapi

import (
	"context"
	"fmt"
	"strconv"

	"github.com/serpapi/serpapi-golang"

	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

// Provider implements web_search via the SerpAPI SDK.
type Provider struct {
	client   serpapi.SerpApiClient
	location string
	num      int
}

// NewProvider creates a SerpAPI provider.
func NewProvider(cfg webapi.SerpAPIConfig, location string, numResults int) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("serpapi: api_key is required (set SERP_API_KEY env var)")
	}
	if location == "" {
		location = "France"
	}
	if numResults <= 0 {
		numResults = 5
	}
	setting := serpapi.NewSerpApiClientSetting(cfg.APIKey)
	setting.Engine = "google"
	client := serpapi.NewClient(setting)
	return &Provider{client: client, location: location, num: numResults}, nil
}

// Search performs a web search via SerpAPI.
func (p *Provider) Search(_ context.Context, in webapi.SearchInput) (webapi.SearchOutput, error) {
	if in.Query == "" {
		return webapi.SearchOutput{}, fmt.Errorf("serpapi: query is required")
	}

	num := in.Count
	if num <= 0 {
		num = p.num
	}

	params := map[string]string{
		"q":        in.Query,
		"location": p.location,
		"num":      strconv.Itoa(num),
		"hl":       "fr",
		"gl":       "fr",
	}

	raw, err := p.client.Search(params)
	if err != nil {
		return webapi.SearchOutput{}, fmt.Errorf("serpapi: search failed: %w", err)
	}

	// Parse organic_results from the raw map.
	organicRaw, ok := raw["organic_results"]
	if !ok {
		return webapi.SearchOutput{}, nil
	}

	organics, ok := organicRaw.([]interface{})
	if !ok {
		return webapi.SearchOutput{}, nil
	}

	results := make([]webapi.SearchResult, 0, len(organics))
	for _, item := range organics {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, webapi.SearchResult{
			Title:       strVal(m, "title"),
			URL:         strVal(m, "link"),
			Description: strVal(m, "snippet"),
		})
	}

	return webapi.SearchOutput{Results: results}, nil
}

func strVal(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
