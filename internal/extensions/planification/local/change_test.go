// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE CHANGE LOG AS DATA — each user-visible change is an entry under its
// release; a session adds one instead of editing a page; the release's page is
// rendered from its entries; the module-update step names one.

func changesOf(t *testing.T, ctx context.Context) api.ChangeService {
	t.Helper()
	svc, err := application.UseService[api.ChangeService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// changeLogPageOf is the page of a release in the world's own project root,
// never the workspace's: a test that writes it writes its own temp page.
func changeLogPageOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, release string) string {
	t.Helper()
	root := projectRootOf(t, ctx, cwm, release)
	if repo := application.ProjectRoot(); repo != "" && filepath.Clean(root) == filepath.Clean(repo) {
		t.Fatalf("the world's project root %s is the process's own: the test would write the workspace", root)
	}
	return filepath.Join(root, "docs", "confluence-tew", "changelog", "1.4.0.md")
}

// changeLogRelease plans the project's release 1.4.0 in a stream of its own.
func changeLogRelease(t *testing.T, ctx context.Context) (release, stream string) {
	t.Helper()
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Version: "1.4.0"})
	s, err := streams(t, ctx).Create(ctx, api.CreateStreamInput{Module: api.SystemModule, Name: "Packages", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := streams(t, ctx).ReleaseIn(ctx, api.StreamInInput{ID: r.ID, Stream: s.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	return r.ID, s.Stream.ID
}

func TestChangeEntries(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	ch := changesOf(t, ctx)
	rel, stream := changeLogRelease(t, ctx)

	first, err := ch.Add(ctx, api.AddChangeInput{
		Release: "1.4.0", Title: "Les factures partent par mail", Domain: "Interface", Value: api.ValueMajor,
		Text: "**Ce que ça vous apporte :** chaque facture part seule.", Requests: []string{"billing.RQ-001", "ui.F-023"}, Date: "2026-10-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := first.Change
	if c.Release != rel || c.Stream != stream || c.Version != "1.4.0" || c.Number != 1 || c.Maturity != "Construit et testé" || c.ForWhom != "tous" ||
		strings.Join(c.Modules, ",") != "billing,ui" || c.Title != "Les factures partent par mail" || !strings.Contains(c.Text, "chaque facture") {
		t.Fatalf("the entry added: %+v; want under %s in %s, number 1, its modules from its requests, its defaults", c, rel, stream)
	}
	if _, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "x", Domain: "Interface", Value: "huge", Text: "y"}); err == nil {
		t.Error("a value none of major, notable, improvement or fix was accepted")
	}
	if _, err := ch.Add(ctx, api.AddChangeInput{Release: "9.9.9", Title: "x", Domain: "Interface", Value: api.ValueFix, Text: "y"}); err == nil {
		t.Error("an entry for a release that does not exist was accepted")
	}

	// Two writers at once never take the same number.
	var wg sync.WaitGroup
	numbers := make([]int, 6)
	for i := range numbers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Correction " + string(rune('a'+i)), Domain: "Workspace & objets",
				Value: api.ValueFix, Text: "Corrigé.", Modules: []string{"cwm"}, Date: "2026-10-05"})
			if err != nil {
				t.Error(err)
				return
			}
			numbers[i] = out.Change.Number
		}(i)
	}
	wg.Wait()
	seen := map[int]bool{1: true}
	for _, n := range numbers {
		if seen[n] {
			t.Fatalf("two entries numbered %d: %v", n, numbers)
		}
		seen[n] = true
	}

	// Queried by stream, module, period and words.
	list := func(in api.ListChangesInput) int {
		t.Helper()
		out, err := ch.List(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return len(out.Changes)
	}
	for name, got := range map[string][2]int{
		"stream":  {list(api.ListChangesInput{Stream: stream}), 7},
		"release": {list(api.ListChangesInput{Release: "1.4.0"}), 7},
		"module":  {list(api.ListChangesInput{Module: "ui"}), 1},
		"period":  {list(api.ListChangesInput{From: "2026-10-02", To: "2026-10-31"}), 6},
		"text":    {list(api.ListChangesInput{Text: "FACTURE mail"}), 1},
		"none":    {list(api.ListChangesInput{Stream: "roadmap.SM-999"}), 0},
	} {
		if got[0] != got[1] {
			t.Errorf("by %s: %d entries; want %d", name, got[0], got[1])
		}
	}

	upd, err := ch.Update(ctx, api.UpdateChangeInput{ID: c.ID, Maturity: "Disponible"})
	if err != nil || upd.Change.Maturity != "Disponible" || upd.Change.Value != api.ValueMajor || upd.Change.Title != c.Title {
		t.Fatalf("an update of its maturity: %+v, %v; want only the maturity changed", upd.Change, err)
	}
}

const pageFixture = `---
title: KaTyoti 1.4.0 — Les factures
---

!!! info
    **Version :** 1.4.0 · **Statut :** en préparation
    **Dernière mise à jour :** 02/10/2026 — changement 7 : la recherche retrouve tout, construit et testé ; 01/10/2026 — changement 5 : les factures partent par mail, construit et testé ; ouverture de la version.

## En un coup d'œil

{{changes:count}}

## Interface

### 3. Un bouton plus lisible

**Valeur :** ★ Amélioration · **Maturité :** Disponible · **Pour qui :** tous

**Ce que ça vous apporte :** le bouton se lit.

---

## Workspace & objets

### 7. La recherche retrouve tout

**Valeur :** ★★ Notable · **Maturité :** Construit et testé · **Pour qui :** architectes

**Ce que ça vous apporte :** tout objet se retrouve. [billing.RQ-002]

---

### 5. Les factures partent par mail

**Valeur :** ★★★ Majeur · **Maturité :** Construit et testé · **Pour qui :** tous

**Ce que ça vous apporte :** chaque facture part seule.

- un mail par facture

---

## Réserves

Aucune.
`

func TestChangeLogPage(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	ch := changesOf(t, ctx)
	rel, _ := changeLogRelease(t, ctx)
	imp, err := ch.Import(ctx, api.ImportChangeLogInput{Release: rel, Markdown: pageFixture})
	if err != nil || len(imp.Changes) != 3 {
		t.Fatalf("import: %+v, %v; want three entries", imp, err)
	}
	if _, err := ch.Import(ctx, api.ImportChangeLogInput{Release: rel, Markdown: pageFixture}); err == nil {
		t.Error("a second import of the same release was accepted")
	}
	got, err := ch.List(ctx, api.ListChangesInput{Text: "tout objet"})
	if err != nil || len(got.Changes) != 1 || got.Changes[0].Number != 7 || got.Changes[0].Date != "2026-10-02" ||
		got.Changes[0].ForWhom != "architectes" || strings.Join(got.Changes[0].Requests, ",") != "billing.RQ-002" || got.Changes[0].Domain != "Workspace & objets" {
		t.Fatalf("the imported entry 7: %+v, %v; want its date, who it is for, its request and its domain", got.Changes, err)
	}

	page, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := ch.Render(ctx, api.RenderChangeLogInput{Release: rel})
	if page.Markdown != again.Markdown {
		t.Fatal("two renderings of the same entries differ")
	}
	md := page.Markdown
	for _, want := range []string{
		"title: KaTyoti 1.4.0 — Les factures",
		"**Dernière mise à jour :** 02/10/2026 — changement 7 : la recherche retrouve tout, construit et testé ; 01/10/2026 — changement 5 : les factures partent par mail, construit et testé ; ouverture de la version.",
		"**3 changements** : 1 majeur · 1 notable · 1 amélioration · 0 correction",
		"### 7. La recherche retrouve tout\n\n**Valeur :** ★★ Notable · **Maturité :** Construit et testé · **Pour qui :** architectes",
		"- un mail par facture",
		"## Réserves\n\nAucune.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the page lacks %q:\n%s", want, md)
		}
	}
	// Domains in value order: the one holding the major first.
	if strings.Index(md, "## Workspace & objets") > strings.Index(md, "## Interface") {
		t.Errorf("Interface (an improvement) came before Workspace & objets (a major):\n%s", md)
	}
	if strings.Index(md, "### 7.") > strings.Index(md, "### 5.") {
		t.Error("in a domain, the newest entry did not come first")
	}

	// A change added later is on the page and in the update line.
	if _, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Un export en PDF", Domain: "Interface", Value: api.ValueNotable,
		Text: "**Ce que ça vous apporte :** un PDF.", Update: "un export en PDF, construit et testé", Date: "2026-10-03"}); err != nil {
		t.Fatal(err)
	}
	after, _ := ch.Render(ctx, api.RenderChangeLogInput{Release: rel})
	if !strings.Contains(after.Markdown, "### 8. Un export en PDF") || !strings.Contains(after.Markdown, "03/10/2026 — changement 8 : un export en PDF, construit et testé ; 02/10/2026") {
		t.Errorf("the entry added after the import is missing from the page or its update line:\n%s", after.Markdown)
	}
}

// A release whose page was never imported holds no head: its first entry
// added, a render keeps the page's frontmatter, info box and closing sections
// and records them on the release; a page listing a change the release does
// not hold is refused, never written away.
func TestRenderAdoptsAnUnimportedPageHead(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	ch := changesOf(t, ctx)
	rel, _ := changeLogRelease(t, ctx)
	path := changeLogPageOf(t, ctx, cwm, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	head := `---
title: KaTyoti 1.4.0 — Les factures
---

!!! info
    **Version :** 1.4.0 · **Statut :** prévue
    **Dernière mise à jour :** ouverture de la version.

## En un coup d'œil

{{changes:count}}
`
	tail := `
## Versions candidates

Aucune pour l'instant.
`
	if err := os.WriteFile(path, []byte(pageFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Un export en PDF", Domain: "Interface", Value: api.ValueNotable,
		Text: "**Ce que ça vous apporte :** un PDF.", Date: "2026-10-03"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel, Write: true}); err == nil || !strings.Contains(err.Error(), "changes import") {
		t.Errorf("a render over a page listing changes the release does not hold: %v; want refused, naming changes import", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != pageFixture {
		t.Fatal("the refused render wrote the page")
	}

	if err := os.WriteFile(path, []byte(head+tail), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title: KaTyoti 1.4.0 — Les factures", "**Statut :** prévue", "## En un coup d'œil", "### 1. Un export en PDF", "## Versions candidates"} {
		if !strings.Contains(out.Markdown, want) {
			t.Errorf("the rendered page lacks %q:\n%s", want, out.Markdown)
		}
	}
	// Recorded on the release: the page gone, a render still has its head.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	again, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel})
	if err != nil || !strings.Contains(again.Markdown, "**Statut :** prévue") || !strings.Contains(again.Markdown, "## Versions candidates") {
		t.Errorf("the head adopted was not recorded on the release: %v\n%s", err, again.Markdown)
	}
}

// A page rendered while it had no head leaves the release holding only blank
// lines: a head written on the page afterwards is still adopted by the next
// render, never written away.
func TestRenderAdoptsAHeadAfterABlankOne(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	ch := changesOf(t, ctx)
	rel, _ := changeLogRelease(t, ctx)
	path := changeLogPageOf(t, ctx, cwm, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Un export en PDF", Domain: "Interface", Value: api.ValueNotable,
		Text: "**Ce que ça vous apporte :** un PDF.", Date: "2026-10-03"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel, Write: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	head := "---\ntitle: KaTyoti 1.4.0 — Les factures\n---\n\n## En un coup d'œil\n\nUn export en PDF.\n\n"
	if err := os.WriteFile(path, append([]byte(head), raw...), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ch.Render(ctx, api.RenderChangeLogInput{Release: rel, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title: KaTyoti 1.4.0 — Les factures", "## En un coup d'œil", "### 1. Un export en PDF"} {
		if !strings.Contains(out.Markdown, want) {
			t.Errorf("the head written on the page after a blank one was not adopted: lacks %q\n%s", want, out.Markdown)
		}
	}
}

func TestModuleUpdateChangeEntry(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := gatedWorld(t, "2020-01-01")
	ch := changesOf(t, ctx)
	rel, _ := changeLogRelease(t, ctx)
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")
	ac := criterion(t, ctx, cwm, uc, "A mail leaves for each invoice")
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: uc, By: session}); err != nil {
		t.Fatal(err)
	}
	// A page is no longer a change-log entry.
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Changelog: "docs/confluence-tew/changelog/1.4.0.md#1", By: session}); err == nil ||
		!strings.Contains(err.Error(), "changes add") {
		t.Fatalf("a page given as the change log: %v; want refused, saying to add an entry", err)
	}
	other, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Autre chose", Domain: "Interface", Value: api.ValueFix, Text: "x", Requests: []string{"billing.RQ-999"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Changelog: other.Change.ID, Checks: []string{ac}, By: session}); err != nil {
		t.Fatal(err)
	}
	if out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: r.ID}); err != nil || out.Accept || !strings.Contains(out.Reason, "does not name "+r.ID) {
		t.Fatalf("an entry that does not name the request: %+v, %v; want refused, saying so", out, err)
	}
	mine, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: "Les factures partent par mail", Domain: "Interface", Value: api.ValueMajor, Text: "x", Requests: []string{r.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Changelog: mine.Change.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if out, _ := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: r.ID}); strings.Contains(out.Reason, "change-log entry") {
		t.Errorf("the entry naming the request was not taken: %s", out.Reason)
	}
}

// What's new: the entries recorded after the one a person last
// saw, by version, newest version first, each opening on its En un coup
// d'œil; a person who saw none gets the latest version; nothing newer, nothing.
func TestWhatsNew(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	ch := changesOf(t, ctx)
	old, _ := changeLogRelease(t, ctx)
	nw := plan(t, ctx, releases(t, ctx), api.PlanInput{Version: "1.5.0", Name: "Les relances"}).ID
	head := "---\ntitle: x\n---\n\n## En un coup d'œil\n\n- **Relancer sans y penser** — chaque facture impayée est relancée.\n\n## Versions candidates\n\n| RC |\n"
	if _, err := ch.Import(ctx, api.ImportChangeLogInput{Release: nw, Markdown: head}); err != nil {
		t.Fatal(err)
	}
	add := func(rel, title string) string {
		t.Helper()
		out, err := ch.Add(ctx, api.AddChangeInput{Release: rel, Title: title, Domain: "Interface", Value: api.ValueNotable,
			Text: "**Ce que ça vous apporte :** " + title + " pour vous.\n\n**Le progrès** : testé."})
		if err != nil {
			t.Fatal(err)
		}
		return out.Change.ID
	}
	seen := add(old, "Avant")
	add(old, "Les factures partent par mail")
	add(nw, "Les relances partent seules")

	both, err := ch.WhatsNew(ctx, api.WhatsNewInput{After: seen})
	if err != nil {
		t.Fatal(err)
	}
	md := both.Markdown
	if strings.Join(both.Versions, ",") != "1.5.0,1.4.0" || strings.Contains(md, "Avant") ||
		!strings.Contains(md, "- **Les factures partent par mail** — Les factures partent par mail pour vous.") {
		t.Fatalf("since an entry before both versions: versions %v, text:\n%s\nwant both versions newest first, only the newer entries, each with its gain", both.Versions, md)
	}
	if g, c := strings.Index(md, "### En un coup d'œil"), strings.Index(md, "Les relances partent seules"); g < 0 || g > c || !strings.Contains(md, "Relancer sans y penser") {
		t.Errorf("the version's En un coup d'œil does not come before its changes:\n%s", md)
	}
	first, _ := ch.WhatsNew(ctx, api.WhatsNewInput{})
	if strings.Join(first.Versions, ",") != "1.5.0" || first.Key != both.Key {
		t.Errorf("a person who saw nothing: versions %v key %s; want the latest version only, keyed by the newest entry", first.Versions, first.Key)
	}
	none, _ := ch.WhatsNew(ctx, api.WhatsNewInput{After: both.Key})
	if none.Markdown != "" || len(none.Versions) != 0 {
		t.Errorf("nothing newer than the key: %+v; want nothing", none)
	}
}

// On an installation What's new is its solution's
// letter: the document <module>.quoi-de-neuf-<lang> its install placed in the
// project - its text up to the line that opens the full change log, the
// document to open whole, and a key that changes with each version's letter.
func TestWhatsNewAnswersTheInstalledLetter(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := worldWith(t, fstest.MapFS{
		"object-types/document.yaml": {Data: []byte("name: document\nfolder: objects\n")},
	})
	ch := changesOf(t, ctx)
	letter := func(body string) {
		t.Helper()
		doc := cwmapi.Object{ID: "billing.quoi-de-neuf-fr", Type: "document", Name: "Quoi de neuf", Parent: "billing", Body: body}
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: doc.ID}); err == nil {
			if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: doc, Revision: got.Revision}); err != nil {
				t.Fatal(err)
			}
			return
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: doc}); err != nil {
			t.Fatal(err)
		}
	}
	letter("# Quoi de neuf — 2026.10-rc1\n\n- **Les études** — une étude se crée en deux champs.\n\n---\n\n# Journal des changements\n\n### 1. Tout le détail\n")

	first, err := ch.WhatsNew(ctx, api.WhatsNewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Document != "billing.quoi-de-neuf-fr" || !strings.Contains(first.Markdown, "une étude se crée") ||
		strings.Contains(first.Markdown, "Tout le détail") || strings.Join(first.Versions, ",") != "Quoi de neuf — 2026.10-rc1" || first.Key == "" {
		t.Fatalf("an installed letter: %+v; want its text before the full log, the document, its title as its version and a key", first)
	}
	if again, _ := ch.WhatsNew(ctx, api.WhatsNewInput{After: first.Key}); again.Markdown != "" {
		t.Errorf("the letter seen is shown again: %+v", again)
	}
	letter("# Quoi de neuf — 2026.10-rc2\n\n- **Les guides** — l'onglet Guides ne propose que les vôtres.\n\n---\n\n# Journal\n")
	next, _ := ch.WhatsNew(ctx, api.WhatsNewInput{After: first.Key})
	if next.Key == first.Key || !strings.Contains(next.Markdown, "onglet Guides") {
		t.Errorf("the next version's letter after the first was seen: %+v; want it, under a new key", next)
	}
}
