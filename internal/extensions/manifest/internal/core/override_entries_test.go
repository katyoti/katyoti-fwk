// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// overrideEntryProvider holds override entries, as a project's object-backed
// provider does.
type overrideEntryProvider struct {
	staticProvider
	ovs []contribapi.OverrideEntry
}

func (p overrideEntryProvider) OverrideEntries() []contribapi.OverrideEntry { return p.ovs }

// executableKind declares kind as executable, as the task kind does.
func executableKind(kind string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "system", Type: "contribution-type", ID: kind}
	c.SetData(json.RawMessage(`{"name":"` + kind + `","execution":"executable"}`))
	return c
}

// overrideOf is the override entry of project acme replacing layers of uri.
func overrideOf(uri, kind, id string, trusted bool, layers map[string]string) contribapi.OverrideEntry {
	e := contribapi.Contribution{Provider: "acme", Type: kind, ID: id}
	for name, text := range layers {
		e.SetLayer(name, []byte(text), "text/markdown")
	}
	return contribapi.OverrideEntry{Overrides: uri, Entry: e, Origin: contribapi.ProjectOrigin("acme"), Object: "acme.obj-" + id, Trusted: trusted}
}

// An override entry takes effect over the entry it names: it replaces only
// its layers, inherits the others, and each replaced layer is recorded with
// the project as its origin; a layer the universe's tree served keeps its
// last word; an untrusted project's override of an executable kind is
// refused; an override of an entry this build does not serve is carried in
// the manifest for the aggregator.
func TestAnOverrideEntryTakesEffectByOrigin(t *testing.T) {
	persona := contribapi.Contribution{Provider: "ktw-x", Type: "persona", ID: "reviewer"}
	persona.SetLayer("prompt", []byte("You review."), "text/markdown")
	persona.SetLayer("i18n.fr", []byte("Tu relis."), "text/markdown")
	persona.SetLayer("i18n.de", []byte("Du prüfst."), "text/markdown")
	task := contribapi.Contribution{Provider: "ktw-x", Type: "task", ID: "deploy"}
	task.SetLayer("source", []byte("run it"), "text/markdown")
	base := overridingProvider{
		staticProvider: staticProvider{provider: "ktw-x", typ: "persona", entries: []contribapi.Contribution{persona}},
		overrides:      []contribapi.Override{{URI: "ktw-x:/persona/reviewer", Layer: "i18n.de", From: contribapi.OriginUniverse}},
	}
	tasks := staticProvider{provider: "ktw-x", typ: "task", entries: []contribapi.Contribution{task}}
	decls := staticProvider{provider: "system", typ: "contribution-type", entries: []contribapi.Contribution{executableKind("task")}}
	project := overrideEntryProvider{
		staticProvider: staticProvider{provider: "acme", typ: "persona"},
		ovs: []contribapi.OverrideEntry{
			overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-fr", false, map[string]string{"i18n.fr": "Vous relisez.", "i18n.de": "Sie prüfen."}),
			overrideOf("ktw-x:/task/deploy", "task", "deploy", false, map[string]string{"source": "run something else"}),
			overrideOf("ktw-y:/persona/coder", "persona", "coder", false, map[string]string{"prompt": "Code."}),
		},
	}
	m, err := NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{decls, base, tasks, project})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		got[c.ResolvedURI()] = c
	}
	r := got["ktw-x:/persona/reviewer"]
	// The override's layers stay in the record (not the producer's to open);
	// the inherited ones are the producer's, as residency leaves them.
	if string(r.Layers["i18n.fr"].Inline) != "Vous relisez." || r.Layers["prompt"].Hash != contribapi.HashContent([]byte("You review.")) {
		t.Fatalf("reviewer layers = %+v; want the French replaced and the prompt inherited", r.Layers)
	}
	if r.Layers["i18n.de"].Hash != contribapi.HashContent([]byte("Du prüfst.")) {
		t.Fatalf("the German layer the universe served was replaced: %+v", r.Layers["i18n.de"])
	}
	if _, served := got["acme:/persona/reviewer-fr"]; served {
		t.Fatal("the override entry was served as an entry of its own")
	}
	var fromProject []string
	for _, o := range m.Overrides {
		if o.From == "project:acme" {
			fromProject = append(fromProject, o.URI+"#"+o.Layer)
		}
	}
	if strings.Join(fromProject, ",") != "ktw-x:/persona/reviewer#i18n.fr" {
		t.Fatalf("the project's override record = %v; want reviewer's French layer only", fromProject)
	}
	if got["ktw-x:/task/deploy"].Layers["source"].Hash != contribapi.HashContent([]byte("run it")) {
		t.Fatal("an untrusted project's override of an executable kind was applied")
	}
	refused := false
	for _, d := range m.Drops {
		if d.Path == "acme:/task/deploy" && strings.Contains(d.Reason, "does not trust") {
			refused = true
		}
	}
	if !refused {
		t.Fatalf("the refused override is not reported: drops %+v", m.Drops)
	}
	if len(m.OverrideEntries) != 1 || m.OverrideEntries[0].Overrides != "ktw-y:/persona/coder" {
		t.Fatalf("carried = %+v; want the override of the entry another member serves", m.OverrideEntries)
	}

	// Trusted, the executable override applies.
	project.ovs[1].Trusted = true
	m, err = NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{decls, base, tasks, project})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range m.Contributions {
		if c.ResolvedURI() == "ktw-x:/task/deploy" && c.Layers["source"].Hash != contribapi.HashContent([]byte("run something else")) {
			t.Fatalf("a trusted project's override of a task was not applied: %+v", c.Layers["source"])
		}
	}
}

// The catalog applies the override entries a member carries over the member
// that serves their entry, records them in that member's manifest, and names
// an override of an entry no member serves as a finding.
func TestTheCatalogAppliesCarriedOverrideEntries(t *testing.T) {
	cache := NewCache(t.TempDir())
	x := makeManifest("ktw-x", "1.0.0", "")
	persona := contribapi.Contribution{Provider: "ktw-x", Type: "persona", ID: "reviewer"}
	persona.SetLayer("prompt", []byte("You review."), "text/markdown")
	persona.SetLayer("i18n.fr", []byte("Tu relis."), "text/markdown")
	x.Contributions = []contribapi.Contribution{persona}
	ReHash(x)
	acme := makeManifest("acme", "folder", "")
	acme.OverrideEntries = []contribapi.OverrideEntry{
		overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-fr", false, map[string]string{"i18n.fr": "Vous relisez."}),
		overrideOf("ktw-x:/persona/gone", "persona", "gone", false, map[string]string{"prompt": "Nothing."}),
	}
	ReHash(acme)
	if err := SeedCacheSave(t, cache, x); err != nil {
		t.Fatal(err)
	}
	if err := SeedCacheSave(t, cache, acme); err != nil {
		t.Fatal(err)
	}
	c, err := NewCatalog(cache, MembersOf(cache, "ktw-x", "acme"))
	if err != nil {
		t.Fatal(err)
	}
	e, ok := c.ContributionAt("ktw-x:/persona/reviewer")
	if !ok {
		t.Fatal("reviewer not in the catalog")
	}
	if string(e.Layers["i18n.fr"].Inline) != "Vous relisez." || string(e.Layers["prompt"].Inline) != "You review." {
		t.Fatalf("reviewer = fr %q prompt %q; want the project's French and the binary's prompt", e.Layers["i18n.fr"].Inline, e.Layers["prompt"].Inline)
	}
	got, err := c.Get(context.Background(), "ktw-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Overrides) != 1 || got.Overrides[0] != (contribapi.Override{URI: "ktw-x:/persona/reviewer", Layer: "i18n.fr", From: "project:acme"}) {
		t.Fatalf("ktw-x's override record = %+v", got.Overrides)
	}
	f := c.OverrideFindings()
	if len(f) != 1 || !strings.Contains(f[0], "ktw-x:/persona/gone") || !strings.Contains(f[0], "no member serves") {
		t.Fatalf("findings = %v; want the override of an entry no member serves", f)
	}
}
