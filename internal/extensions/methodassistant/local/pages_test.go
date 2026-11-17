// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	macore "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/internal/core"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"gopkg.in/yaml.v3"
)

// THE METHOD PANELS AS PAGES: What's Next is drawn in Go from Overview's
// answer, the composition is a page declared over Composition's answer. Each
// is checked for its shape — the grammar, and every action the old views
// offered — on a module written for it.

// nodes walks a page tree (as JSON) and returns every node.
func nodes(t *testing.T, tree any) []map[string]any {
	t.Helper()
	b, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["kind"].(string); ok {
				out = append(out, x)
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(root)
	return out
}

// actionsOf lists every action of the tree, as JSON text.
func actionsOf(t *testing.T, tree any) []string {
	t.Helper()
	var out []string
	for _, n := range nodes(t, tree) {
		for _, k := range []string{"action", "open", "submit"} {
			if a, ok := n[k]; ok {
				b, _ := json.Marshal(a)
				out = append(out, string(b))
			}
		}
	}
	return out
}

func hasAction(acts []string, parts ...string) bool {
	for _, a := range acts {
		all := true
		for _, p := range parts {
			all = all && strings.Contains(a, p)
		}
		if all {
			return true
		}
	}
	return false
}

func TestNextPageDrawsWhatsNextWithItsActions(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", nil)
	f := put(t, ctx, cwm, "us1", "feature", "Run it", map[string]any{"business_value": 8},
		cwmapi.ObjectEdge{Kind: "serves", To: g})
	ac := criterion(t, ctx, cwm, f, "us1-AC1", "It runs", "fail")
	criterion(t, ctx, cwm, "m", "loose-AC1", "Loose", "")

	def, err := svc.NextPage(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if def.Kind != "page" {
		t.Fatalf("kind %q", def.Kind)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("off the page grammar: %v", err)
	}
	acts := actionsOf(t, def.DSLTree)
	for name, want := range map[string][]string{
		"a goal generates its features in a chat": {`"task":"` + macore.GenerateFeaturesTask + `"`, `"target":"` + g + `"`},
		"a feature is broken down in a chat":      {`"task":"` + macore.BreakDownTask + `"`, `"target":"` + f + `"`},
		"a feature opens":                         {`"navigate":{"object":"` + f + `"}`},
		"a criterion chip opens its criterion":    {`"navigate":{"object":"` + ac + `"}`},
		"an action runs on the act URI":           {`"uri":"` + api.ActURI + `"`, `"id":"m"`, `"kind":`},
	} {
		if !hasAction(acts, want...) {
			t.Errorf("%s: no action with %v in %v", name, want, acts)
		}
	}
	var sections []string
	for _, n := range nodes(t, def.DSLTree) {
		if n["kind"] == "section" {
			sections = append(sections, n["title"].(string))
		}
		if n["kind"] == "gauge" && n["label"] == "Acceptance criteria" && n["valueText"] != "0 / 2 pass" {
			t.Errorf("the standing: %v", n)
		}
	}
	if got := strings.Join(sections, "|"); got != "Goals|Features|Criteria under no feature|Alignment|What's next" {
		t.Errorf("sections: %s", got)
	}
}

// What's Next answers in the person's language: French when the panel asks
// for it, English by default — the data (titles) as written.
func TestNextPageAnswersInThePersonsLanguage(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", nil)
	f := put(t, ctx, cwm, "us1", "feature", "Run it", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})
	criterion(t, ctx, cwm, f, "us1-AC1", "It runs", "fail")
	titles := func(lang string) string {
		def, err := svc.NextPage(ctx, api.ObjectInput{ID: "m", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, n := range nodes(t, def.DSLTree) {
			if n["kind"] == "section" {
				out = append(out, n["title"].(string))
			}
		}
		b, _ := json.Marshal(def.DSLTree)
		if !strings.Contains(string(b), "Run it") {
			t.Errorf("%q: the feature's title is data, kept as written", lang)
		}
		return strings.Join(out, "|")
	}
	if got := titles("fr"); got != "Objectifs|Fonctionnalités|Et ensuite ?" {
		t.Errorf("in French: %s", got)
	}
	for _, lang := range []string{"", "en", "de"} {
		if got := titles(lang); got != "Goals|Features|What's next" {
			t.Errorf("%q: in English: %s", lang, got)
		}
	}
}

// Requests waiting on intake are listed (five, then counted) with their
// requester, each opening; "All requests" opens the module's Requests tab.
func TestNextTreeFoldsInTheRequestsToTriage(t *testing.T) {
	var rs []planapi.Request
	for _, id := range []string{"m.r1", "m.r2", "m.r3", "m.r4", "m.r5", "m.r6"} {
		r := planapi.Request{ID: id, Title: "Ask " + id}
		r.Requester = cwmapi.Actor{ID: "billing", Kind: "module"}
		rs = append(rs, r)
	}
	rs[0].Kind = "question"
	tree := macore.NextTree("", "m", api.OverviewOutput{Act: api.ActURI}, rs)
	if err := render.ValidatePage(tree); err != nil {
		t.Fatal(err)
	}
	acts := actionsOf(t, tree)
	if !hasAction(acts, `"object":"m"`, `"panel":"requests"`) || !hasAction(acts, `"object":"m.r1"`) || hasAction(acts, `"object":"m.r6"`) {
		t.Errorf("request actions: %v", acts)
	}
	b, _ := json.Marshal(tree)
	for _, want := range []string{`"title":"Requests to triage"`, `"and 1 more"`, `"from billing"`, `"text":"question"`, `"empty":"Nothing pending — all green."`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(string(b), "Requests to triage") != true {
		t.Fatal("no triage section")
	}
	if b2, _ := json.Marshal(macore.NextTree("", "m", api.OverviewOutput{}, nil)); strings.Contains(string(b2), "Requests to triage") {
		t.Error("no request to triage: no section")
	}
}

// The composition page is declared over Composition's answer: it validates,
// its section binds the answer's planning tree (its empty state) with an Add
// for each type it creates under the object — a type the method ships —, a
// feature sits under the goal it serves, and the page has its Advanced switch
// and its Full view link to What's Next.
// No section lists a use case: the want ladder is criterion, feature, goal.
// Every data-type the page names is one the pack ships: a retired one fails
// the whole tab ("schema: data-type not found").
func TestTheCompositionPageBindsTheAnswer(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "G", nil)
	put(t, ctx, cwm, "f1", "feature", "F", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})
	out, err := svc.Composition(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	answer := asJSON(t, out)

	var decl paneapi.PanelDecl
	if err := yaml.Unmarshal(cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-panels/composition-overview.yaml"), &decl); err != nil {
		t.Fatal(err)
	}
	if decl.Content == nil || decl.Content.Kind != "page" {
		t.Fatalf("content: %+v", decl.Content)
	}
	if err := decl.Content.Validate(); err != nil {
		t.Fatal(err)
	}
	acts := actionsOf(t, decl.Content.DSLTree)
	for _, typ := range []string{"goal", "feature"} {
		if !hasAction(acts, `"create":{"type":"`+typ+`"}`) {
			t.Errorf("no Add for %s: %v", typ, acts)
		}
	}
	if hasAction(acts, `"create":{"type":"use-case"}`) {
		t.Errorf("the page still adds a use case: %v", acts)
	}
	for _, a := range acts {
		var act struct {
			Create *struct {
				Type string `json:"type"`
			} `json:"create"`
		}
		if json.Unmarshal([]byte(a), &act) == nil && act.Create != nil {
			cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/"+act.Create.Type+".yaml")
		}
	}
	lens := map[string]string{"$.tree": ""}
	bound := map[string]bool{}
	for _, n := range nodes(t, decl.Content.DSLTree) {
		df, _ := n["dataField"].(string)
		if n["kind"] == "section" {
			if _, ok := lens[df]; !ok || n["empty"] == nil {
				t.Errorf("section %v binds %q", n["title"], df)
			}
			bound[df] = true
		}
		if s, ok := n["schema"].(string); ok {
			name := strings.TrimPrefix(s, "ktw-methods:/data-type/")
			if name == s {
				t.Errorf("%s names a schema outside the method: %s", n["kind"], s)
			} else {
				cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/"+name+".dsl.yaml")
			}
		}
		if n["kind"] == "tree" && df == "$.tree" {
			rows, _ := answer["tree"].([]any)
			served := false
			for _, r := range rows {
				m := r.(map[string]any)
				served = served || (m["type"] == "feature" && m["parent"] != nil && m["parent"] != "")
			}
			if !served {
				t.Errorf("no feature sits under the goal it serves: %v", rows)
			}
		}
	}
	for df := range lens {
		if !bound[df] {
			t.Errorf("no section binds %s", df)
		}
		if _, ok := answer[strings.TrimPrefix(df, "$.")]; !ok {
			t.Errorf("the answer has no %s", df)
		}
	}
	if b, _ := json.Marshal(decl.Content.DSLTree); !strings.Contains(string(b), `"advanced":true`) || !strings.Contains(string(b), `"panel":"method-next"`) {
		t.Error("the page has no Advanced switch or no Full view link to What's Next")
	}
}

// The shipped panels on a module, as the object pane serves them.
func shippedPanels(t *testing.T) map[string]paneapi.ObjectPanel {
	t.Helper()
	files := map[string]string{}
	for _, id := range []string{"method-next", "composition-overview"} {
		files["object-panels/"+id+".yaml"] = string(cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-panels/"+id+".yaml"))
	}
	// The panels show on a planned module, as a development
	// module is.
	// Their filter asks the module level capability.
	files["capabilities/planned.yaml"] = "name: planned\n"
	files["capabilities/module.yaml"] = "name: module\n"
	ctx, cwm := cwmapp.New(t, methodPack(files), panelocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	for _, c := range []string{"module", "planned"} {
		if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: c}); err != nil {
			t.Fatal(err)
		}
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, paneapi.DetailInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]paneapi.ObjectPanel{}
	for _, p := range d.Panels {
		out[strings.TrimPrefix(p.ID, "tab:")] = p
	}
	return out
}
