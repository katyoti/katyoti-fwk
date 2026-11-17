// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The builder's posture at live ingest is availability-preserving
// (contribution-types.md §8): an INCOMPLETE kind declaration is logged and its
// entries are kept. Only a kind with NO declaration is refused. The census is
// the strict side; this pins the lenient one, so a builder that starts
// dropping (or refusing) an incomplete kind's entries fails here.
//
// What the builder does with the finding is a slog warning, and nothing else:
// no Drop (Manifest.Drops is what a provider could not publish, not a verdict
// on a kind) and no error.
func TestBuilder_IncompleteKindIsKeptAndWarned(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	providers := []contribapi.ContributionProvider{
		// A declaration naming its schema and nothing else: no layers, no
		// validate severity, no meta budget.
		staticProvider{provider: "ktw-x", typ: "contribution-type", entries: []contribapi.Contribution{
			{Provider: "ktw-x", Type: "contribution-type", ID: "widget",
				Layers: contribapi.InlineData(json.RawMessage(`{"name":"widget","schema":"ktw-x:/data-type/widget"}`))},
		}},
		staticProvider{provider: "ktw-x", typ: "widget", entries: []contribapi.Contribution{
			{Provider: "ktw-x", Type: "widget", ID: "w1", Layers: contribapi.InlineData(json.RawMessage(`{"n":1}`))},
		}},
	}

	m, err := NewBuilder("test", "dev").BuildFromProviders(context.Background(), providers)
	if err != nil {
		t.Fatalf("an incomplete kind must not fail the build: %v", err)
	}

	var kept bool
	for _, c := range m.Contributions {
		if c.URI == "ktw-x:/widget/w1" {
			kept = true
		}
	}
	if !kept {
		t.Errorf("the incomplete kind's entry ktw-x:/widget/w1 is missing from the manifest; got %d contributions", len(m.Contributions))
	}
	if len(m.Drops) != 0 {
		t.Errorf("an incomplete kind is not a drop; Manifest.Drops = %v", m.Drops)
	}

	out := logs.String()
	if !strings.Contains(out, "manifest: incomplete kind declaration") {
		t.Fatalf("no incomplete-kind warning logged; log was:\n%s", out)
	}
	for _, want := range []string{`kind \"widget\"`, "declares no layers", "declares no meta budget"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning does not name %q; log was:\n%s", want, out)
		}
	}
}
