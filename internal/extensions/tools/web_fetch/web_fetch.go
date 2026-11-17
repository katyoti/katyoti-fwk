// SPDX-License-Identifier: Apache-2.0

// Package web_fetch implements the web_fetch agent tool for URL content
// extraction, crawling, and site mapping.
//
// Backed by Firecrawl via the shared WebQueryService.
package web_fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

// FetchInput is the tool's input schema.
type FetchInput struct {
	// Action selects the operation. Default: "scrape".
	// Options: "scrape" (single URL), "crawl" (multi-page), "map" (discover URLs), "crawl_status" (check job).
	Action string `json:"action,omitempty" jsonschema:"Action: 'scrape' (default), 'crawl', 'map', or 'crawl_status'"`

	// URL is the target URL. Required for scrape/crawl/map.
	URL string `json:"url,omitempty" jsonschema:"The URL to fetch, crawl, or map"`

	// Formats specifies output formats for scrape/crawl. Default: ["markdown"].
	Formats []string `json:"formats,omitempty" jsonschema:"Output formats (e.g. 'markdown', 'links', 'html'). Default: ['markdown']"`

	// MaxDepth is the crawl depth limit. Only for action=crawl. Default: 10.
	MaxDepth *int `json:"max_depth,omitempty" jsonschema:"Max crawl depth (crawl action only). Default: 10"`

	// Limit is the max pages for crawl or max URLs for map. Default: 5 (crawl), 100 (map).
	Limit *int `json:"limit,omitempty" jsonschema:"Max pages (crawl) or URLs (map). Default: 5/100"`

	// JobID is the crawl job ID. Required for action=crawl_status.
	JobID string `json:"job_id,omitempty" jsonschema:"Crawl job ID (crawl_status action only)"`
}

// FetchOutput is the tool's output (varies by action).
type FetchOutput struct {
	// Scrape results
	Content  string         `json:"content,omitempty"  jsonschema:"Extracted page content (markdown by default)."`
	Title    string         `json:"title,omitempty"    jsonschema:"Page title."`
	Links    []string       `json:"links,omitempty"    jsonschema:"Links found on the page(s)."`
	Metadata map[string]any `json:"metadata,omitempty" jsonschema:"Additional metadata."`

	// Crawl results
	Status    string      `json:"status,omitempty"    jsonschema:"Crawl status (completed, scraping, etc.)."`
	Total     int         `json:"total,omitempty"     jsonschema:"Total pages found."`
	Completed int         `json:"completed,omitempty" jsonschema:"Pages scraped."`
	Pages     []CrawlPage `json:"pages,omitempty"     jsonschema:"Crawled pages (crawl action)."`
}

// CrawlPage is a single page from a crawl.
type CrawlPage struct {
	URL     string `json:"url"              jsonschema:"Page URL."`
	Title   string `json:"title,omitempty"  jsonschema:"Page title."`
	Content string `json:"content"          jsonschema:"Page content (markdown)."`
}

// NewWebFetchTool creates the web_fetch tool.
func NewWebFetchTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"web-fetch",
		toolDescription,
		executeWebFetch,
		toolsapi.WithReadOnly(),
		toolsapi.WithCancelOnInterrupt(),
		toolsapi.WithDescriber(describeWebFetch),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🌐", Label: "Fetch", Color: "#82AAFF", Template: "{{.url}}"}),
	)
}

func executeWebFetch(ctx context.Context, in FetchInput) (FetchOutput, error) {
	action := in.Action
	if action == "" {
		action = "scrape"
	}

	// Validate inputs before resolving DI service.
	switch action {
	case "scrape", "crawl", "map":
		if strings.TrimSpace(in.URL) == "" {
			return FetchOutput{}, fmt.Errorf("web_fetch: url is required for %s", action)
		}
	case "crawl_status":
		if strings.TrimSpace(in.JobID) == "" {
			return FetchOutput{}, fmt.Errorf("web_fetch: job_id is required for crawl_status")
		}
	default:
		return FetchOutput{}, fmt.Errorf("web_fetch: unknown action %q (use: scrape, crawl, map, crawl_status)", action)
	}

	svc, err := application.UseService[webapi.WebQueryService](ctx)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("web_fetch: WebQueryService not available: %w", err)
	}

	switch action {
	case "scrape":
		return doScrape(ctx, svc, in)
	case "crawl":
		return doCrawl(ctx, svc, in)
	case "map":
		return doMap(ctx, svc, in)
	case "crawl_status":
		return doCrawlStatus(ctx, svc, in)
	default:
		return FetchOutput{}, fmt.Errorf("unreachable")
	}
}

func doScrape(ctx context.Context, svc webapi.WebQueryService, in FetchInput) (FetchOutput, error) {
	result, err := svc.Fetch(ctx, webapi.FetchInput{URL: in.URL, Formats: in.Formats})
	if err != nil {
		return FetchOutput{}, err
	}
	return FetchOutput{
		Content:  result.Content,
		Title:    result.Title,
		Links:    result.Links,
		Metadata: result.Metadata,
	}, nil
}

func doCrawl(ctx context.Context, svc webapi.WebQueryService, in FetchInput) (FetchOutput, error) {
	ci := webapi.CrawlInput{URL: in.URL, Formats: in.Formats}
	if in.MaxDepth != nil {
		ci.MaxDepth = *in.MaxDepth
	}
	if in.Limit != nil {
		ci.Limit = *in.Limit
	}
	result, err := svc.Crawl(ctx, ci)
	if err != nil {
		return FetchOutput{}, err
	}
	pages := make([]CrawlPage, len(result.Pages))
	for i, p := range result.Pages {
		pages[i] = CrawlPage{URL: p.URL, Title: p.Title, Content: p.Content}
	}
	return FetchOutput{
		Status:    result.Status,
		Total:     result.Total,
		Completed: result.Completed,
		Pages:     pages,
	}, nil
}

func doMap(ctx context.Context, svc webapi.WebQueryService, in FetchInput) (FetchOutput, error) {
	limit := 0
	if in.Limit != nil {
		limit = *in.Limit
	}
	result, err := svc.MapSite(ctx, webapi.MapInput{URL: in.URL, Limit: limit})
	if err != nil {
		return FetchOutput{}, err
	}
	return FetchOutput{Links: result.Links}, nil
}

func doCrawlStatus(ctx context.Context, svc webapi.WebQueryService, in FetchInput) (FetchOutput, error) {
	result, err := svc.CrawlStatus(ctx, in.JobID)
	if err != nil {
		return FetchOutput{}, err
	}
	return FetchOutput{
		Status:    result.Status,
		Total:     result.Total,
		Completed: result.Completed,
	}, nil
}

func describeWebFetch(raw json.RawMessage) string {
	var in FetchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "web_fetch <invalid>"
	}
	action := in.Action
	if action == "" {
		action = "scrape"
	}
	switch action {
	case "crawl_status":
		return fmt.Sprintf("web_fetch crawl_status job_id=%s", in.JobID)
	default:
		u := in.URL
		if len(u) > 50 {
			u = u[:50] + "..."
		}
		return fmt.Sprintf("web_fetch %s url=%s", action, u)
	}
}

const toolDescription = `Fetches, crawls, or maps web content from URLs.

Supports four actions via the 'action' parameter:

- **scrape** (default): Extract content from a single URL. Returns markdown, title, links, and metadata.
- **crawl**: Multi-page crawl starting from a URL. Follows links up to max_depth, returns all pages.
- **map**: Discover all accessible URLs on a website without scraping content.
- **crawl_status**: Check the status of an ongoing crawl job by job_id.

# Instructions
- Always provide the full URL including protocol (https://).
- For single-page reading, use action=scrape (or omit action).
- For site-wide content, use action=crawl with appropriate limit and max_depth.
- For URL discovery, use action=map.
- For web searching, use the web_search tool instead.
`
