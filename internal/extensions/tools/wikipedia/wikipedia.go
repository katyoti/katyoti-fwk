// SPDX-License-Identifier: Apache-2.0

// Package wikipedia implements the wikipedia agent tool for searching
// and reading Wikipedia articles.
//
// Uses the Wikipedia REST API directly (no external SDK needed).
package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

const (
	apiEndpoint = "https://en.wikipedia.org/w/api.php"
	userAgent   = "ktw-agents/1.0 (https://github.com/katyoti/katyoti-fwk)"
)

// WikiInput is the tool's input schema.
type WikiInput struct {
	// Action selects the operation. Default: "search".
	// Options: "search" (find pages), "summary" (get page summary).
	Action string `json:"action,omitempty" jsonschema:"Action: 'search' (default) or 'summary'"`

	// Query is the search query or page title. Required.
	Query string `json:"query" jsonschema:"Search query or page title" required:"true"`

	// Language is the Wikipedia language code. Default: "en".
	Language string `json:"language,omitempty" jsonschema:"Wikipedia language code (e.g. 'en', 'fr', 'de'). Default: 'en'"`

	// MaxResults is the max number of search results. Default: 5. Only for action=search.
	MaxResults *int `json:"max_results,omitempty" jsonschema:"Max search results (search action only). Default: 5"`
}

// WikiOutput is the tool's output.
type WikiOutput struct {
	// Search results (action=search)
	Results []SearchResult `json:"results,omitempty" jsonschema:"Search results."`

	// Summary (action=summary)
	Title   string `json:"title,omitempty"   jsonschema:"Article title."`
	Summary string `json:"summary,omitempty" jsonschema:"Article summary text."`
	URL     string `json:"url,omitempty"     jsonschema:"Article URL."`
}

// SearchResult is a Wikipedia search result.
type SearchResult struct {
	Title   string `json:"title"             jsonschema:"Article title."`
	Snippet string `json:"snippet,omitempty" jsonschema:"Text snippet."`
	URL     string `json:"url"               jsonschema:"Article URL."`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// NewWikipediaTool creates the wikipedia tool.
func NewWikipediaTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"wikipedia",
		toolDescription,
		executeWikipedia,
		toolsapi.WithReadOnly(),
		toolsapi.WithCancelOnInterrupt(),
		toolsapi.WithDescriber(describeWikipedia),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📖", Label: "Wiki", Color: "#82AAFF", Template: "{{.query}}"}),
	)
}

func executeWikipedia(ctx context.Context, in WikiInput) (WikiOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return WikiOutput{}, fmt.Errorf("wikipedia: query is required")
	}

	action := in.Action
	if action == "" {
		action = "search"
	}

	lang := in.Language
	if lang == "" {
		lang = "en"
	}
	endpoint := fmt.Sprintf("https://%s.wikipedia.org/w/api.php", lang)

	switch action {
	case "search":
		return doSearch(ctx, endpoint, in)
	case "summary":
		return doSummary(ctx, endpoint, lang, in)
	default:
		return WikiOutput{}, fmt.Errorf("wikipedia: unknown action %q (use: search, summary)", action)
	}
}

func doSearch(ctx context.Context, endpoint string, in WikiInput) (WikiOutput, error) {
	maxResults := 5
	if in.MaxResults != nil && *in.MaxResults > 0 {
		maxResults = *in.MaxResults
	}

	params := url.Values{
		"action":   {"query"},
		"list":     {"search"},
		"srsearch": {in.Query},
		"srlimit":  {fmt.Sprintf("%d", maxResults)},
		"format":   {"json"},
		"utf8":     {"1"},
	}

	body, err := wikiGet(ctx, endpoint, params)
	if err != nil {
		return WikiOutput{}, err
	}

	var resp struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
				PageID  int    `json:"pageid"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return WikiOutput{}, fmt.Errorf("wikipedia: decode response: %w", err)
	}

	lang := in.Language
	if lang == "" {
		lang = "en"
	}

	results := make([]SearchResult, len(resp.Query.Search))
	for i, r := range resp.Query.Search {
		// Strip HTML tags from snippet.
		snippet := stripHTML(r.Snippet)
		results[i] = SearchResult{
			Title:   r.Title,
			Snippet: snippet,
			URL:     fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(strings.ReplaceAll(r.Title, " ", "_"))),
		}
	}

	return WikiOutput{Results: results}, nil
}

func doSummary(ctx context.Context, _ string, lang string, in WikiInput) (WikiOutput, error) {
	// Use the REST API for summaries — cleaner than the action API.
	title := strings.ReplaceAll(strings.TrimSpace(in.Query), " ", "_")
	summaryURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", lang, url.PathEscape(title))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, nil)
	if err != nil {
		return WikiOutput{}, fmt.Errorf("wikipedia: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return WikiOutput{}, fmt.Errorf("wikipedia: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return WikiOutput{}, fmt.Errorf("wikipedia: no article found for %q — try using action=search first", in.Query)
	}
	if resp.StatusCode != http.StatusOK {
		return WikiOutput{}, fmt.Errorf("wikipedia: HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return WikiOutput{}, fmt.Errorf("wikipedia: decode response: %w", err)
	}

	return WikiOutput{
		Title:   raw.Title,
		Summary: raw.Extract,
		URL:     raw.ContentURLs.Desktop.Page,
	}, nil
}

func wikiGet(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	u, _ := url.Parse(endpoint)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("wikipedia: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikipedia: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikipedia: HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// stripHTML removes basic HTML tags from a string (Wikipedia snippets have <span> etc.).
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func describeWikipedia(raw json.RawMessage) string {
	var in WikiInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "wikipedia <invalid>"
	}
	action := in.Action
	if action == "" {
		action = "search"
	}
	q := in.Query
	if len(q) > 50 {
		q = q[:50] + "..."
	}
	return fmt.Sprintf("wikipedia %s query=%q", action, q)
}

const toolDescription = `Searches and reads Wikipedia articles.

Supports two actions via the 'action' parameter:

- **search** (default): Find Wikipedia pages matching a query. Returns titles, snippets, and URLs.
- **summary**: Get the summary of a specific Wikipedia article by title.

# Instructions
- Use action=search to find relevant articles, then action=summary to read one.
- The 'language' parameter lets you search in other Wikipedia editions (e.g. 'fr', 'de', 'es').
- Summaries are the introductory section of the article — typically 1-3 paragraphs of factual information.
`
