// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	decktesting "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/testing"
	decklocal "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/local"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// routeDeck routes the pack's lint hook to the DeckService, as the action
// service does in a universe.
type routeDeck struct{}

func (routeDeck) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (routeDeck) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI != "ktw-docs:deck/lint-object" {
		return actionsapi.RunOutput{}, fmt.Errorf("no route for %s", in.URI)
	}
	svc, err := application.UseService[deckapi.DeckService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var hin deckapi.DeckObjectInput
	if err := json.Unmarshal(in.Input, &hin); err != nil {
		return actionsapi.RunOutput{}, err
	}
	out, err := svc.LintObject(ctx, hin)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

func world(t *testing.T) (context.Context, cwmapi.CWMService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		verificationlocal.Extension(),
		cwmapp.Pack(fstest.MapFS{"capabilities/content.yaml": {Data: []byte("name: content\n")}}),
		contrib.Pack("ktw-test"),
		contrib.ContributeLayouts("ktw-test"),
		decklocal.Extension(),
		panelocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return routeDeck{}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "talks")
	return ctx, cwm
}

func deckObject(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, local, slides string) cwmapi.Object {
	t.Helper()
	body, _ := json.Marshal(slides)
	obj := cwmapi.NewRecordObject("talks", local, "slide-deck", "talks", "A talk", body)
	obj.Capabilities["deck"] = map[string]any{"aspect": "4:3"}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "talks", Object: obj})
	if err != nil {
		t.Fatalf("create %s: %v", local, err)
	}
	return out.Object
}

// `object-lint` on a deck object runs the deck lint through the pack's hook: a
// clean deck has no finding, a broken one names the slide and the rule.
func TestLintingADeckObjectChecksItsDeck(t *testing.T) {
	ctx, cwm := world(t)
	good := deckObject(t, ctx, cwm, "good", "<!-- slide: layout=cover -->\n# A talk\nSubtitle\n\n---\n\n## Two\n- a")
	bad := deckObject(t, ctx, cwm, "bad", "# One\n\n---\n\n<!-- slide: layout=hero -->\n## Two")

	out, err := verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: good.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.Runs == 0 || len(out.Findings) != 0 {
		t.Fatalf("clean deck: runs=%d findings=%+v", out.Runs, out.Findings)
	}
	out, err = verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: bad.ID})
	if err != nil {
		t.Fatal(err)
	}
	var hit bool
	for _, f := range out.Findings {
		hit = hit || (f.Rule == "layout-unknown" && strings.HasPrefix(f.Message, "slide 2: "))
	}
	if !hit {
		t.Fatalf("broken deck: want layout-unknown on slide 2, got %+v", out.Findings)
	}
}

// The Deck panel and the lint compose the same deck from the object.
func TestThePanelAndTheLintComposeTheSameDeck(t *testing.T) {
	ctx, cwm := world(t)
	slides := "<!-- slide: layout=cover -->\n# A talk\n\n---\n\n## Two\n\nNote:\nsay it"
	obj := deckObject(t, ctx, cwm, "same", slides)
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := pane.Detail(ctx, paneapi.DetailInput{ID: obj.ID})
	if err != nil {
		t.Fatal(err)
	}
	panel := panelDeck(t, ctx, detail)
	a, b := deckapi.Parse(panel), deckapi.Parse(deckapi.ComposeDeck(deckapi.DeckObjectInput{
		Title: "A talk", Deck: deckapi.DeckObjectFields{Aspect: "4:3"}, Body: slides,
	}))
	if len(a.Slides) != 2 || len(a.Slides) != len(b.Slides) {
		t.Fatalf("slides: panel %d, lint %d", len(a.Slides), len(b.Slides))
	}
	// Lines differ (the panel keeps a key for every setting, empty or not);
	// what a slide shows does not.
	shown := func(s deckapi.Slide) string {
		var parts []string
		for _, c := range s.Slots {
			parts = append(parts, c.Name+"="+c.Markdown)
		}
		return strings.Join(parts, "|")
	}
	for i := range a.Slides {
		ja, jb := shown(a.Slides[i]), shown(b.Slides[i])
		if string(ja) != string(jb) || a.Slides[i].Notes != b.Slides[i].Notes {
			t.Errorf("slide %d differs:\n panel %s\n lint  %s", i+1, ja, jb)
		}
	}
	for _, k := range []string{"title", "aspect"} {
		if a.Settings[k] != b.Settings[k] {
			t.Errorf("%s: panel %v, lint %v", k, a.Settings[k], b.Settings[k])
		}
	}
}

// Every Deck setting the panel shows, the lint sees: an object with all of
// them set composes, in the panel and in the lint, the same frontmatter —
// each key of DeckObjectFields (a lint that skipped watermark and mark let
// `mark: maybe` pass while the view refused it).
func TestTheLintSeesEveryDeckSettingThePanelShows(t *testing.T) {
	ctx, cwm := world(t)
	fields := map[string]any{"aspect": "4:3", "theme": "katyoti", "transition": "fade", "author": "Ann", "watermark": "BROUILLON", "mark": "off"}
	body, _ := json.Marshal("# A talk\n\n---\n\n## Two\n")
	obj := cwmapi.NewRecordObject("talks", "every", "slide-deck", "talks", "A talk", body)
	obj.Capabilities["deck"] = fields
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "talks", Object: obj})
	if err != nil {
		t.Fatal(err)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := pane.Detail(ctx, paneapi.DetailInput{ID: created.Object.ID})
	if err != nil {
		t.Fatal(err)
	}
	panel := panelDeck(t, ctx, detail)
	var deck deckapi.DeckObjectFields
	raw, _ := json.Marshal(fields)
	_ = json.Unmarshal(raw, &deck)
	a := deckapi.Parse(panel).Settings
	b := deckapi.Parse(deckapi.ComposeDeck(deckapi.DeckObjectInput{Title: "A talk", Deck: deck, Body: "# A talk\n"})).Settings
	for k, want := range fields {
		if fmt.Sprint(a[k]) != want || fmt.Sprint(b[k]) != want {
			t.Errorf("%s: panel %v, lint %v, want %v", k, a[k], b[k], want)
		}
	}
}

// panelDeck is the deck the Deck panel shows: the pane names its source,
// deck/view, which the GUI calls with the object id.
func panelDeck(t *testing.T, ctx context.Context, detail paneapi.DetailOutput) string {
	t.Helper()
	for _, p := range detail.Panels {
		if p.Name != "Deck" {
			continue
		}
		if p.Source != "ktw-docs:deck/view" {
			t.Fatalf("the Deck panel's source is %q, want ktw-docs:deck/view", p.Source)
		}
		svc, err := application.UseService[deckapi.DeckService](ctx)
		if err != nil {
			t.Fatal(err)
		}
		def, err := svc.View(ctx, deckapi.ViewInput{ID: detail.Object.ID})
		if err != nil {
			t.Fatal(err)
		}
		md, _ := def.Data.(string)
		return md
	}
	t.Fatal("no Deck panel")
	return ""
}

// The deck the Deck panel shows carries the production mark — the one
// definition, docsapi.MarkOf — unless the deck's mark is off.
func TestTheDeckViewCarriesTheProductionMark(t *testing.T) {
	ctx, cwm := world(t)
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	on := deckObject(t, ctx, cwm, "marked", "# A talk\n")
	detail, err := pane.Detail(ctx, paneapi.DetailInput{ID: on.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(deckapi.Parse(panelDeck(t, ctx, detail)).Settings["production_mark"])
	if !strings.HasPrefix(got, docsapi.DefaultMarkText+" · ") {
		t.Fatalf("production_mark = %q, want the docs mark", got)
	}
	body, _ := json.Marshal("# A talk\n")
	obj := cwmapi.NewRecordObject("talks", "unmarked", "slide-deck", "talks", "A talk", body)
	obj.Capabilities["deck"] = map[string]any{"mark": "off"}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "talks", Object: obj})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err = pane.Detail(ctx, paneapi.DetailInput{ID: created.Object.ID}); err != nil {
		t.Fatal(err)
	}
	if v, ok := deckapi.Parse(panelDeck(t, ctx, detail)).Settings["production_mark"]; ok {
		t.Fatalf("a deck whose mark is off carries %v", v)
	}
}

// verificationOf is the verification service of the test's application: it
// reads the lint and verify points CWM runs.
func verificationOf(t *testing.T, ctx context.Context) verificationapi.VerificationService {
	t.Helper()
	v, err := application.UseService[verificationapi.VerificationService](ctx)
	if err != nil {
		t.Fatalf("resolve VerificationService: %v", err)
	}
	return v
}

// ImportTheme is the importer behind the service: it refuses an input
// without a template or a name, and a file that is not a PowerPoint.
func TestImportThemeRefusesWhatItCannotRead(t *testing.T) {
	ctx, _ := world(t)
	svc, err := application.UseService[deckapi.DeckService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportTheme(ctx, deckapi.ImportThemeInput{Template: "x.pptx"}); err == nil {
		t.Error("an import without a name was accepted")
	}
	notPPTX := filepath.Join(t.TempDir(), "notes.pptx")
	if err := os.WriteFile(notPPTX, []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ImportTheme(ctx, deckapi.ImportThemeInput{Template: notPPTX, Name: "acme"})
	if err == nil || !strings.Contains(err.Error(), "not a PowerPoint") {
		t.Errorf("a text file: err = %v", err)
	}
}

// A template imported into a module is a deck-theme object there: the
// resolver finds it before any sidecar theme and reads its files from the
// module, a deck object naming it lints clean, and a second import of the
// same name is refused.
func TestImportThemeIntoAModule(t *testing.T) {
	ctx, cwm := world(t)
	svc, err := application.UseService[deckapi.DeckService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	tpl := filepath.Join(t.TempDir(), "acme.pptx")
	if err := os.WriteFile(tpl, decktesting.Template(t), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := svc.ImportTheme(ctx, deckapi.ImportThemeInput{Template: tpl, Name: "acme", Module: "talks"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Object == "" || len(out.Written) < 3 {
		t.Fatalf("import = object %q, written %v", out.Object, out.Written)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "talks", ID: out.Object})
	if err != nil {
		t.Fatal(err)
	}
	if got.Object.Type != deckapi.ObjectTypeDeckTheme {
		t.Fatalf("object type = %q", got.Object.Type)
	}
	ref, err := cwm.ObjectFileRead(ctx, cwmapi.ObjectFileReadInput{ID: out.Object, Path: "themes/acme/reference.pptx"})
	if err != nil || len(ref.Content) == 0 {
		t.Fatalf("reference.pptx: %v (%d bytes)", err, len(ref.Content))
	}

	themes, err := deckapi.Themes(ctx, mbapi.Q{Reach: mbapi.ReachSelf})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, th := range themes {
		found = found || (th.Body.Name == "acme" && th.Provider == deckapi.ProviderProject)
	}
	if !found {
		t.Fatalf("themes in reach miss the project theme: %v", themes)
	}
	resolved, err := deckapi.ResolveTheme(ctx, mbapi.Q{Reach: mbapi.ReachSelf}, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Assets["logo.png"]) == 0 || len(resolved.Assets["sample.deck.md"]) == 0 {
		t.Fatalf("resolved files = %d (logo %d bytes)", len(resolved.Assets), len(resolved.Assets["logo.png"]))
	}

	deck := deckObject(t, ctx, cwm, "acme-talk", "<!-- slide: layout=cover variant=cream -->\n# A talk\nSubtitle")
	deck.Capabilities["deck"] = map[string]any{"aspect": "16:9", "theme": "acme"}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "talks", Object: deck}); err != nil {
		t.Fatal(err)
	}
	lint, err := verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: deck.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range lint.Findings {
		if strings.Contains(f.Rule, "theme") || strings.Contains(f.Rule, "variant") {
			t.Errorf("a deck in the project theme: %s %s", f.Rule, f.Message)
		}
	}

	if _, err := svc.ImportTheme(ctx, deckapi.ImportThemeInput{Template: tpl, Name: "acme", Module: "talks"}); err == nil {
		t.Error("a second theme of the same name was accepted")
	}
}

// A deck offers its actions — translate, a short version — each a task run
// on the deck (the pane's Actions list them).
func TestDeckTypeOffersItsActions(t *testing.T) {
	ctx, cwm := world(t)
	out, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: "slide-deck"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range out.ObjectType.Actions {
		got[a.URI] = true
	}
	for _, want := range []string{"ktw-docs:tasks/deck-translate", "ktw-docs:tasks/deck-shorten"} {
		if !got[want] {
			t.Errorf("slide-deck actions miss %s: %+v", want, out.ObjectType.Actions)
		}
	}
}
