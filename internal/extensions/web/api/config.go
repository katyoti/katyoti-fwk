// SPDX-License-Identifier: Apache-2.0

package api

// WebQueryServiceConfig is the typed config loaded from WebQueryService.yaml.
type WebQueryServiceConfig struct {
	FetchProvider  string `yaml:"fetch_provider"  json:"fetch_provider"`  // "firecrawl"
	SearchProvider string `yaml:"search_provider" json:"search_provider"` // "valueserp", "serpapi", "brave"

	// Global defaults — applied to all operations unless overridden per-call.
	SearchLocation   string   `yaml:"search_location"    json:"search_location"    default:"France"`
	SearchNumResults int      `yaml:"search_num_results" json:"search_num_results" default:"5"`
	CrawlMaxDepth    int      `yaml:"crawl_max_depth"    json:"crawl_max_depth"    default:"3"`
	CrawlLimit       int      `yaml:"crawl_limit"        json:"crawl_limit"        default:"10"`
	ScrapeFormats    []string `yaml:"scrape_formats"     json:"scrape_formats"`

	// Cache settings.
	CacheEnabled  *bool  `yaml:"cache_enabled"   json:"cache_enabled"   default:"true"`
	CacheTTLHours int    `yaml:"cache_ttl_hours" json:"cache_ttl_hours" default:"24"`
	CacheDir      string `yaml:"cache_dir"       json:"cache_dir"`

	// Provider-specific settings.
	Firecrawl FirecrawlConfig `yaml:"firecrawl" json:"firecrawl"`
	ValueSERP ValueSERPConfig `yaml:"valueserp" json:"valueserp"`
	SerpAPI   SerpAPIConfig   `yaml:"serpapi"   json:"serpapi"`
	Brave     BraveConfig     `yaml:"brave"     json:"brave"`
}

// FirecrawlConfig holds Firecrawl-specific settings.
type FirecrawlConfig struct {
	APIKey          string `yaml:"api_key"           json:"api_key"           envkey:"FIRECRAWL_API_KEY"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"   json:"timeout_seconds"   default:"30"`
	OnlyMainContent *bool  `yaml:"only_main_content" json:"only_main_content" default:"true"`
}

// ValueSERPConfig holds ValueSERP-specific settings.
type ValueSERPConfig struct {
	APIKey string `yaml:"api_key" json:"api_key" envkey:"VALUESERP_API_KEY"`
}

// SerpAPIConfig holds SerpAPI-specific settings.
type SerpAPIConfig struct {
	APIKey string `yaml:"api_key" json:"api_key" envkey:"SERP_API_KEY"`
}

// BraveConfig holds Brave Search-specific settings.
type BraveConfig struct {
	APIKey         string `yaml:"api_key"          json:"api_key"          envkey:"BRAVE_SEARCH_API_KEY"`
	TimeoutSeconds int    `yaml:"timeout_seconds"  json:"timeout_seconds"  default:"10"`
	ResultCount    int    `yaml:"result_count"     json:"result_count"     default:"10"`
}
