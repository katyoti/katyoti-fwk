// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// --- M4: a URI two members carry resolves to one host, every load ---

// sharedEntry is one logical contribution two members both carry (a layer
// provider's entry bundled into two binaries), each copy tagged with the
// carrier so a read can tell which one answered.
func sharedEntry(carrier string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "shared-pack", Type: "recipe", ID: "twice"}
	c.SetData(json.RawMessage(`{"name":"twice","from":"` + carrier + `"}`))
	return c
}

// Before: reindexHosts ranged the (name → manifest) map, so the host of a URI
// two members carried was a coin toss per Reload. Now: member order decides,
// the same host answers on every load, and the shadowed carrier is a finding.
func TestHostOf_OneURITwoMembers_SameHostOnEveryReload(t *testing.T) {
	cache := NewCache(t.TempDir())
	for _, name := range []string{"ktw-a", "ktw-b"} {
		m := makeManifest(name, "1.0.0", "sha256:"+name)
		m.Contributions = []contribapi.Contribution{sharedEntry(name)}
		if err := SeedCacheSave(t, cache, m); err != nil {
			t.Fatal(err)
		}
	}
	uri := sharedEntry("").TypedURI()

	for _, order := range [][]string{{"ktw-b", "ktw-a"}, {"ktw-a", "ktw-b"}} {
		cat, err := NewCatalog(cache, MembersOf(cache, order...))
		if err != nil {
			t.Fatal(err)
		}
		want := order[0]
		for i := 0; i < 20; i++ {
			if err := cat.Reload(); err != nil {
				t.Fatal(err)
			}
			if got := cat.HostOf(uri); got != want {
				t.Fatalf("order %v, reload %d: host = %q, want the first member %q", order, i, got, want)
			}
			e, ok := cat.ContributionAt(uri.String())
			if !ok || string(e.Data()) != `{"name":"twice","from":"`+want+`"}` {
				t.Fatalf("order %v, reload %d: ContributionAt answered %s, want %s's copy", order, i, e.Data(), want)
			}
		}
		cols := cat.URICollisions()
		if len(cols) != 1 || cols[0].URI != uri.String() || cols[0].Host != want || len(cols[0].Shadowed) != 1 || cols[0].Shadowed[0] != order[1] {
			t.Fatalf("order %v: collisions = %+v, want one: %s hosted by %s, shadowing %s", order, cols, uri, want, order[1])
		}
		byType, err := cat.GetContributionsByType(context.Background(), "recipe")
		if err != nil || len(byType) != 1 {
			t.Fatalf("order %v: by-type listed %d copies of one URI, want 1 (%v)", order, len(byType), err)
		}
	}
}

// The memory view obeys the same rule: the first member in declared order
// answers, on every rebuild, and the collision is reported.
func TestMemoryCatalog_OneURITwoMembers_SameEntryOnEveryReplace(t *testing.T) {
	manifests := func() []*mbapi.Manifest {
		return []*mbapi.Manifest{
			{Provider: "ktw-b", Contributions: []contribapi.Contribution{sharedEntry("ktw-b")}},
			{Provider: "ktw-a", Contributions: []contribapi.Contribution{sharedEntry("ktw-a")}},
		}
	}
	cat := NewMemoryCatalog(manifests(), nil)
	uri := sharedEntry("").ResolvedURI()
	for i := 0; i < 20; i++ {
		e, ok := cat.ContributionAt(uri)
		if !ok || string(e.Data()) != `{"name":"twice","from":"ktw-b"}` {
			t.Fatalf("replace %d: ContributionAt answered %s, want the first member's copy", i, e.Data())
		}
		cat.Replace(manifests(), nil)
	}
	cols := cat.URICollisions()
	if len(cols) != 1 || cols[0].Host != "ktw-b" || len(cols[0].Shadowed) != 1 || cols[0].Shadowed[0] != "ktw-a" {
		t.Fatalf("collisions = %+v, want one hosted by ktw-b shadowing ktw-a", cols)
	}
	byType, _ := cat.GetContributionsByType(context.Background(), "recipe")
	if len(byType) != 1 {
		t.Fatalf("by-type listed %d copies of one URI, want 1", len(byType))
	}
}

// --- L6: reads proceed while a slow validation runs ---

// gatedValidator blocks the first ValidateBody until released, so a test can
// hold a reindex inside its schema-service call and ask whether reads wait.
type gatedValidator struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (g *gatedValidator) GetDataTypeByURI(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"root":"T","types":{"T":{"kind":"object","fields":{}}}}`), nil
}

func (g *gatedValidator) ValidateBody(context.Context, string, json.RawMessage) error {
	g.once.Do(func() { close(g.started) })
	<-g.release
	return nil
}

// Before: Reindex held the catalog's write lock across every schema-service
// call, so one slow validation stalled every read of the universe. Now the
// verdicts are computed with no lock held and applied afterwards.
func TestReindex_ReadsProceedDuringASlowValidation(t *testing.T) {
	cache := NewCache(t.TempDir())
	m := makeManifest("ktw-x", "1.0.0", "sha256:x")
	entry := contribapi.Contribution{Provider: "ktw-x", Type: "recipe", ID: "r1",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"r1"}`))}
	m.Contributions = []contribapi.Contribution{
		entry,
		memberKindDecl("ktw-x", "recipe", "ktw-x:/data-type/recipe", "error"),
	}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "ktw-x"))
	if err != nil {
		t.Fatal(err)
	}
	gate := &gatedValidator{started: make(chan struct{}), release: make(chan struct{})}
	if err := cat.SetSchema(gate); err != nil {
		t.Fatal(err)
	}

	reindexDone := make(chan error, 1)
	go func() { reindexDone <- cat.Reindex(context.Background(), nil) }()
	<-gate.started // the reindex is inside the schema service now

	readsDone := make(chan struct{})
	go func() {
		defer close(readsDone)
		ctx := context.Background()
		if _, err := cat.Get(ctx, "ktw-x"); err != nil {
			t.Errorf("Get during validation: %v", err)
		}
		if _, err := cat.List(ctx); err != nil {
			t.Errorf("List during validation: %v", err)
		}
		if _, ok := cat.ContributionAt(entry.ResolvedURI()); !ok {
			t.Error("ContributionAt during validation: not found")
		}
		cat.HostOf(entry.TypedURI())
	}()
	select {
	case <-readsDone:
	case <-time.After(3 * time.Second):
		t.Fatal("reads blocked while Reindex was inside the schema service: the write lock spans validation")
	}

	close(gate.release)
	if err := <-reindexDone; err != nil {
		t.Fatal(err)
	}
	got := entryAt(t, cat, entry.ResolvedURI())
	if got.Valid == nil || !*got.Valid {
		t.Fatalf("after the gated validation the verdict is valid=%v errors=%v, want an explicit pass", got.Valid, got.ValidationErrors)
	}
}

// A Reload while a reindex is between its phases must not receive the old
// verdicts: the manifest the verdict was computed for is no longer the loaded
// one, so it is left alone (its own reindex follows).
func TestReindex_VerdictsAreNotAppliedToAManifestReloadedMeanwhile(t *testing.T) {
	cache := NewCache(t.TempDir())
	m := makeManifest("ktw-x", "1.0.0", "sha256:x")
	entry := contribapi.Contribution{Provider: "ktw-x", Type: "recipe", ID: "r1",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"r1"}`))}
	m.Contributions = []contribapi.Contribution{entry, memberKindDecl("ktw-x", "recipe", "ktw-x:/data-type/recipe", "error")}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "ktw-x"))
	if err != nil {
		t.Fatal(err)
	}
	gate := &gatedValidator{started: make(chan struct{}), release: make(chan struct{})}
	_ = cat.SetSchema(gate)
	done := make(chan error, 1)
	go func() { done <- cat.Reindex(context.Background(), nil) }()
	<-gate.started
	if err := cat.Reload(); err != nil { // swaps every loaded manifest
		t.Fatal(err)
	}
	close(gate.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got := entryAt(t, cat, entry.ResolvedURI())
	if got.Valid != nil {
		t.Fatalf("a verdict computed for the previous load was stamped on the reloaded manifest: valid=%v", *got.Valid)
	}
}

// --- L2: a same-URI merge is a finding, never silent ---

func TestDedupeByURI_ReportsEveryMerge(t *testing.T) {
	a := contribapi.Contribution{Provider: "p", Type: "config-file", ID: "X.yaml"}
	b := contribapi.Contribution{Provider: "p", Type: "config-file", ID: "X.yaml"}
	b.SetData(json.RawMessage(`{"hasDefault":true}`))
	c := contribapi.Contribution{Provider: "p", Type: "skill", ID: "alone"}
	out, drops := dedupeByURI([]contribapi.Contribution{a, b, c}, nil)
	if len(out) != 2 {
		t.Fatalf("kept %d entries, want 2", len(out))
	}
	if len(drops) != 1 || drops[0].URI != "p:/config-file/X.yaml" || drops[0].Copies != 2 {
		t.Fatalf("drops = %+v, want the one merged URI with 2 copies", drops)
	}
	if len(out[0].Data()) == 0 {
		t.Fatal("the config-file copy with a default lost to the empty one")
	}
	_, none := dedupeByURI([]contribapi.Contribution{a, c}, nil)
	if len(none) != 0 {
		t.Fatalf("no duplicate, yet drops = %+v", none)
	}
}

// --- M7: skipped and unchecked share one wire form, by decision ---

// The envelope carries no `validation` status field: a check that was
// skipped (severity off, no data layer, a bulk-band body) records nothing —
// Valid nil, no errors — exactly as an entry nothing checked. validation.md §2
// states it as the contract ("`valid` absent ... and no more"); this holds the
// code to it, so a status field, if one is ever added, is added on purpose.
func TestSkippedVerdict_IsAbsentOnTheWire_LikeUnchecked(t *testing.T) {
	ctx := context.Background()
	withData := contribapi.Contribution{Provider: "p", Type: "recipe", ID: "r",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"r"}`))}
	noData := contribapi.Contribution{Provider: "p", Type: "blob", ID: "b",
		Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: {Hash: "sha256:abc", Size: 3}}}
	bulk := contribapi.Contribution{Provider: "p", Type: "dataset", ID: "d",
		Layers: contribapi.DataDescriptor("sha256:big", 64<<20)} // bulk band: descriptor only, no bytes
	unchecked := contribapi.Contribution{Provider: "p", Type: "recipe", ID: "u",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"u"}`))}

	cases := []struct {
		name     string
		entry    contribapi.Contribution
		severity string
	}{
		{"severity off", withData, contribapi.ValidateOff},
		{"no data layer", noData, contribapi.ValidateOn},
		{"bulk-band body", bulk, contribapi.ValidateOn},
	}
	for _, tc := range cases {
		out := ValidateContribution(ctx, tc.severity, tc.entry, "p:/data-type/x", nil)
		if !out.Skipped {
			t.Fatalf("%s: not skipped (%+v)", tc.name, out)
		}
		valid, errs := validationVerdict(ctx, tc.entry, "p:/data-type/x", tc.severity, nil, nil)
		if valid != nil || errs != nil {
			t.Fatalf("%s: skipped verdict recorded valid=%v errs=%v, want nothing", tc.name, valid, errs)
		}
		stamped := tc.entry
		stamped.Valid, stamped.ValidationErrors = valid, errs
		wire, _ := json.Marshal(stamped)
		var keys map[string]json.RawMessage
		_ = json.Unmarshal(wire, &keys)
		if _, has := keys["valid"]; has {
			t.Fatalf("%s: `valid` on the wire for a skipped check", tc.name)
		}
		if _, has := keys["validationErrors"]; has {
			t.Fatalf("%s: `validationErrors` on the wire for a skipped check", tc.name)
		}
	}
	// The unchecked entry reads the same: no verdict field at all.
	wire, _ := json.Marshal(unchecked)
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(wire, &keys)
	if _, has := keys["valid"]; has {
		t.Fatal("an entry nothing validated carries `valid`")
	}
}
