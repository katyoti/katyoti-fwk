// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE LISTS OF OBJECTS the pane answers as pages — Contents (what a module
// or a project holds, its whole subtree) and Objects (a search across the
// universe) — on a real CWM, with nested goals and features.

// listsWorld is a module m holding goals nested two deep, a feature under a
// goal (its lifecycle moved once) and a note, beside a module n holding a goal.
func listsWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/lifecycle.yaml": "name: lifecycle\nbadges: [{field: state, colors: {idea: gray, proposed: yellow}}]\n",
		"capabilities/project.yaml":   "name: project\n",
		"workflows/ladder.yaml": "name: ladder\ninitial: idea\nstates: [{name: idea}, {name: proposed}]\n" +
			"transitions: [{name: propose, from: [idea], to: proposed}]\n",
		"object-types/goal.yaml":    "name: goal\nlabel: Goal\nlabel_plural: Goals\ncapabilities: [lifecycle]\nworkflow: ladder\n",
		"object-types/feature.yaml": "name: feature\nlabel: Feature\nlabel_plural: Features\ncapabilities: [lifecycle]\nworkflow: ladder\n",
		"object-types/note.yaml":    "name: note\nlabel: Note\n",
		"object-types/project.yaml": "name: project\ncapabilities: [project]\ntree: root\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.g1", Type: "goal", Name: "Ship it"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.g2", Type: "goal", Name: "Ship the first half", Parent: "m.g1"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.f1", Type: "feature", Name: "Export", Parent: "m.g2"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.n1", Type: "note", Name: "A remark about export"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.f1", To: "proposed", By: cwmapi.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "n", Object: cwmapi.NewContainerObject("n", "n", "module", "", "n", "")}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "n", Object: cwmapi.Object{ID: "n.g9", Type: "goal", Name: "Elsewhere", Parent: "n"}}); err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

// pageJSON validates a page and answers it as JSON, to read what it shows.
func pageJSON(t *testing.T, d render.Def) string {
	t.Helper()
	if err := d.Validate(); err != nil {
		t.Fatalf("the page is off the grammar: %v", err)
	}
	raw, _ := json.Marshal(d)
	return string(raw)
}

// folds are the accordion folds of a page: type label → the ids it lists.
func folds(t *testing.T, d render.Def) map[string][]string {
	t.Helper()
	raw, _ := json.Marshal(d.DSLTree)
	var tree any
	_ = json.Unmarshal(raw, &tree)
	out := map[string][]string{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if n["kind"] == "accordion" {
				ps, _ := n["panels"].([]any)
				for _, p := range ps {
					pm := p.(map[string]any)
					if pm["id"] == "advanced" {
						continue // the search's Advanced form, not a group of results
					}
					label := pm["label"].(string)
					out[label] = append(out[label], navigated(pm["content"])...)
				}
				return
			}
			for _, c := range n {
				walk(c)
			}
		case []any:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(tree)
	return out
}

// navigated lists the objects a fold's rows open by their name link (the
// first link of each row).
func navigated(content any) []string {
	var out []string
	rows, _ := content.(map[string]any)["children"].([]any)
	for _, r := range rows {
		left := r.(map[string]any)["children"].([]any)[0].(map[string]any)
		link := left["children"].([]any)[0].(map[string]any)
		out = append(out, link["action"].(map[string]any)["navigate"].(map[string]any)["object"].(string))
	}
	return out
}

func TestContentsListsTheWholeSubtreeByType(t *testing.T) {
	ctx, _, svc := listsWorld(t)
	d, err := svc.Contents(ctx, api.ContentsInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	page := pageJSON(t, d)
	got := folds(t, d)
	// Every object under m, however deep — not n's, not m itself.
	if strings.Join(got["Goals"], ",") != "m.g2,m.g1" && strings.Join(got["Goals"], ",") != "m.g1,m.g2" {
		t.Errorf("goals fold = %v, want both nested goals", got["Goals"])
	}
	if strings.Join(got["Features"], ",") != "m.f1" || strings.Join(got["note"], ",") != "" && strings.Join(got["Note"], ",") != "m.n1" {
		t.Errorf("folds = %v", got)
	}
	if len(got) != 3 {
		t.Errorf("folds = %v, want Goals, Features and Note", got)
	}
	// The states as CWM answers them (initial when never moved), in their
	// colour; the parent opening it; the chips counted.
	for _, want := range []string{`"text":"proposed"`, `"color":"yellow"`, `"text":"idea"`, `"↳ Ship the first half"`,
		`"text":"Goals · 2"`, `"text":"All · 4"`, `"query":{"args":{"type":"goal"}}`, `"query":{"args":{"state":"proposed"}}`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(page, "n.g9") {
		t.Error("an object of another module is listed")
	}

	// The filter: a type, a state, a text — each re-read through the verb.
	d, _ = svc.Contents(ctx, api.ContentsInput{ID: "m", Type: "goal"})
	if got := folds(t, d); len(got) != 1 || len(got["Goals"]) != 2 {
		t.Errorf("type goal: %v", got)
	}
	d, _ = svc.Contents(ctx, api.ContentsInput{ID: "m", State: "proposed"})
	if got := folds(t, d); len(got) != 1 || strings.Join(got["Features"], ",") != "m.f1" {
		t.Errorf("state proposed: %v", got)
	}
	d, _ = svc.Contents(ctx, api.ContentsInput{ID: "m", Text: "export"})
	if got := folds(t, d); len(got) != 2 || len(got["Goals"]) != 0 {
		t.Errorf("text export: %v, want the feature and the note", got)
	}
	if !strings.Contains(pageJSON(t, d), `"filter":{"text":"export"}`) {
		t.Error("the text filter is not echoed to the form")
	}

	// Under a goal: only what hangs under it.
	d, _ = svc.Contents(ctx, api.ContentsInput{ID: "m.g1"})
	if got := folds(t, d); len(got["Goals"]) != 1 || len(got["Features"]) != 1 || len(got) != 2 {
		t.Errorf("under m.g1: %v", got)
	}
}

// A project's contents are the whole index: its modules hang under it through
// the index, not through a parent.
func TestContentsOfAProjectIsTheWholeIndex(t *testing.T) {
	ctx, cwm, svc := listsWorld(t)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "demo", Object: cwmapi.Object{ID: "demo", Type: "project", Name: "demo"}}); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Contents(ctx, api.ContentsInput{ID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if got := folds(t, d); len(got["Goals"]) != 3 {
		t.Errorf("project contents goals = %v, want the goals of every module", got["Goals"])
	}
}

func TestSearchRunsSavedViewsAndCriteria(t *testing.T) {
	ctx, _, svc := listsWorld(t)
	views := []api.SavedView{
		{Name: "goals-in-idea", Label: "Goals in idea", Labels: map[string]string{"fr": "Objectifs en idée"}, SearchCriteria: api.SearchCriteria{Types: []string{"goal"}, States: []string{"idea"}}},
		{Name: "proposed", Label: "Proposed", SearchCriteria: api.SearchCriteria{States: []string{"proposed"}}},
	}
	// Nothing chosen: the views and the form, nothing searched.
	d, err := svc.Search(ctx, api.SearchInput{Views: views})
	if err != nil {
		t.Fatal(err)
	}
	if page := pageJSON(t, d); !strings.Contains(page, `"query":{"args":{"view":"goals-in-idea"}}`) || len(folds(t, d)) != 0 {
		t.Errorf("the empty search: %s", page)
	}
	// A view: its criteria, across every module; in French, its French label.
	d, _ = svc.Search(ctx, api.SearchInput{Views: views, View: "goals-in-idea", Language: "fr"})
	if got := folds(t, d); len(got["Goals"]) != 3 {
		t.Errorf("goals in idea = %v, want the three goals of both modules", got)
	}
	if !strings.Contains(pageJSON(t, d), "Objectifs en idée") {
		t.Error("the view's French label is not shown")
	}
	// The target narrows to a module; the criteria typed in the form.
	d, _ = svc.Search(ctx, api.SearchInput{Target: "n", Views: views, View: "goals-in-idea"})
	if got := folds(t, d); strings.Join(got["Goals"], ",") != "n.g9" {
		t.Errorf("target n: %v", got)
	}
	d, _ = svc.Search(ctx, api.SearchInput{SearchCriteria: api.SearchCriteria{Text: "export"}})
	if got := folds(t, d); len(got) != 2 {
		t.Errorf("text export: %v", got)
	}
	if _, err := svc.Search(ctx, api.SearchInput{View: "nope"}); err == nil {
		t.Error("an unknown view must be refused")
	}
}

// A panel standing for a child type (`replaces_children`) takes its children
// tab: the type is listed once, in that panel.
func TestAPanelReplacesAChildrenTab(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/goal.yaml":   "name: goal\nlabel_plural: Goals\ntree: none\n",
		"object-types/record.yaml": "name: record\nlabel_plural: Records\ntree: none\n",
		"object-panels/goals.yaml": "name: Goals board\nfilter: {types: [module]}\nreplaces_children: [goal]\nsource: 'ktw-methods:goals/board'\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.g", Type: "goal"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.r", Type: "record"})
	by := panelByID(detail(t, ctx, svc, "m", false).Panels)
	if _, ok := by["tab:children/goal"]; ok {
		t.Errorf("the goal children tab shows beside the panel standing for it: %v", by)
	}
	if _, ok := by["tab:goals"]; !ok {
		t.Errorf("the panel is missing: %v", by)
	}
	if _, ok := by["tab:children/record"]; !ok {
		t.Errorf("a type no panel stands for lost its tab: %v", by)
	}
}

// The SHIPPED panels on a planned module (one carrying `planned`, as a
// development module does): Contents (ktw-agents), Goals and Features
// (ktw-methods), Requests (planification) show, and the goal, feature and
// request children tabs they stand for do not — each type is listed once.
func TestShippedListPanelsOnAModule(t *testing.T) {
	shipped := func(rel string) string { return string(cwmapp.ShippedFile(t, rel)) }
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/module.yaml":      "name: module\n",
		"capabilities/planned.yaml":     "name: planned\n",
		"object-types/module.yaml":      "name: module\ncapabilities: [module, planned]\ntree: node\n",
		"object-types/goal.yaml":        "name: goal\nlabel_plural: Goals\ntree: none\n",
		"object-types/feature.yaml":     "name: feature\nlabel_plural: Features\ntree: none\n",
		"object-types/use-case.yaml":    "name: use-case\nlabel_plural: Use cases\ntree: none\n",
		"object-types/request.yaml":     "name: request\nlabel_plural: Requests\ntree: none\n",
		"object-panels/contents.yaml":   shipped("app/ktw-agents/embed/cwm/object-panels/contents.yaml"),
		"object-panels/objects.yaml":    shipped("app/ktw-agents/embed/cwm/object-panels/objects.yaml"),
		"object-panels/goals.yaml":      shipped("app/ktw-methods/embed/cwm/object-panels/goals.yaml"),
		"object-panels/features.yaml":   shipped("app/ktw-methods/embed/cwm/object-panels/features.yaml"),
		"object-panels/challenged.yaml": shipped("app/ktw-methods/embed/cwm/object-panels/challenged-by.yaml"),
		"object-panels/requests.yaml":   shipped("internal/extensions/planification/contrib/embed/cwm/object-panels/requests.yaml"),
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.g", Type: "goal"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.f", Type: "feature"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.u", Type: "use-case"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.RQ-001", Type: "request"})
	by := panelByID(detail(t, ctx, svc, "m", false).Panels)
	for _, id := range []string{"tab:contents", "tab:goals", "tab:features", "tab:requests", "tab:children/use-case"} {
		if _, ok := by[id]; !ok {
			t.Errorf("%s is missing: %v", id, ids(detail(t, ctx, svc, "m", false).Panels))
		}
	}
	for _, id := range []string{"tab:children/goal", "tab:children/feature", "tab:children/request", "tab:objects", "tab:challenged"} {
		if _, ok := by[id]; ok {
			t.Errorf("%s shows on the module", id)
		}
	}
	if by["tab:contents"].Source != "ktw-agents:object-pane/contents" || by["tab:goals"].Source != "ktw-methods:goals/board" {
		t.Errorf("sources: %+v", by)
	}
	global, err := svc.GlobalPanels(ctx, api.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	var objects *api.ObjectPanel
	for i := range global.Panels {
		if global.Panels[i].ID == "home:objects" {
			objects = &global.Panels[i]
		}
	}
	if objects == nil || objects.Source != "ktw-agents:object-pane/search" {
		t.Fatalf("the Objects search is not in the Global strip: %+v", global.Panels)
	}
	views, _ := objects.Input["views"].([]any)
	if len(views) != 4 {
		t.Errorf("the Objects panel carries %d saved views, want 4: %v", len(views), objects.Input)
	}
}

// The Objects panel finds an object fast: a code typed short -
// F-1, f-001 - names its object first, under Best match; a criterion set
// beyond the quick box shows as a chip whose query removes it; the Advanced
// form is not offered at the Essentiel display level.
func TestQuickSearch(t *testing.T) {
	ctx, cwm, svc := listsWorld(t)
	create(t, ctx, cwm, cwmapi.Object{ID: "m.F-001", Type: "feature", Name: "Search by code"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.F-010", Type: "feature", Name: "Another one"})
	for _, text := range []string{"F-1", "f-001", "m.F-001"} {
		d, err := svc.Search(ctx, api.SearchInput{SearchCriteria: api.SearchCriteria{Text: text}})
		if err != nil {
			t.Fatal(err)
		}
		if best := folds(t, d); !strings.Contains(pageJSON(t, d), "Best match") || !strings.Contains(bestOf(t, d), "m.F-001") || strings.Contains(bestOf(t, d), "m.F-010") {
			t.Errorf("%q: best match %q; want m.F-001 alone (results %v)", text, bestOf(t, d), best)
		}
	}
	// Words name no id: no best match.
	d, _ := svc.Search(ctx, api.SearchInput{SearchCriteria: api.SearchCriteria{Text: "export"}})
	if strings.Contains(pageJSON(t, d), "Best match") {
		t.Error("words that name no id gave a best match")
	}
	// A state set beyond the box: a chip removing it, the text and types kept.
	d, _ = svc.Search(ctx, api.SearchInput{SearchCriteria: api.SearchCriteria{Text: "ship", Types: []string{"goal"}, States: []string{"idea"}}})
	page := pageJSON(t, d)
	if !strings.Contains(page, `"text":"States : idea ✕"`) || !strings.Contains(page, `"query":{"args":{"capabilities":null,"incoming":null,"outgoing":null,"states":[],"text":"ship","types":["goal"]}}`) {
		t.Errorf("the state in force: no chip removing it and keeping the rest: %s", page)
	}
	// The quick box keeps the criteria set beyond it.
	if !strings.Contains(page, `"states":["idea"],"text":"${value.text}","types":"${value.types}"`) {
		t.Errorf("the quick box drops the state in force: %s", page)
	}
	// Advanced: offered at Standard, not at Essentiel.
	if !strings.Contains(page, `"id":"advanced"`) {
		t.Error("no Advanced form at Standard")
	}
	d, _ = svc.Search(ctx, api.SearchInput{Level: "essential", SearchCriteria: api.SearchCriteria{Text: "ship"}})
	if strings.Contains(pageJSON(t, d), `"id":"advanced"`) {
		t.Error("the Advanced form is offered at Essentiel")
	}
}

// bestOf is the ids the page's Best match section opens.
func bestOf(t *testing.T, d render.Def) string {
	t.Helper()
	page := pageJSON(t, d)
	i := strings.Index(page, "Best match")
	if i < 0 {
		return ""
	}
	rest := page[i:]
	if j := strings.Index(rest, `"title":"Results"`); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// The method's panels - Goals, Features, Composition, What's Next - show on a
// module only when it is planned: an étude de l'existant, a
// module not planned, shows none of them; a planned module and a software
// module (its type carries planned) keep them.
func TestMethodPanelsFollowPlanned(t *testing.T) {
	shipped := func(rel string) string { return string(cwmapp.ShippedFile(t, rel)) }
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/module.yaml":                "name: module\n",
		"capabilities/planned.yaml":               "name: planned\n",
		"object-types/module.yaml":                "name: module\ncapabilities: [module]\ntree: node\n",
		"capabilities/code.yaml":                  "name: code\n",
		"object-types/software-module.yaml":       "name: software-module\ncapabilities: [module, code, planned]\ntree: node\n",
		"object-types/goal.yaml":                  "name: goal\nlabel_plural: Goals\ntree: none\n",
		"object-types/feature.yaml":               "name: feature\nlabel_plural: Features\ntree: none\n",
		"object-panels/goals.yaml":                shipped("app/ktw-methods/embed/cwm/object-panels/goals.yaml"),
		"object-panels/features.yaml":             shipped("app/ktw-methods/embed/cwm/object-panels/features.yaml"),
		"object-panels/composition-overview.yaml": shipped("app/ktw-methods/embed/cwm/object-panels/composition-overview.yaml"),
		"object-panels/method-next.yaml":          shipped("app/ktw-methods/embed/cwm/object-panels/method-next.yaml"),
	})
	planned := cwmapi.NewContainerObject("p", "p", "module", "", "planned", "")
	planned.Capabilities = map[string]map[string]any{"planned": {}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "p", Object: planned}); err != nil {
		t.Fatal(err)
	}
	code := cwmapi.NewContainerObject("c", "c", "software-module", "", "code", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "c", Object: code}); err != nil {
		t.Fatal(err)
	}
	method := []string{"tab:goals", "tab:features", "tab:composition-overview", "tab:method-next"}
	unplanned := panelByID(detail(t, ctx, svc, "m", false).Panels)
	for _, id := range method {
		if _, ok := unplanned[id]; ok {
			t.Errorf("a module not planned shows %s", id)
		}
	}
	for _, mod := range []string{"p", "c"} {
		got := panelByID(detail(t, ctx, svc, mod, false).Panels)
		for _, id := range method {
			if _, ok := got[id]; !ok {
				t.Errorf("%s lacks %s: %v", mod, id, ids(detail(t, ctx, svc, mod, false).Panels))
			}
		}
	}
}

// The method's types follow the same rule in a module's « + » (
// ): goal and feature, as shipped, are offered under a planned
// module and a code module, not under a module that is not planned (a study).
func TestMethodTypesFollowPlanned(t *testing.T) {
	acceptWhen := func(rel string) string {
		for _, line := range strings.Split(strings.ReplaceAll(string(cwmapp.ShippedFile(t, rel)), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(line, "accept_when:") {
				return line + "\n"
			}
		}
		t.Fatalf("%s declares no accept_when", rel)
		return ""
	}
	ctx, cwm, _ := paneWorld(t, map[string]string{
		"capabilities/module.yaml":          "name: module\n",
		"capabilities/planned.yaml":         "name: planned\n",
		"object-types/module.yaml":          "name: module\ncapabilities: [module]\ntree: node\n",
		"capabilities/code.yaml":            "name: code\n",
		"object-types/software-module.yaml": "name: software-module\ncapabilities: [module, code, planned]\ntree: node\n",
		"object-types/goal.yaml":            "name: goal\ntree: none\n" + acceptWhen("app/ktw-methods/embed/cwm/object-types/goal.yaml"),
		"object-types/feature.yaml":         "name: feature\ntree: none\n" + acceptWhen("app/ktw-methods/embed/cwm/object-types/feature.yaml"),
	})
	planned := cwmapi.NewContainerObject("p", "p", "module", "", "planned", "")
	planned.Capabilities = map[string]map[string]any{"planned": {}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "p", Object: planned}); err != nil {
		t.Fatal(err)
	}
	code := cwmapi.NewContainerObject("c", "c", "software-module", "", "code", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "c", Object: code}); err != nil {
		t.Fatal(err)
	}
	offered := func(parent string) map[string]bool {
		out, err := cwm.ObjectAllowedChildren(ctx, cwmapi.ObjectAllowedChildrenInput{ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, c := range out.Choices {
			m[c.Name] = true
		}
		return m
	}
	if got := offered("m"); got["goal"] || got["feature"] {
		t.Errorf("a module not planned offers %v", got)
	}
	for _, mod := range []string{"p", "c"} {
		if got := offered(mod); !got["goal"] || !got["feature"] {
			t.Errorf("%s does not offer goal and feature: %v", mod, got)
		}
	}
}

// Planification's types follow it too:
// request, increment, release and stream, as shipped, are offered under a
// planned module, not under a module that is not planned (a study).
func TestPlanificationTypesFollowPlanned(t *testing.T) {
	names := []string{"request", "increment", "release", "stream"}
	files := map[string]string{
		"capabilities/module.yaml":  "name: module\n",
		"capabilities/planned.yaml": "name: planned\n",
		"object-types/module.yaml":  "name: module\ncapabilities: [module]\ntree: node\n",
	}
	for _, n := range names {
		rel := "internal/extensions/planification/contrib/embed/cwm/object-types/" + n + ".yaml"
		line := ""
		for _, l := range strings.Split(strings.ReplaceAll(string(cwmapp.ShippedFile(t, rel)), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(l, "accept_when:") {
				line = l + "\n"
			}
		}
		if line == "" {
			t.Fatalf("%s declares no accept_when", rel)
		}
		files["object-types/"+n+".yaml"] = "name: " + n + "\ntree: none\n" + line
	}
	ctx, cwm, _ := paneWorld(t, files)
	planned := cwmapi.NewContainerObject("p", "p", "module", "", "planned", "")
	planned.Capabilities = map[string]map[string]any{"planned": {}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "p", Object: planned}); err != nil {
		t.Fatal(err)
	}
	offered := func(parent string) map[string]bool {
		out, err := cwm.ObjectAllowedChildren(ctx, cwmapi.ObjectAllowedChildrenInput{ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, c := range out.Choices {
			m[c.Name] = true
		}
		return m
	}
	unplanned, plannedOffer := offered("m"), offered("p")
	for _, n := range names {
		if unplanned[n] {
			t.Errorf("a module not planned offers %s", n)
		}
		if !plannedOffer[n] {
			t.Errorf("a planned module does not offer %s: %v", n, plannedOffer)
		}
	}
}

// : a declared panel's tab carries a badge read from the
// object as a header chip's - its value in its tone (`*` any value) - and
// none when the value is empty or 0; a panel declaring none has none.
func TestAPanelsTabCarriesABadgeReadFromTheObject(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/record.yaml":  "name: record\nlabel_plural: Records\ntree: none\n",
		"object-panels/review.yaml": "name: Sections\nfilter: {types: [record]}\nsource: 'ktw-x:sections/board'\nbadge: {field: description, colors: {'*': red}}\n",
		"object-panels/plain.yaml":  "name: Plain\nfilter: {types: [record]}\nsource: 'ktw-x:plain/board'\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.two", Type: "record", Description: "2"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.zero", Type: "record", Description: "0"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.none", Type: "record"})
	by := panelByID(detail(t, ctx, svc, "m.two", false).Panels)
	if b := by["tab:review"].Badge; b == nil || b.Label != "2" || b.Color != "red" {
		t.Errorf("the tab's badge reads the object's value in its tone: %+v", b)
	}
	if by["tab:plain"].Badge != nil {
		t.Errorf("a panel declaring no badge has one: %+v", by["tab:plain"].Badge)
	}
	for _, id := range []string{"m.zero", "m.none"} {
		if b := panelByID(detail(t, ctx, svc, id, false).Panels)["tab:review"].Badge; b != nil {
			t.Errorf("%s: a badge for an empty or zero value: %+v", id, b)
		}
	}
}

// : a declared panel's tab carries its `count` action - the
// GUI calls it and draws the answer as the tab's badge; a panel declaring no
// count carries none. Detail itself never calls it.
func TestAPanelsTabCarriesItsCountAction(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/record.yaml":   "name: record\nlabel_plural: Records\ntree: none\n",
		"object-panels/counted.yaml": "name: Open\nfilter: {types: [record]}\nsource: 'ktw-x:open/board'\ncount: 'ktw-x:open/count'\n",
		"object-panels/plain.yaml":   "name: Plain\nfilter: {types: [record]}\nsource: 'ktw-x:plain/board'\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.one", Type: "record"})
	by := panelByID(detail(t, ctx, svc, "m.one", false).Panels)
	if got := by["tab:counted"].Count; got != "ktw-x:open/count" {
		t.Errorf("the tab carries its count action: %q", got)
	}
	if got := by["tab:plain"].Count; got != "" {
		t.Errorf("a panel declaring no count carries one: %q", got)
	}
}
