// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// The second half of the D1 delivery path: an `object-type` contributed by
// ANOTHER sidecar is resolved out of the aggregate catalog into
// CatalogTypeSet.Objects — which is the set the store validates writes against.
//
// The loader and this resolution are the two links that were both missing while
// the declaration files already existed. Test both, or "declared" keeps meaning
// "on disk and read by nobody".
func TestCatalogResolvesObjectTypesFromOtherSidecar(t *testing.T) {
	mustBody := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	manifest := &mbapi.Manifest{
		Provider: "ktw-fake",
		Contributions: []contribapi.Contribution{
			{Provider: "ktw-fake", Type: "object-type", ID: "fake-object", Layers: contribapi.InlineData(mustBody(
				api.ObjectType{Name: "fake-object", Capabilities: []string{"container", "checkable"}}))},
			// No name — must be DROPPED, not registered. It could never match an
			// object's type field, and a non-empty declared set is what switches
			// validation on: counting it would turn validation on while matching
			// nothing.
			{Provider: "ktw-fake", Type: "object-type", ID: "nameless", Layers: contribapi.InlineData(mustBody(
				api.ObjectType{Capabilities: []string{"container"}}))},
			// A kind CWM does not consume, to prove the switch still filters.
			{Provider: "ktw-fake", Type: "skill", ID: "not-a-type", Layers: contribapi.InlineData(mustBody(map[string]string{"name": "x"}))},
		},
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{manifest}, []string{"ktw-fake"})

	set := LoadCWMTypesFromCatalog(context.Background(), cat)
	if len(set.Objects) != 1 {
		t.Fatalf("want 1 resolved object-type, got %d: %+v", len(set.Objects), set.Objects)
	}
	got := set.Objects[0]
	if got.Name != "fake-object" {
		t.Errorf("resolved name = %q, want fake-object", got.Name)
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("capabilities lost in transit: %v", got.Capabilities)
	}
}

// A cross-provider name collision must resolve the SAME WAY every run.
//
// This is not hypothetical: `module` is declared by ktw-code (as a node type,
// derived) and by ktw-methods (as an explicit object type), and they are
// genuinely different shapes of the same concept. Catalog iteration is
// map-backed, so without an explicit order the winner would change between
// runs — and a type whose meaning depends on iteration order is worse than
// either meaning.
func TestObjectTypeResolutionIsDeterministic(t *testing.T) {
	mustBody := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	mk := func(provider string, names ...string) *mbapi.Manifest {
		m := &mbapi.Manifest{Provider: provider}
		for _, n := range names {
			m.Contributions = append(m.Contributions, contribapi.Contribution{
				Provider: provider, Type: "object-type", ID: n,
				Layers: contribapi.InlineData(mustBody(api.ObjectType{Name: n, Label: provider})),
			})
		}
		return m
	}
	// Same inputs, different manifest order: the resolved sequence must match.
	a := LoadCWMTypesFromCatalog(context.Background(), manifestlocal.NewMemoryCatalog(
		[]*mbapi.Manifest{mk("ktw-code", "module", "charter"), mk("ktw-methods", "module", "use-case")},
		[]string{"ktw-code", "ktw-methods"}))
	b := LoadCWMTypesFromCatalog(context.Background(), manifestlocal.NewMemoryCatalog(
		[]*mbapi.Manifest{mk("ktw-methods", "use-case", "module"), mk("ktw-code", "charter", "module")},
		[]string{"ktw-methods", "ktw-code"}))

	if len(a.Objects) != len(b.Objects) {
		t.Fatalf("different counts: %d vs %d", len(a.Objects), len(b.Objects))
	}
	for i := range a.Objects {
		if a.Objects[i].Name != b.Objects[i].Name {
			t.Fatalf("resolution order differs at %d: %q vs %q — the winner of a collision would vary between runs",
				i, a.Objects[i].Name, b.Objects[i].Name)
		}
	}
	// The collision is never silent: the first provider in name
	// order keeps `module`, whatever the manifest order, and the other is
	// refused with a finding naming both.
	for _, set := range []struct {
		name string
		objs []api.ObjectType
		fs   []api.DeclarationFinding
	}{{"a", a.Objects, a.Findings}, {"b", b.Objects, b.Findings}} {
		n, winner := 0, ""
		for _, o := range set.objs {
			if o.Name == "module" {
				n, winner = n+1, o.Label
			}
		}
		if n != 1 || winner != "ktw-code" {
			t.Errorf("%s: %d `module` declarations, kept %q; want ktw-code's only", set.name, n, winner)
		}
		clash := 0
		for _, f := range set.fs {
			if f.Rule == api.FindingNameClash && f.Name == "module" && f.Provider == "ktw-methods" {
				clash++
			}
		}
		if clash != 1 {
			t.Errorf("%s: name-clash findings on module from ktw-methods = %d, want 1 (%+v)", set.name, clash, set.fs)
		}
	}
}

// TestCatalogResolvesEveryKindItsDecoderHandles closes the gap that let aspects
// ship dead for four days.
//
// `LoadCWMTypesFromCatalog` names the consumed kinds TWICE — an allow-list that
// filters, and a switch that decodes — and from 7g until 2026-09-25 the two
// disagreed: `aspect` was in the decoder and missing from the filter, so every
// authored aspect was dropped before it was ever read and `set.Aspects` was
// always empty in production. Nothing failed; aspects simply contributed
// nothing.
//
// This feeds ONE contribution of every kind the decoder handles and asserts
// each lands. A kind added to one place and not the other fails here.
func TestCatalogResolvesEveryKindItsDecoderHandles(t *testing.T) {
	body := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	manifest := &mbapi.Manifest{
		Provider: "ktw-fake",
		Contributions: []contribapi.Contribution{
			{Provider: "ktw-fake", Type: "edge-type", ID: "e", Layers: contribapi.InlineData(body(api.EdgeTypeRegistration{Name: "e"}))},
			{Provider: "ktw-fake", Type: "object-type", ID: "o", Layers: contribapi.InlineData(body(api.ObjectType{Name: "o"}))},
			{Provider: "ktw-fake", Type: "capability", ID: "c", Layers: contribapi.InlineData(body(api.Capability{Name: "c"}))},
			{Provider: "ktw-fake", Type: "aspect", ID: "t", Layers: contribapi.InlineData(body(api.Aspect{Name: "t", Capabilities: []string{"c"}}))},
			{Provider: "ktw-fake", Type: "workflow", ID: "w", Layers: contribapi.InlineData(body(api.Workflow{Name: "w", Initial: "a"}))},
			{Provider: "ktw-fake", Type: "hook-type", ID: "review", Layers: contribapi.InlineData(body(api.HookType{Name: "review", Timing: "on-demand", Answer: "findings"}))},
			{Provider: "ktw-fake", Type: "hook", ID: "h", Layers: contribapi.InlineData(body(api.Hook{Name: "h", Type: "lint", URI: "x:y/z"}))},
		},
	}
	set := LoadCWMTypesFromCatalog(context.Background(),
		manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{manifest}, []string{"ktw-fake"}))

	for _, row := range []struct {
		kind string
		got  int
	}{
		{"edge-type", len(set.Edges)},
		{"object-type", len(set.Objects)},
		{"capability", len(set.Capabilities)},
		{"aspect", len(set.Aspects)},
		{"workflow", len(set.Workflows)},
		{"hook-type", len(set.HookTypes)},
		{"hook", len(set.Hooks)},
	} {
		if row.got != 1 {
			t.Errorf("%s: resolved %d, want 1 — the allow-list and the decoder disagree about this kind", row.kind, row.got)
		}
	}
}
