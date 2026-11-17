// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// filesOnlyPack is the e19 fixture: a pack that declares a kind of its own, its
// schema, and one entry — files only.
func filesOnlyPack(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "..",
		"internal", "test", "fixtures", "contributions", "files-only-pack")
}

// TestHostServesAKindItHasNoCodeFor is the in-process half of e19's files-only
// proof: the generic host, pointed at the fixture, publishes the pack's kind
// declaration, its schema and its entry — and the entry passes the SAME census
// the compiled sidecars pass (every kind complete, required layers carried,
// schema shipped, body valid against it). The cross-process half, through a
// hub, is internal/test/catalog.TestFilesOnlyKindThroughTheUniverse.
func TestHostServesAKindItHasNoCodeFor(t *testing.T) {
	census.Isolate(t)
	t.Setenv(contentRootEnv, filesOnlyPack(t))

	app := application.New(application.Config(), buildExtensions(embed.FS{})...)
	if err := app.Run(); err != nil {
		t.Fatalf("run host over the fixture: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	m, err := manifestlocal.NewBuilder("files-only-pack", "1.0.0").Build(ctx)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	const (
		kindURI   = "files-only-pack:/contribution-type/recipe"
		schemaURI = "files-only-pack:/data-type/recipe"
		entryURI  = "files-only-pack:/recipe/pancakes"
	)
	byURI := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		byURI[c.ResolvedURI()] = c
	}
	for _, uri := range []string{kindURI, schemaURI, entryURI} {
		if _, ok := byURI[uri]; !ok {
			t.Fatalf("host did not publish %s (%d contributions)", uri, len(m.Contributions))
		}
	}

	// The kind reads as declared by the pack, and names the pack's schema.
	kinds := contribapi.KindsFrom(m.Contributions)
	if got := kinds.SchemaFor("recipe"); got != schemaURI {
		t.Errorf("kind recipe: schema = %q, want %q", got, schemaURI)
	}
	if got := kinds.DefaultLayer("recipe"); got != contribapi.LayerSource {
		t.Errorf("kind recipe: default layer = %q, want source", got)
	}

	// Both declared layers are carried: data (structure) and source (the file).
	entry := byURI[entryURI]
	var data struct {
		Name   string `json:"name"`
		Serves int    `json:"serves"`
	}
	if err := json.Unmarshal(entry.Data(), &data); err != nil || data.Name != "Pancakes" || data.Serves != 4 {
		t.Errorf("entry data layer = %s (err %v), want the recipe's structure", entry.Data(), err)
	}
	if _, ok := entry.Layers[contribapi.LayerSource]; !ok {
		t.Errorf("entry carries no source layer: %v", entry.Layers)
	}

	// The whole census, then a direct check that validation really ran against
	// the pack's schema (a census that skipped the entry would pass silently).
	svc := census.SchemaService(t)
	census.AssertValidWith(t, "files-only-pack", m, svc)
	out, err := svc.ValidateAgainst(context.Background(), schemaapi.ValidateAgainstInput{URI: schemaURI, Body: entry.Data()})
	if err != nil {
		t.Fatalf("validate entry against %s: %v", schemaURI, err)
	}
	if !out.Valid {
		t.Errorf("entry does not validate against the pack's schema: %v", out.Diagnostics)
	}
	bad, _ := json.Marshal(map[string]any{"name": 7})
	out, err = svc.ValidateAgainst(context.Background(), schemaapi.ValidateAgainstInput{URI: schemaURI, Body: bad})
	if err != nil || out.Valid {
		t.Errorf("a body violating the pack's schema validated (err %v) — the schema is not what the check read", err)
	}
}
