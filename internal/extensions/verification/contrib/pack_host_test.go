// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestPackWorksUnderAnotherHost: the verification pack names no binary — hosted
// by `other-host`, its capability's type_schema and both functions name
// `other-host`, the provider that publishes the data-type and serves them.
func TestPackWorksUnderAnotherHost(t *testing.T) {
	points := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(application.Config(), points, Pack("other-host"))
	if err := app.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var caps []contribapi.Contribution
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if p.Type() != "capability" || p.Provider() != "other-host" {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("Enumerate: %v", err)
		}
		caps = append(caps, got...)
	}
	if len(caps) != 1 || caps[0].ID != "verification" {
		t.Fatalf("capabilities = %v, want verification under other-host", caps)
	}
	var c cwmapi.Capability
	if err := json.Unmarshal(caps[0].Data(), &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	uris := []string{c.TypeSchema}
	for _, f := range c.Functions {
		uris = append(uris, f.Action)
	}
	if len(uris) != 3 {
		t.Fatalf("uris = %v, want type_schema and two functions", uris)
	}
	for _, u := range uris {
		if !strings.HasPrefix(u, "other-host:") {
			t.Errorf("%q does not name its host other-host", u)
		}
	}
}
