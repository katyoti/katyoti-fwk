// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

func TestEnvkeyResolution(t *testing.T) {
	os.Setenv("FIRECRAWL_API_KEY", "test-fc-key")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test-brave-key")
	defer os.Unsetenv("FIRECRAWL_API_KEY")
	defer os.Unsetenv("BRAVE_SEARCH_API_KEY")

	var cfg webapi.WebQueryServiceConfig
	if err := config.ProcessConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Firecrawl.APIKey != "test-fc-key" {
		t.Errorf("Firecrawl.APIKey = %q, want test-fc-key", cfg.Firecrawl.APIKey)
	}
	if cfg.Brave.APIKey != "test-brave-key" {
		t.Errorf("Brave.APIKey = %q, want test-brave-key", cfg.Brave.APIKey)
	}
}
