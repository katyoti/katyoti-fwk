// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/contrib"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	reviewscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/contrib"
	reviewslocal "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE REVIEWS PACK, AS SHIPPED, on a real CWM: a module (reviewable by
// attachment), a review filed under it with a finding about the module and a
// document, the panels' sources and the object pane reading them.

type world struct {
	ctx  context.Context
	cwm  cwmapi.CWMService
	svc  api.ReviewsService
	pane paneapi.ObjectPaneService
}

func setup(t *testing.T) world { return setupWith(t, nil) }

// setupWith is setup with more files in the pack that declares the others
// (review types, say), and more extensions (a fake action service, say).
func setupWith(t *testing.T, extra fstest.MapFS, exts ...application.Extension) world {
	t.Helper()
	// What the reviews pack names but does not declare: the module, the
	// document and the slide deck it holds, the lifecycle its workflows move,
	// the file-overview capability and panel — the shipped declarations where
	// they exist.
	files := fstest.MapFS{
		"object-types/module.yaml":         {Data: []byte("name: module\nlabel: Module\ntree: node\ncapabilities: [module]\n")},
		"object-types/document.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-types/document.yaml")},
		"object-types/slide-deck.yaml":     {Data: []byte("name: slide-deck\nlabel: Slide deck\nfolder: decks\n")},
		"capabilities/lifecycle.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/lifecycle.yaml")},
		"capabilities/file-overview.yaml":  {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/file-overview.yaml")},
		"object-panels/file-overview.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-panels/file-overview.yaml")},
	}
	for k, v := range extra {
		files[k] = v
	}
	others := cwmapp.Pack(files)
	ctx, cwm := cwmapp.New(t, append([]application.Extension{cwmcontrib.CorePack(""), reviewscontrib.Pack(""), others, reviewslocal.Extension(), panelocal.Extension()}, exts...)...)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.ReviewsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return world{ctx, cwm, svc, pane}
}

func (w world) create(t *testing.T, o cwmapi.Object) {
	t.Helper()
	if o.Capabilities == nil {
		o.Capabilities = map[string]map[string]any{}
	}
	if _, err := w.cwm.ObjectCreate(w.ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create %s: %v", o.ID, err)
	}
}

func asJSON(t *testing.T, d render.Def) string {
	t.Helper()
	if err := d.Validate(); err != nil {
		t.Fatalf("the page must display: %v", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (w world) panels(t *testing.T, id string) map[string]paneapi.ObjectPanel {
	t.Helper()
	d, err := w.pane.Detail(w.ctx, paneapi.DetailInput{ID: id})
	if err != nil {
		t.Fatalf("Detail(%s): %v", id, err)
	}
	out := map[string]paneapi.ObjectPanel{}
	for _, p := range d.Panels {
		out[p.ID] = p
	}
	return out
}

// The pack loads: both types with their fields, both workflows, and the
// reviewable capability attached to every module.
func TestTheTypesWorkflowsAndCapabilityLoad(t *testing.T) {
	w := setup(t)
	for typ, wf := range map[string]string{api.ReviewType: "review", api.FindingType: "finding"} {
		got, err := w.cwm.ObjectTypeGet(w.ctx, cwmapi.ObjectTypeGetInput{Name: typ})
		if err != nil {
			t.Fatalf("type %s: %v", typ, err)
		}
		if got.ObjectType.Workflow != wf || !strings.HasSuffix(got.ObjectType.Fields, "/data-type/"+typ) {
			t.Errorf("type %s: workflow %q fields %q", typ, got.ObjectType.Workflow, got.ObjectType.Fields)
		}
	}
	ok, err := cwmapi.HasCapability(w.ctx, w.cwm, "m", api.Capability)
	if err != nil || !ok {
		t.Fatalf("a module is reviewable by attachment: %v %v", ok, err)
	}
}

// The Reviews panel is a tab of a reviewable object, sourced from this
// service; a review's pane has the Review panel and its Overview.
func TestDetailShowsTheReviewsPanelOnAReviewableObject(t *testing.T) {
	w := setup(t)
	p, ok := w.panels(t, "m")["tab:reviews"]
	if !ok {
		t.Fatalf("no Reviews panel on a module: %v", w.panels(t, "m"))
	}
	if p.Kind != paneapi.PanelKindSource || p.Source != "ktw-test:reviews/reviews-of" {
		t.Errorf("Reviews panel: kind %q source %q", p.Kind, p.Source)
	}
	w.create(t, cwmapi.Object{ID: "m.rv1", Type: api.ReviewType, Parent: "m", Name: "Review 1"})
	got := w.panels(t, "m.rv1")
	if p := got["tab:review"]; p.Source != "ktw-test:reviews/review" {
		t.Errorf("Review panel on a review: %+v (strip %v)", p, got)
	}
	if _, ok := got["tab:file-overview"]; !ok {
		t.Errorf("a review has its overview: %v", got)
	}
	if _, ok := got["tab:reviews"]; ok {
		t.Errorf("a review is not reviewable itself: %v", got)
	}
}

// A review is not drawn in the project tree: the drawn tree of a module that
// holds reviews has no review row, while the module's Reviews panel still
// lists them.
func TestTheDrawnTreeHasNoReviewWhileTheReviewsPanelListsThem(t *testing.T) {
	w := setup(t)
	w.create(t, cwmapi.Object{ID: "m.rv1", Type: api.ReviewType, Parent: "m", Name: "go-checks review",
		Fields: map[string]any{"reviewed_at": "2026-11-06T09:00:00Z", "rating": "good"}})

	tree, err := w.cwm.ObjectTree(w.ctx, cwmapi.ObjectTreeInput{Root: "m", DrawnOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var walk func([]cwmapi.ObjectTreeNode)
	walk = func(ns []cwmapi.ObjectTreeNode) {
		for _, n := range ns {
			if n.Object.Type == api.ReviewType {
				t.Errorf("the drawn tree draws the review %s", n.Object.ID)
			}
			walk(n.Children)
		}
	}
	walk(tree.Roots)

	list, err := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if page := asJSON(t, list); !strings.Contains(page, `"navigate":{"object":"m.rv1"`) {
		t.Errorf("the Reviews panel no longer lists the review:\n%s", page)
	}
}

// A review with a finding about the module and a document: both panels read
// them back, and the finding's state moves through its workflow.
func TestAReviewWithAFindingAndADocument(t *testing.T) {
	w := setup(t)
	w.create(t, cwmapi.Object{
		ID: "m.rv1", Type: api.ReviewType, Parent: "m", Name: "Security review",
		Fields: map[string]any{"scope": "internal/extensions/docs", "reviewed_at": "2026-03-18T10:00:00Z", "reviewers": []any{"claude"}, "rating": "critical", "summary": "Unauthenticated shutdown."},
	})
	w.create(t, cwmapi.Object{
		ID: "m.fd1", Type: api.FindingType, Parent: "m.rv1", Name: "Unauthenticated /shutdown",
		Fields: map[string]any{"severity": "critical", "statement": "Any client can stop the service.", "location": "server/server.go:193"},
		Edges:  []cwmapi.ObjectEdge{{Kind: api.EdgeAbout, To: "m"}},
	})
	w.create(t, cwmapi.Object{ID: "m.fd2", Type: api.FindingType, Parent: "m.rv1", Name: "World-readable outputs",
		Fields: map[string]any{"severity": "medium"}})
	w.create(t, cwmapi.Object{ID: "m.doc1", Type: api.DocumentType, Parent: "m.rv1", Name: "Security analysis"})
	w.create(t, cwmapi.Object{ID: "m.rv0", Type: api.ReviewType, Parent: "m", Name: "Earlier review",
		Fields: map[string]any{"reviewed_at": "2026-03-12T09:00:00Z", "rating": "good"}})

	list, err := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	page := asJSON(t, list)
	for _, want := range []string{`"text":"critical"`, `"text":"1 critical"`, `"text":"1 medium"`, `2 finding(s), 2 open`,
		`"navigate":{"object":"m.rv1"`, `"create":{"type":"review","parent":"m"}`, `"text":"good"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Reviews page lacks %s:\n%s", want, page)
		}
	}
	if strings.Index(page, "m.rv1") > strings.Index(page, "m.rv0") {
		t.Errorf("the newest review comes first:\n%s", page)
	}

	one, err := w.svc.Review(w.ctx, api.ObjectInput{ID: "m.rv1"})
	if err != nil {
		t.Fatal(err)
	}
	page = asJSON(t, one)
	for _, want := range []string{`"navigate":{"object":"m.fd1"`, `about m`, `Any client can stop the service.`,
		`"navigate":{"object":"m.doc1"`, `"create":{"type":"finding","parent":"m.rv1"}`, `"create":{"type":"slide-deck","parent":"m.rv1"}`} {
		if !strings.Contains(page, want) {
			t.Errorf("Review page lacks %s:\n%s", want, page)
		}
	}
	// The critical finding is listed before the medium one.
	if strings.Index(page, "m.fd1") > strings.Index(page, "m.fd2") {
		t.Errorf("findings are ordered by severity:\n%s", page)
	}
	if _, err := w.svc.Review(w.ctx, api.ObjectInput{ID: "m.doc1"}); err == nil {
		t.Error("a document is no review")
	}

	// The finding moves: addressed then closed; it no longer counts as open.
	for _, to := range []string{api.FindingAddressed, api.FindingClosed} {
		if _, err := w.cwm.ObjectTransition(w.ctx, cwmapi.ObjectTransitionInput{ID: "m.fd1", To: to, By: cwmapi.Actor{ID: "alice", Kind: "person"}}); err != nil {
			t.Fatalf("finding to %s: %v", to, err)
		}
	}
	if _, err := w.cwm.ObjectTransition(w.ctx, cwmapi.ObjectTransitionInput{ID: "m.fd2", To: api.FindingClosed, By: cwmapi.Actor{ID: "alice", Kind: "person"}}); err == nil {
		t.Error("an open finding is not closed before it is addressed")
	}
	list, _ = w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	page = asJSON(t, list)
	if !strings.Contains(page, "2 finding(s), 1 open") || strings.Contains(page, `"text":"1 critical"`) {
		t.Errorf("a closed finding is not open:\n%s", page)
	}
}

// Both review pages answer in the person's language: French when the panel
// asks for it, English by default; the data (titles, ratings, severities) as
// written.
func TestTheReviewPagesAnswerInThePersonsLanguage(t *testing.T) {
	w := setup(t)
	w.create(t, cwmapi.Object{
		ID: "m.rv1", Type: api.ReviewType, Parent: "m", Name: "Security review",
		Fields: map[string]any{"reviewed_at": "2026-03-18T10:00:00Z", "rating": "critical"},
	})
	w.create(t, cwmapi.Object{ID: "m.fd1", Type: api.FindingType, Parent: "m.rv1", Name: "Open shutdown",
		Fields: map[string]any{"severity": "critical"}, Edges: []cwmapi.ObjectEdge{{Kind: api.EdgeAbout, To: "m"}}})

	pages := func(lang string) (string, string) {
		list, err := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		one, err := w.svc.Review(w.ctx, api.ObjectInput{ID: "m.rv1", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		return asJSON(t, list), asJSON(t, one)
	}
	check := func(what, page string, wants ...string) {
		for _, want := range wants {
			if !strings.Contains(page, want) {
				t.Errorf("%s lacks %s:\n%s", what, want, page)
			}
		}
	}
	list, one := pages("fr")
	check("the French reviews page", list, `"title":"Revues"`, `1 constat(s), 1 ouvert(s)`, `"label":"Nouvelle revue"`, `Security review`, `"text":"critical"`)
	check("the French review page", one, `"title":"Note"`, `"title":"Constats"`, `"title":"Présentations"`, `sur m`, `Open shutdown`)
	for _, lang := range []string{"", "en"} {
		list, one = pages(lang)
		check("the English reviews page", list, `"title":"Reviews"`, `1 finding(s), 1 open`, `"label":"New review"`)
		check("the English review page", one, `"title":"Findings"`, `"title":"Slide decks"`, `about m`)
		if strings.Contains(list+one, "Revues") || strings.Contains(list+one, "Constats") {
			t.Errorf("%q: French in the English pages", lang)
		}
	}
}

// A finding filed under the module stays one object across the reviews of its
// type: the review that raised it (found-in) and the latest that saw it
// (seen-in) both list it, a review that neither raised nor saw it does not,
// and the review page shows the run's type, commit and score.
func TestAFindingUnderTheModuleIsFollowedAcrossReviews(t *testing.T) {
	w := setup(t)
	score := 72
	w.create(t, cwmapi.Object{ID: "m.rv1", Type: api.ReviewType, Parent: "m", Name: "Security, first",
		Fields: map[string]any{"type": "security", "reviewed_at": "2026-10-01T08:00:00Z", "commit": "0123456789abcdef", "trigger": "manual", "rating": "needs-attention"}})
	w.create(t, cwmapi.Object{ID: "m.rv2", Type: api.ReviewType, Parent: "m", Name: "Security, second",
		Fields: map[string]any{"type": "security", "reviewed_at": "2026-10-09T08:00:00Z", "commit": "fedcba9876543210", "trigger": "automatic", "requested_by": "m.RQ-001", "rating": "good", "score": score}})
	w.create(t, cwmapi.Object{ID: "m.rv3", Type: api.ReviewType, Parent: "m", Name: "Usability",
		Fields: map[string]any{"type": "usability", "reviewed_at": "2026-10-09T09:00:00Z", "rating": "good"}})
	fp := api.Fingerprint("security", "server/server.go", "unauthenticated shutdown")
	w.create(t, cwmapi.Object{ID: "m.fd1", Type: api.FindingType, Parent: "m", Name: "Unauthenticated /shutdown",
		Fields: map[string]any{"severity": "high", "found_by": "security", "fingerprint": fp},
		Edges:  []cwmapi.ObjectEdge{{Kind: api.EdgeAbout, To: "m"}, {Kind: api.EdgeFoundIn, To: "m.rv1"}, {Kind: api.EdgeSeenIn, To: "m.rv2"}}})

	for _, rv := range []string{"m.rv1", "m.rv2"} {
		one, err := w.svc.Review(w.ctx, api.ObjectInput{ID: rv})
		if err != nil {
			t.Fatal(err)
		}
		if page := asJSON(t, one); !strings.Contains(page, `"navigate":{"object":"m.fd1"`) {
			t.Errorf("%s lists the finding it raised or saw:\n%s", rv, page)
		}
	}
	one, err := w.svc.Review(w.ctx, api.ObjectInput{ID: "m.rv3"})
	if err != nil {
		t.Fatal(err)
	}
	if page := asJSON(t, one); strings.Contains(page, "m.fd1") {
		t.Errorf("a review of another type does not list it:\n%s", page)
	}
	one, _ = w.svc.Review(w.ctx, api.ObjectInput{ID: "m.rv2"})
	page := asJSON(t, one)
	for _, want := range []string{"security · 2026-10-09 · fedcba987", `"text":"score 72"`, `"text":"good"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Review page lacks %s:\n%s", want, page)
		}
	}
	list, _ := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	if page := asJSON(t, list); strings.Index(page, "m.rv3") > strings.Index(page, "m.rv1") {
		t.Errorf("the newest review comes first by reviewed_at:\n%s", page)
	}
}

// A fingerprint ignores case and spacing, and differs by type, location or
// key.
func TestFingerprint(t *testing.T) {
	a := api.Fingerprint("security", "server/server.go", "Unauthenticated  shutdown")
	if a != api.Fingerprint("Security", " server/server.go", "unauthenticated shutdown") {
		t.Error("case and spacing change the fingerprint")
	}
	for _, other := range []string{
		api.Fingerprint("usability", "server/server.go", "unauthenticated shutdown"),
		api.Fingerprint("security", "server/other.go", "unauthenticated shutdown"),
		api.Fingerprint("security", "server/server.go", "open shutdown"),
	} {
		if other == a {
			t.Error("a different type, location or key gives the same fingerprint")
		}
	}
	if len(a) != 16 {
		t.Errorf("fingerprint %q: want 16 hex characters", a)
	}
}

// Review types are review-type contributions of a pack: TypesFor lists those
// whose applies selects the object — by type, by capability and by a when
// predicate over its fields — grouped by dimension, with the presets they
// form; a mode keeps the manual or the automatic ones.
func TestTheReviewTypesThatApplyToAModule(t *testing.T) {
	w := setupWith(t, fstest.MapFS{
		"review-types/security.yaml":     {Data: []byte("name: security\ntitle: Security\ndimension: security\napplies: {capabilities: [reviewable]}\nrunner: {skill: 'ktw-code:/skill/code-review'}\npresets: [t-full]\n")},
		"review-types/threat-model.yaml": {Data: []byte("name: threat-model\ntitle: Threat model\ndimension: security\napplies: {types: [module]}\nrunner: {skill: 'ktw-code:/skill/code-review'}\npresets: [t-full, t-code]\n")},
		"review-types/checks.yaml":       {Data: []byte("name: checks\ntitle: Build and tests\ndimension: quality\napplies: {types: [module]}\ntrigger: {mode: automatic, kinds: [build, fix]}\nrunner: {check: 'ktw-gocode:gocode/module-audit'}\npresets: [t-code]\n")},
		"review-types/ux.yaml":           {Data: []byte("name: ux\ntitle: Usability\ndimension: usability\napplies: {types: [module], when: \"object.name == 'm'\"}\nrunner: {skill: 'ktw-agents:/skill/usability-review'}\n")},
		"review-types/elsewhere.yaml":    {Data: []byte("name: elsewhere\ntitle: Not for modules\ndimension: quality\napplies: {types: [document]}\nrunner: {skill: x}\n")},
		"review-types/not-this-one.yaml": {Data: []byte("name: not-this-one\ntitle: Another module\ndimension: usability\napplies: {types: [module], when: \"object.name == 'other'\"}\nrunner: {skill: x}\n")},
	})
	// Only this test's types: the reviews pack's own (documentation …) apply too.
	mine := map[string]bool{"security": true, "threat-model": true, "checks": true, "ux": true, "elsewhere": true, "not-this-one": true}
	names := func(out api.TypesOutput) string {
		var b strings.Builder
		for _, d := range out.Dimensions {
			var ts []string
			for _, t := range d.Types {
				if mine[t.Name] {
					ts = append(ts, " "+t.Name)
				}
			}
			if len(ts) > 0 {
				b.WriteString(d.Dimension + ":" + strings.Join(ts, "") + "; ")
			}
		}
		return b.String()
	}
	presets := func(out api.TypesOutput) []api.Preset {
		var ps []api.Preset
		for _, p := range out.Presets {
			if strings.HasPrefix(p.Name, "t-") {
				ps = append(ps, p)
			}
		}
		return ps
	}
	out, err := w.svc.TypesFor(w.ctx, api.TypesInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(out), "quality: checks; security: security threat-model; usability: ux; "; got != want {
		t.Errorf("types of m by dimension:\n got %s\nwant %s", got, want)
	}
	if ps := presets(out); len(ps) != 2 || ps[0].Name != "t-code" || strings.Join(ps[0].Types, ",") != "checks,threat-model" ||
		ps[1].Name != "t-full" || strings.Join(ps[1].Types, ",") != "security,threat-model" {
		t.Errorf("presets: %+v", ps)
	}
	auto, err := w.svc.TypesFor(w.ctx, api.TypesInput{ID: "m", Mode: api.ModeAutomatic})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(auto); got != "quality: checks; " {
		t.Errorf("automatic types: %s", got)
	}
	if c := auto.Dimensions[0].Types[0]; c.Runner.Check != "ktw-gocode:gocode/module-audit" || !c.Trigger.Automatic() || strings.Join(c.Trigger.Kinds, ",") != "build,fix" {
		t.Errorf("the checks type as declared: %+v", c)
	}
	manual, _ := w.svc.TypesFor(w.ctx, api.TypesInput{ID: "m", Mode: api.ModeManual})
	if got := names(manual); strings.Contains(got, "checks") {
		t.Errorf("a manual listing holds no automatic type: %s", got)
	}
}

// A module keeps two reviews of a type: the third supersedes the first, which
// is folded into the module's reviewable history (its rating, score, its
// findings and the open ones by severity) and removed with its own findings;
// a finding under the module it raised loses only that link; another type's
// reviews are untouched.
func TestANewReviewSupersedesTheOldestOfItsType(t *testing.T) {
	w := setup(t)
	score := 40
	review := func(id, typ, at, rating string, extra map[string]any) {
		f := map[string]any{"type": typ, "reviewed_at": at, "rating": rating}
		for k, v := range extra {
			f[k] = v
		}
		w.create(t, cwmapi.Object{ID: id, Type: api.ReviewType, Parent: "m", Name: id, Fields: f})
	}
	review("m.rv1", "security", "2026-10-01T08:00:00Z", "critical", map[string]any{"commit": "aaa", "score": score})
	w.create(t, cwmapi.Object{ID: "m.fd-own", Type: api.FindingType, Parent: "m.rv1", Name: "under the review",
		Fields: map[string]any{"severity": "high"}})
	w.create(t, cwmapi.Object{ID: "m.fd-kept", Type: api.FindingType, Parent: "m", Name: "under the module",
		Fields: map[string]any{"severity": "medium", "found_by": "security"},
		Edges:  []cwmapi.ObjectEdge{{Kind: api.EdgeFoundIn, To: "m.rv1"}}})
	review("m.rv2", "security", "2026-10-05T08:00:00Z", "needs-attention", nil)
	review("m.ux1", "usability", "2026-09-01T08:00:00Z", "good", nil)
	review("m.ux2", "usability", "2026-09-02T08:00:00Z", "good", nil)
	review("m.ux3", "usability", "2026-09-03T08:00:00Z", "good", nil)

	out, err := w.svc.Supersede(w.ctx, api.SupersedeInput{ID: "m.rv2"})
	if err != nil || out.Current != "m.rv2" || out.Previous != "m.rv1" || len(out.Folded) != 0 {
		t.Fatalf("two reviews of a type are both kept: %+v %v", out, err)
	}
	review("m.rv3", "security", "2026-10-09T08:00:00Z", "good", nil)
	out, err = w.svc.Supersede(w.ctx, api.SupersedeInput{ID: "m.rv3"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Current != "m.rv3" || out.Previous != "m.rv2" || strings.Join(out.Folded, ",") != "m.rv1" {
		t.Fatalf("the third folds the first: %+v", out)
	}
	exists := func(id string) bool {
		_, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
		return err == nil
	}
	for id, want := range map[string]bool{"m.rv1": false, "m.fd-own": false, "m.rv2": true, "m.rv3": true, "m.fd-kept": true,
		"m.ux1": true, "m.ux2": true, "m.ux3": true} {
		if exists(id) != want {
			t.Errorf("%s exists = %v, want %v", id, !want, want)
		}
	}
	kept, _ := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.fd-kept"})
	if kept.Object.EdgeTo(api.EdgeFoundIn) != "" {
		t.Errorf("the finding's link to the folded review goes: %+v", kept.Object.Edges)
	}
	mod, _ := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	b, _ := json.Marshal(mod.Object.Capabilities[api.Capability])
	var hist api.ReviewableFields
	if err := json.Unmarshal(b, &hist); err != nil || len(hist.History) != 1 {
		t.Fatalf("one history entry: %s %v", b, err)
	}
	h := hist.History[0]
	if h.Review != "m.rv1" || h.Type != "security" || h.Rating != "critical" || h.Commit != "aaa" || h.Score == nil || *h.Score != 40 ||
		h.Findings != 2 || h.Open["high"] != 1 || h.Open["medium"] != 1 {
		t.Errorf("the history entry: %+v", h)
	}
}

// The reviews pack ships the generic review types: every module can be
// reviewed for its documentation, its goals versus its state, its usability,
// and the where-are-we synthesis — each run by an agent following the
// review-dimension skill with its own guide.
func TestTheShippedGenericReviewTypes(t *testing.T) {
	w := setup(t)
	out, err := w.svc.TypesFor(w.ctx, api.TypesInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.ReviewTypeDef{}
	for _, d := range out.Dimensions {
		for _, ty := range d.Types {
			got[ty.Name] = ty
		}
	}
	for _, name := range []string{"documentation", "goals-vs-state", "usability", "where-are-we"} {
		ty, ok := got[name]
		if !ok {
			t.Errorf("no %s review type for a module: %v", name, got)
			continue
		}
		if ty.Runner.Skill != "ktw-agents:/skill/review-dimension" || ty.Runner.Input["guide"] != "ktw-agents:/reference/review/"+name+".md" || ty.Title == "" {
			t.Errorf("%s: %+v", name, ty)
		}
	}
	for _, p := range out.Presets {
		if p.Name == "full" && strings.Contains(strings.Join(p.Types, ","), "where-are-we") {
			t.Error("where-are-we runs on its own, not in the full preset")
		}
	}
}
