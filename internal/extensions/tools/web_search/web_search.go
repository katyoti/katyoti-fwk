// SPDX-License-Identifier: Apache-2.0

// Package web_search implements the web_search agent tool for web search queries.
//
// Uses WebQueryService (Brave Search provider) to search the web and return
// structured results.
package web_search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

// SearchInput is the tool's input schema.
type SearchInput struct {
	// Query is the search query. Required.
	Query string `json:"query" jsonschema:"The search query" required:"true"`

	// Count is the maximum number of results. Default: from config (typically 10).
	Count *int `json:"count,omitempty" jsonschema:"Maximum number of results to return"`
}

// SearchOutput is the tool's output.
type SearchOutput struct {
	Results []Result `json:"results" jsonschema:"Search results."`
}

// Result is a single search result.
type Result struct {
	Title       string `json:"title"       jsonschema:"Result title."`
	URL         string `json:"url"         jsonschema:"Result URL."`
	Description string `json:"description" jsonschema:"Result description/snippet."`
}

// NewWebSearchTool creates the web_search tool.
func NewWebSearchTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"web-search",
		toolDescription,
		executeWebSearch,
		toolsapi.WithReadOnly(),
		toolsapi.WithCancelOnInterrupt(),
		toolsapi.WithDescriber(describeWebSearch),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🔍", Label: "Search", Color: "#82AAFF", Template: "{{.query}}"}),
	)
}

func executeWebSearch(ctx context.Context, in SearchInput) (SearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return SearchOutput{}, fmt.Errorf("web_search: query is required")
	}

	svc, err := application.UseService[webapi.WebQueryService](ctx)
	if err != nil {
		return SearchOutput{}, fmt.Errorf("web_search: WebQueryService not available: %w", err)
	}

	count := 0
	if in.Count != nil {
		count = *in.Count
	}

	result, err := svc.Search(ctx, webapi.SearchInput{
		Query: in.Query,
		Count: count,
	})
	if err != nil {
		return SearchOutput{}, fmt.Errorf("web_search: %w", err)
	}

	out := SearchOutput{
		Results: make([]Result, len(result.Results)),
	}
	for i, r := range result.Results {
		out.Results[i] = Result{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		}
	}
	return out, nil
}

func describeWebSearch(raw json.RawMessage) string {
	var in SearchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "web_search <invalid>"
	}
	q := in.Query
	if len(q) > 60 {
		q = q[:60] + "..."
	}
	return fmt.Sprintf("web_search query=%q", q)
}

const toolDescription = `Searches the web and returns structured results.

Uses a web search service to find relevant web pages for a given query. Returns titles, URLs, and description snippets.

# Instructions
- Provide a clear, specific search query.
- Results include title, URL, and a description snippet for each match.
- Use this for finding information, documentation, or resources on the web.
- To read the full content of a specific URL, use the web_fetch tool.
`
