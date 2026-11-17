// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"path/filepath"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cache"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

// Extension registers the WebQueryService in the DI container and declares
// its infrastructure (config file + API keys) for the config/check commands.
func Extension(init *application.ApplicationInit) {
	// Declare infrastructure so `config` and `check` commands see the config file and API keys.
	init.AddExtension(appcore.DeclareInfra("web",
		// DeclareInfra has no service type to derive a file from, so this one
		// is declared. Only PublishService derives its own.
		appcore.WithConfigFiles("WebQueryService.yaml"),
		appcore.WithAPIKeys(
			publish.APIKeyDef{EnvName: "FIRECRAWL_API_KEY", Label: "Firecrawl API key", Description: "Required for web_fetch (scrape, crawl, map)"},
			publish.APIKeyDef{EnvName: "VALUESERP_API_KEY", Label: "ValueSERP API key", Description: "Required for web_search (default provider)"},
			publish.APIKeyDef{EnvName: "SERP_API_KEY", Label: "SerpAPI key", Description: "Required for web_search (serpapi provider)"},
			publish.APIKeyDef{EnvName: "BRAVE_SEARCH_API_KEY", Label: "Brave Search API key", Description: "Required for web_search (brave provider)"},
		),
	))

	// Register the DI service (internal, not published).
	init.AddExtension(application.DeclareService[webapi.WebQueryService, webapi.WebQueryServiceConfig](
		func(_ context.Context, cfg *webapi.WebQueryServiceConfig) (webapi.WebQueryService, error) {
			var c cache.Cache
			if cfg.CacheEnabled == nil || *cfg.CacheEnabled {
				cacheDir := cfg.CacheDir
				if cacheDir == "" {
					cacheDir = filepath.Join(application.Env().GetCacheSubDir("web"))
				}
				ttlHours := cfg.CacheTTLHours
				if ttlHours <= 0 {
					ttlHours = 24
				}
				ttl := time.Duration(ttlHours) * time.Hour
				c = cache.NewFileCacheWithTTL(cacheDir, ttl)
			} else {
				c = cache.NopCache{}
			}
			return NewWebQueryService(*cfg, c)
		},
	))
}
