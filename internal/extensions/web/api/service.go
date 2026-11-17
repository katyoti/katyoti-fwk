// SPDX-License-Identifier: Apache-2.0

// Package api defines the WebQueryService interface used by web_fetch
// and web_search tools to perform URL content extraction and web search.
package api

import "context"

// WebQueryService provides URL fetching, crawling, and web search capabilities.
// Resolved via application.UseService[WebQueryService](ctx).
type WebQueryService interface {
	// Fetch retrieves the content of a single URL (scrape).
	Fetch(ctx context.Context, in FetchInput) (FetchOutput, error)

	// Crawl starts a multi-page crawl from a URL. Blocks until complete.
	Crawl(ctx context.Context, in CrawlInput) (CrawlOutput, error)

	// CrawlStatus checks the status of an async crawl job.
	CrawlStatus(ctx context.Context, jobID string) (CrawlStatusOutput, error)

	// MapSite discovers all accessible URLs on a website.
	MapSite(ctx context.Context, in MapInput) (MapOutput, error)

	// Search performs a web search query.
	Search(ctx context.Context, in SearchInput) (SearchOutput, error)
}

// ── Fetch (scrape) ──────────────────────────────────────────────────────

type FetchInput struct {
	URL     string   `json:"url"`
	Formats []string `json:"formats,omitempty"` // e.g. ["markdown", "links"]; default: ["markdown"]
}

type FetchOutput struct {
	Content  string         `json:"content"`
	Title    string         `json:"title,omitempty"`
	Links    []string       `json:"links,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ── Crawl ───────────────────────────────────────────────────────────────

type CrawlInput struct {
	URL      string   `json:"url"`
	MaxDepth int      `json:"max_depth,omitempty"` // default: 10
	Limit    int      `json:"limit,omitempty"`     // max pages; default: 5
	Formats  []string `json:"formats,omitempty"`
}

type CrawlOutput struct {
	Status    string      `json:"status"`    // "completed", "scraping", etc.
	Total     int         `json:"total"`     // total pages found
	Completed int         `json:"completed"` // pages scraped
	Pages     []CrawlPage `json:"pages"`
}

type CrawlPage struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"` // markdown
}

type CrawlStatusOutput struct {
	Status    string `json:"status"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
}

// ── Map ─────────────────────────────────────────────────────────────────

type MapInput struct {
	URL   string `json:"url"`
	Limit int    `json:"limit,omitempty"` // default: 100
}

type MapOutput struct {
	Links []string `json:"links"`
}

// ── Search ──────────────────────────────────────────────────────────────

type SearchInput struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
}

type SearchOutput struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}
