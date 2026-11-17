// SPDX-License-Identifier: Apache-2.0

// Package firecrawl implements the Firecrawl-backed fetch/crawl/map provider.
package firecrawl

import (
	"context"
	"fmt"
	"time"

	fc "github.com/mendableai/firecrawl-go/v2"

	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

// Provider implements web_fetch operations via the Firecrawl API.
type Provider struct {
	app             *fc.FirecrawlApp
	formats         []string
	onlyMainContent *bool
	crawlMaxDepth   int
	crawlLimit      int
}

// NewProvider creates a Firecrawl provider from config.
func NewProvider(cfg webapi.FirecrawlConfig, globalFormats []string, crawlMaxDepth, crawlLimit int) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("firecrawl: api_key is required (set FIRECRAWL_API_KEY env var)")
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	app, err := fc.NewFirecrawlApp(cfg.APIKey, "", timeout)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: create client: %w", err)
	}
	formats := globalFormats
	if len(formats) == 0 {
		formats = []string{"markdown"}
	}
	if crawlMaxDepth <= 0 {
		crawlMaxDepth = 3
	}
	if crawlLimit <= 0 {
		crawlLimit = 10
	}
	onlyMain := cfg.OnlyMainContent
	if onlyMain == nil {
		t := true
		onlyMain = &t
	}
	return &Provider{app: app, formats: formats, onlyMainContent: onlyMain, crawlMaxDepth: crawlMaxDepth, crawlLimit: crawlLimit}, nil
}

// Fetch scrapes a single URL.
func (p *Provider) Fetch(_ context.Context, in webapi.FetchInput) (webapi.FetchOutput, error) {
	formats := in.Formats
	if len(formats) == 0 {
		formats = p.formats
	}

	doc, err := p.app.ScrapeURL(in.URL, &fc.ScrapeParams{Formats: formats, OnlyMainContent: p.onlyMainContent})
	if err != nil {
		return webapi.FetchOutput{}, fmt.Errorf("firecrawl: scrape %s: %w", in.URL, err)
	}
	if doc == nil {
		return webapi.FetchOutput{}, fmt.Errorf("firecrawl: no content returned for %s", in.URL)
	}

	return docToFetchOutput(doc), nil
}

// Crawl starts a multi-page crawl and blocks until complete.
func (p *Provider) Crawl(_ context.Context, in webapi.CrawlInput) (webapi.CrawlOutput, error) {
	formats := in.Formats
	if len(formats) == 0 {
		formats = p.formats
	}
	maxDepth := in.MaxDepth
	if maxDepth <= 0 {
		maxDepth = p.crawlMaxDepth
	}
	limit := in.Limit
	if limit <= 0 {
		limit = p.crawlLimit
	}

	params := &fc.CrawlParams{
		ScrapeOptions: fc.ScrapeParams{Formats: formats, OnlyMainContent: p.onlyMainContent},
		MaxDepth:      &maxDepth,
		Limit:         &limit,
	}

	resp, err := p.app.CrawlURL(in.URL, params, nil)
	if err != nil {
		return webapi.CrawlOutput{}, fmt.Errorf("firecrawl: crawl %s: %w", in.URL, err)
	}

	out := webapi.CrawlOutput{
		Status:    resp.Status,
		Total:     resp.Total,
		Completed: resp.Completed,
	}
	for _, doc := range resp.Data {
		if doc == nil {
			continue
		}
		page := webapi.CrawlPage{Content: doc.Markdown}
		if doc.Metadata != nil {
			if doc.Metadata.Title != nil {
				page.Title = *doc.Metadata.Title
			}
			if doc.Metadata.SourceURL != nil {
				page.URL = *doc.Metadata.SourceURL
			}
		}
		out.Pages = append(out.Pages, page)
	}

	return out, nil
}

// CrawlStatus checks the status of an async crawl job.
func (p *Provider) CrawlStatus(_ context.Context, jobID string) (webapi.CrawlStatusOutput, error) {
	resp, err := p.app.CheckCrawlStatus(jobID)
	if err != nil {
		return webapi.CrawlStatusOutput{}, fmt.Errorf("firecrawl: check crawl %s: %w", jobID, err)
	}
	return webapi.CrawlStatusOutput{
		Status:    resp.Status,
		Total:     resp.Total,
		Completed: resp.Completed,
	}, nil
}

// MapSite discovers all URLs on a website.
func (p *Provider) MapSite(_ context.Context, in webapi.MapInput) (webapi.MapOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	resp, err := p.app.MapURL(in.URL, &fc.MapParams{Limit: &limit})
	if err != nil {
		return webapi.MapOutput{}, fmt.Errorf("firecrawl: map %s: %w", in.URL, err)
	}
	return webapi.MapOutput{Links: resp.Links}, nil
}

func docToFetchOutput(doc *fc.FirecrawlDocument) webapi.FetchOutput {
	out := webapi.FetchOutput{
		Content: doc.Markdown,
		Links:   doc.Links,
	}
	if doc.Metadata != nil {
		if doc.Metadata.Title != nil {
			out.Title = *doc.Metadata.Title
		}
		out.Metadata = map[string]any{}
		if doc.Metadata.Description != nil {
			out.Metadata["description"] = []string(*doc.Metadata.Description)
		}
		if doc.Metadata.URL != nil {
			out.Metadata["url"] = *doc.Metadata.URL
		}
		if doc.Metadata.StatusCode != nil {
			out.Metadata["status_code"] = *doc.Metadata.StatusCode
		}
	}
	return out
}
