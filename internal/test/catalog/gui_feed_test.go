// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// gui_feed_test.go — the FEED behind ktw-gui's Contributions view.
//
// The GUI's binding (App.ListContributions) is three calls over a
// `manifestremote.Catalog` built on a universe sender:
//
//	cat     := manifestremote.New(sender)
//	entries := mbapi.Query(ctx, Q{Reach: ReachUniverse, Catalog: cat})
//	kinds   := mbapi.KindSchemas(ctx, Q{Reach: ReachUniverse, Catalog: cat})
//	         → ContributionSummary{…, Schema: mbapi.SchemaFor(kinds, e.Type)}
//
// This exercises that exact pipeline against a real hub. Per
// `ktw-compose:/skill/test-gui`, the data behind a screen is tested at the feed
// — a browser-driven GUI in dev mode serves NO catalog data, so driving the UI
// can only ever be a presentation-layer verdict.
//
// What it proves: the GUI is handed every contribution the universe holds, and
// every kind that declares a schema arrives WITH it. What it cannot prove: that
// the panel paints them. That is the eye, on a built binary.

// guiFeed runs the GUI's query and returns what the Contributions view receives.
func guiFeed(t *testing.T, cat mbapi.CatalogReader) ([]contribapi.Contribution, map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	entries, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		t.Fatalf("the Contributions view's query failed: %v", err)
	}
	kinds, err := mbapi.KindSchemas(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		// The GUI degrades to a listing with no schema column. A test must not:
		// the column is half of what this suite is here to prove.
		t.Fatalf("KindSchemas failed, so the GUI's schema column would be empty: %v", err)
	}
	return entries, kinds
}

// TestGUIFeed_ListsEveryContributionWithItsSchema is the acceptance test for
// "ktw-gui shows all the contributions, in the right place, with the schemas".
func TestGUIFeed_ListsEveryContributionWithItsSchema(t *testing.T) {
	sender := startContractUniverse(t)
	cat := manifestremote.New(sender)

	entries, kinds := guiFeed(t, cat)
	if len(entries) == 0 {
		t.Fatal("the Contributions view would render empty — the universe query returned nothing")
	}

	// (1) Every provider in the universe reaches the view. A view that silently
	// shows one binary's contributions looks exactly like a working one.
	providers := map[string]int{}
	byKind := map[string]int{}
	for _, e := range entries {
		providers[e.Provider]++
		byKind[e.Type]++
	}
	for _, want := range sidecarsToInclude {
		if providers[want] == 0 {
			t.Errorf("no contribution from %q reached the view (providers seen: %v)",
				want, sortedKeys(providers))
		}
	}

	// (2) Every entry is addressable. The view renders the URI and every action
	// on the row resolves through it.
	for _, e := range entries {
		if e.ResolvedURI() == "" {
			t.Errorf("contribution %s/%s/%s has no URI — the row cannot act on it",
				e.Provider, e.Type, e.ID)
			continue
		}
		if _, err := contribapi.ParseResourceURI(e.ResolvedURI()); err != nil {
			t.Errorf("contribution URI %q does not parse: %v", e.ResolvedURI(), err)
		}
	}

	// (3) The schema column. A kind that DECLARES a schema must hand the view a
	// schema URI for every entry of that kind — `SchemaFor` is what fills the
	// column, and an empty one is how a form loses its fields.
	if len(kinds) == 0 {
		t.Fatal("no kind declared a schema — the view's schema column would be empty for everything")
	}
	missing := map[string]bool{}
	for _, e := range entries {
		if _, declared := kinds[e.Type]; !declared {
			continue // the kind declares none; the column is legitimately blank
		}
		if mbapi.SchemaFor(kinds, e.Type) == "" {
			missing[e.Type] = true
		}
	}
	if len(missing) > 0 {
		t.Errorf("kinds that DECLARE a schema but hand the view an empty column: %v", sortedKeys(missing))
	}

	t.Logf("GUI feed: %d contributions · %d providers · %d kinds · %d kinds declare a schema",
		len(entries), len(providers), len(byKind), len(kinds))
}

// TestGUIFeed_DeclaredSchemasResolve closes the other half: the column holds a
// URI, and the URI ANSWERS.
//
// A row showing a schema nobody can fetch is the c26/c27 failure wearing a
// different hat — the view looks complete and the form behind it renders
// nothing.
func TestGUIFeed_DeclaredSchemasResolve(t *testing.T) {
	sender := startContractUniverse(t)
	cat := manifestremote.New(sender)

	_, kinds := guiFeed(t, cat)
	if len(kinds) == 0 {
		t.Fatal("no kind declared a schema")
	}

	var unresolved []string
	for kind, schemaURI := range kinds {
		if schemaURI == "" {
			continue
		}
		// A schema is a data-type's SOURCE layer — the content lane's question,
		// asked over the wire (CatalogService/OpenLayer). The body lane answers
		// the data layer only and has nothing to say about a data-type.
		rc, ref, err := cat.OpenLayer(context.Background(), schemaURI, contribapi.LayerSource)
		if err != nil {
			unresolved = append(unresolved, kind+" → "+schemaURI+": "+err.Error())
			continue
		}
		body, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil || len(body) == 0 {
			unresolved = append(unresolved, kind+" → "+schemaURI+": resolved to an EMPTY source layer")
			continue
		}
		if got := contribapi.HashContent(body); got != ref.Hash {
			unresolved = append(unresolved, kind+" → "+schemaURI+": content does not match its address")
		}
	}
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		t.Errorf("%d kind schema(s) the GUI would show but cannot fetch:\n  %s",
			len(unresolved), strings.Join(unresolved, "\n  "))
	}
	t.Logf("GUI feed: %d declared kind schema(s), all resolvable", len(kinds))
}

// TestGUIFeed_SchemaASTBindingAnswers drives the call App.GetSchemaAST makes.
//
// The previous test proves the schema URI resolves through the CATALOG. This
// one proves it resolves through the SCHEMA SERVICE, which is the lane the
// panel actually uses:
//
//	sender.Call(ctx, "ktw-hub:schema/GetDataTypeWithExtensions", …) → AST
//
// They are different lanes and have failed independently — a URI that answers
// one and not the other is a form that renders nothing while the row beside it
// looks healthy.
func TestGUIFeed_SchemaASTBindingAnswers(t *testing.T) {
	sender := startContractUniverse(t)
	cat := manifestremote.New(sender)
	_, kinds := guiFeed(t, cat)

	var broken []string
	checked := 0
	for kind, schemaURI := range kinds {
		if schemaURI == "" {
			continue
		}
		checked++
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		raw, err := sender.Call(ctx, "ktw-hub:schema/GetDataTypeWithExtensions",
			schemaapi.GetDataTypeWithExtensionsInput{
				URI:        schemaURI,
				Extensions: []schemaapi.ExtensionRef{{Type: "*", Name: "*"}},
			})
		cancel()
		if err != nil {
			broken = append(broken, kind+" → "+schemaURI+": "+err.Error())
			continue
		}
		var resp schemaapi.GetDataTypeOutput
		if err := json.Unmarshal(raw, &resp); err != nil {
			broken = append(broken, kind+" → "+schemaURI+": decode: "+err.Error())
			continue
		}
		// The binding turns a nil Schema into the "registration may have failed
		// at hub Phase 2" error the user sees instead of a form.
		if resp.Schema == nil {
			broken = append(broken, kind+" → "+schemaURI+": nil schema (the panel shows a registration error)")
			continue
		}
		if len(resp.Schema.Types) == 0 {
			broken = append(broken, kind+" → "+schemaURI+": schema declares no types (a form with no fields)")
		}
	}
	if len(broken) > 0 {
		sort.Strings(broken)
		t.Errorf("%d of %d kind schema(s) fail the binding the schema panel uses:\n  %s",
			len(broken), checked, strings.Join(broken, "\n  "))
	}
	if checked == 0 {
		t.Fatal("no kind declared a schema — this test checked nothing")
	}
	t.Logf("GUI schema binding: %d kind schema(s) return a usable AST", checked)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ = hubharness.MissingBinary
