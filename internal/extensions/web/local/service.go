// SPDX-License-Identifier: Apache-2.0

// Package local provides the DI wiring for the WebQueryService.
package local

import (
	"context"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/cache"
	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/web/internal/brave"
	"github.com/katyoti/katyoti-fwk/internal/extensions/web/internal/firecrawl"
	"github.com/katyoti/katyoti-fwk/internal/extensions/web/internal/serpapi"
	"github.com/katyoti/katyoti-fwk/internal/extensions/web/internal/valueserp"
)

// FetchProvider handles scrape, crawl, map operations.
type FetchProvider interface {
	Fetch(ctx context.Context, in webapi.FetchInput) (webapi.FetchOutput, error)
	Crawl(ctx context.Context, in webapi.CrawlInput) (webapi.CrawlOutput, error)
	CrawlStatus(ctx context.Context, jobID string) (webapi.CrawlStatusOutput, error)
	MapSite(ctx context.Context, in webapi.MapInput) (webapi.MapOutput, error)
}

// SearchProvider handles web search.
type SearchProvider interface {
	Search(ctx context.Context, in webapi.SearchInput) (webapi.SearchOutput, error)
}

// webQueryService routes calls to the configured providers with caching.
// Providers are created lazily on first use.
type webQueryService struct {
	cfg       webapi.WebQueryServiceConfig
	cache     cache.Cache
	fetchO    sync.Once
	searchO   sync.Once
	fetch     FetchProvider
	fetchErr  error
	search    SearchProvider
	searchErr error
}

// NewWebQueryService creates a WebQueryService from config.
func NewWebQueryService(cfg webapi.WebQueryServiceConfig, c cache.Cache) (webapi.WebQueryService, error) {
	return &webQueryService{cfg: cfg, cache: c}, nil
}

func (s *webQueryService) getFetch() (FetchProvider, error) {
	s.fetchO.Do(func() {
		formats := s.cfg.ScrapeFormats
		switch s.cfg.FetchProvider {
		case "firecrawl", "":
			s.fetch, s.fetchErr = firecrawl.NewProvider(s.cfg.Firecrawl, formats, s.cfg.CrawlMaxDepth, s.cfg.CrawlLimit)
		default:
			s.fetchErr = fmt.Errorf("unknown fetch_provider %q (supported: firecrawl)", s.cfg.FetchProvider)
		}
	})
	return s.fetch, s.fetchErr
}

func (s *webQueryService) getSearch() (SearchProvider, error) {
	s.searchO.Do(func() {
		loc := s.cfg.SearchLocation
		num := s.cfg.SearchNumResults
		switch s.cfg.SearchProvider {
		case "valueserp", "":
			s.search, s.searchErr = valueserp.NewProvider(s.cfg.ValueSERP, loc, num)
		case "serpapi":
			s.search, s.searchErr = serpapi.NewProvider(s.cfg.SerpAPI, loc, num)
		case "brave":
			s.search, s.searchErr = brave.NewProvider(s.cfg.Brave)
		default:
			s.searchErr = fmt.Errorf("unknown search_provider %q (supported: valueserp, serpapi, brave)", s.cfg.SearchProvider)
		}
	})
	return s.search, s.searchErr
}

func (s *webQueryService) Fetch(ctx context.Context, in webapi.FetchInput) (webapi.FetchOutput, error) {
	f, err := s.getFetch()
	if err != nil {
		return webapi.FetchOutput{}, err
	}
	key := cache.MakeKey("fetch", in)
	return cache.Cached(s.cache, "web-fetch", key, func() (webapi.FetchOutput, error) {
		return f.Fetch(ctx, in)
	})
}

func (s *webQueryService) Crawl(ctx context.Context, in webapi.CrawlInput) (webapi.CrawlOutput, error) {
	f, err := s.getFetch()
	if err != nil {
		return webapi.CrawlOutput{}, err
	}
	key := cache.MakeKey("crawl", in)
	return cache.Cached(s.cache, "web-fetch", key, func() (webapi.CrawlOutput, error) {
		return f.Crawl(ctx, in)
	})
}

func (s *webQueryService) CrawlStatus(ctx context.Context, jobID string) (webapi.CrawlStatusOutput, error) {
	f, err := s.getFetch()
	if err != nil {
		return webapi.CrawlStatusOutput{}, err
	}
	return f.CrawlStatus(ctx, jobID)
}

func (s *webQueryService) MapSite(ctx context.Context, in webapi.MapInput) (webapi.MapOutput, error) {
	f, err := s.getFetch()
	if err != nil {
		return webapi.MapOutput{}, err
	}
	key := cache.MakeKey("map", in)
	return cache.Cached(s.cache, "web-fetch", key, func() (webapi.MapOutput, error) {
		return f.MapSite(ctx, in)
	})
}

func (s *webQueryService) Search(ctx context.Context, in webapi.SearchInput) (webapi.SearchOutput, error) {
	sr, err := s.getSearch()
	if err != nil {
		return webapi.SearchOutput{}, err
	}
	key := cache.MakeKey("search", in)
	return cache.Cached(s.cache, "web-search", key, func() (webapi.SearchOutput, error) {
		return sr.Search(ctx, in)
	})
}
