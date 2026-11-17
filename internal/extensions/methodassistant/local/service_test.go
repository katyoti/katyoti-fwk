// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	malocal "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/local"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE METHOD PANELS' SOURCES, on a real CWM reached as ktw-methods reaches it
// (the generated bus client): What's Next and the composition of a module
// read back what was written, and Act's write is seen by the next Overview.

func methodPack(extra map[string]string) application.Extension {
	pack := fstest.MapFS{
		"object-types/module.yaml":               {Data: []byte("name: module\ntree: node\n")},
		"object-types/project.yaml":              {Data: []byte("name: project\ntree: node\ncapabilities: [project]\n")},
		"object-types/goal.yaml":                 {Data: []byte("name: goal\nfolder: goals\n")},
		"object-types/feature.yaml":              {Data: []byte("name: feature\nfolder: features\n")},
		"object-types/deliverable.yaml":          {Data: []byte("name: deliverable\nfolder: deliverables\n")},
		"object-types/acceptance-criterion.yaml": {Data: []byte("name: acceptance-criterion\nfolder: criteria\n")},
		"object-types/verdict.yaml":              {Data: []byte("name: verdict\nfolder: verdicts\n")},
	}
	for p, b := range extra {
		pack[p] = &fstest.MapFile{Data: []byte(b)}
	}
	return cwmapp.Pack(pack)
}

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.MethodAssistantService) {
	t.Helper()
	ctx, cwm := cwmapp.RemoteHosting(t, []application.Extension{methodPack(nil)}, malocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.MethodAssistantService](ctx)
	if err != nil {
		t.Fatalf("resolve MethodAssistantService: %v", err)
	}
	return ctx, cwm, svc
}

func put(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, local, typ, title string, fields map[string]any, edges ...cwmapi.ObjectEdge) string {
	t.Helper()
	return putUnder(t, ctx, cwm, "m", local, typ, title, fields, edges...)
}

func putUnder(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, parent, local, typ, title string, fields map[string]any, edges ...cwmapi.ObjectEdge) string {
	t.Helper()
	o := cwmapi.NewContainerObject("m", local, typ, parent, title, "")
	o.Fields = fields
	o.Edges = edges
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create %s: %v", local, err)
	}
	return o.ID
}

// criterion puts an acceptance criterion under parent and, when status is
// not empty, the verdict a run recorded for it, under it.
func criterion(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, parent, local, title, status string) string {
	t.Helper()
	id := putUnder(t, ctx, cwm, parent, local, "acceptance-criterion", title, nil)
	if status != "" {
		v := cwmapi.NewContainerObject("m", "verdict-"+local, "verdict", id, "Verdict: "+title, "")
		v.Capabilities["verdict"] = map[string]any{"status": status, "kind": "command"}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: v}); err != nil {
			t.Fatalf("create the verdict of %s: %v", local, err)
		}
	}
	return id
}

// The answer as the view reads it: its JSON.
func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// What's Next on a module: the goal, the feature with the acceptance criteria
// under it (a criterion's status read from its verdict, the CoD from the
// fields), the criterion under no feature, the findings, the ranked actions
// and the act URI.
func TestOverviewReadsTheModulesMethodObjects(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", map[string]any{"scope": "billing"})
	f := put(t, ctx, cwm, "us1", "feature", "Run it", map[string]any{"business_value": 8, "time_criticality": 2},
		cwmapi.ObjectEdge{Kind: "serves", To: g})
	criterion(t, ctx, cwm, f, "us1-AC1", "It runs", "pass")
	criterion(t, ctx, cwm, "m", "loose-AC1", "Loose", "")

	out, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) != 1 || out.Goals[0].ID != g || out.Goals[0].ServingUS != 1 || out.Goals[0].DeliveredUS != 1 {
		t.Errorf("goals: %+v", out.Goals)
	}
	if len(out.Stories) != 1 || out.Stories[0].CoD != 10 || !out.Stories[0].Delivered ||
		len(out.Stories[0].Criteria) != 1 || out.Stories[0].Criteria[0].Status != "pass" || out.Stories[0].Criteria[0].NodeID != f {
		t.Errorf("stories: %+v", out.Stories)
	}
	if len(out.LooseCriteria) != 1 || out.LooseCriteria[0].Status != "no-test" {
		t.Errorf("loose criteria: %+v", out.LooseCriteria)
	}
	if out.Overview.CriteriaTotal != 2 || out.Overview.CriteriaPassing != 1 || out.Overview.CriteriaNoTest != 1 {
		t.Errorf("overview: %+v", out.Overview)
	}
	if len(out.Findings) == 0 || len(out.Actions) == 0 {
		t.Errorf("a criterion under no feature is a finding and an action: %+v / %+v", out.Findings, out.Actions)
	}
	if out.Act != api.ActURI {
		t.Errorf("act = %q", out.Act)
	}
	// The keys the view reads.
	j := asJSON(t, out)
	for _, k := range []string{"overview", "findings", "actions", "goals", "stories", "looseCriteria", "act"} {
		if _, ok := j[k]; !ok {
			t.Errorf("the answer has no %q: %v", k, j)
		}
	}
	if ov := j["overview"].(map[string]any); ov["CriteriaPassing"] == nil {
		t.Errorf("overview keys: %v", ov)
	}
	if _, err := svc.Overview(ctx, api.ObjectInput{ID: "m.nothing"}); err == nil {
		t.Error("an id naming no object must be refused")
	}
}

// A project's modules are parentless: its scope is the whole index.
func TestOverviewOnAProjectIsTheWholeIndex(t *testing.T) {
	ctx, cwm, svc := setup(t)
	proj := cwmapi.NewContainerObject("p", "p", "project", "", "P", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "p", Object: proj}); err != nil {
		t.Fatal(err)
	}
	put(t, ctx, cwm, "g1", "goal", "G", nil)
	out, err := svc.Overview(ctx, api.ObjectInput{ID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) != 1 {
		t.Errorf("a project's What's Next sees its modules' goals: %+v", out.Goals)
	}
}

// A criterion under no feature is named, not moved: which feature it proves
// is a judgement, so Act writes nothing and the next Overview still finds it.
func TestActNamesAnOrphanCriterionWithoutMovingIt(t *testing.T) {
	ctx, cwm, svc := setup(t)
	put(t, ctx, cwm, "us1", "feature", "Run it", nil)
	criterion(t, ctx, cwm, "m", "loose-AC1", "Loose", "")

	res, err := svc.Act(ctx, api.ActInput{ID: "m", Kind: "retro-link"})
	if err != nil || res.OK || res.Action != "retro-link" || !strings.Contains(res.Message, "Loose") {
		t.Fatalf("act: %+v %v", res, err)
	}
	out, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.LooseCriteria) != 1 || len(out.Stories) != 1 || len(out.Stories[0].Criteria) != 0 {
		t.Errorf("after act: loose %+v stories %+v", out.LooseCriteria, out.Stories)
	}
	if res, _ := svc.Act(ctx, api.ActInput{ID: "m", Kind: "no-such-kind"}); res.OK || res.Message != "no matching action" {
		t.Errorf("an unknown kind: %+v", res)
	}
}

// A feature's criteria are those under it at any depth: a sub-feature's
// criteria count for it too, and it is delivered only when they all pass.
// A failing criterion is a build action; one never run, a write-probe.
func TestAFeatureIsProvenByTheCriteriaUnderIt(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", nil)
	top := put(t, ctx, cwm, "f1", "feature", "Top", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})
	sub := putUnder(t, ctx, cwm, top, "f2", "feature", "Sub", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})
	criterion(t, ctx, cwm, top, "f1-AC1", "Top passes", "pass")
	red := criterion(t, ctx, cwm, sub, "f2-AC1", "Sub fails", "fail")
	never := criterion(t, ctx, cwm, sub, "f2-AC2", "Sub never ran", "")

	out, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]api.StoryView{}
	for _, s := range out.Stories {
		byID[s.ID] = s
	}
	if s := byID[top]; s.TotalCount != 3 || s.PassCount != 1 || s.Delivered {
		t.Errorf("the top feature counts its sub-feature's criteria: %+v", s)
	}
	if s := byID[sub]; s.TotalCount != 2 || s.PassCount != 0 || s.Delivered {
		t.Errorf("the sub-feature: %+v", s)
	}
	if len(out.LooseCriteria) != 0 || out.Overview.CriteriaFailing != 1 || out.Overview.USDelivered != 0 {
		t.Errorf("overview: %+v loose %+v", out.Overview, out.LooseCriteria)
	}
	kinds := map[string]string{}
	for _, a := range out.Actions {
		kinds[a.Subject] = a.Kind
	}
	if kinds[red] != "build" || kinds[never] != "write-probe" {
		t.Errorf("actions: %+v", out.Actions)
	}
}

// shape-us creates a feature beside the goal that serves it — the type the
// Generate Features task creates — and the next Overview counts it.
func TestActShapesAFeatureForAnUnservedGoal(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", nil)
	res, err := svc.Act(ctx, api.ActInput{ID: "m", Kind: "shape-us"})
	if err != nil || !res.OK {
		t.Fatalf("act: %+v %v", res, err)
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "feature"})
	if err != nil || len(list.Objects) != 1 {
		t.Fatalf("the feature: %+v %v", list, err)
	}
	us := list.Objects[0]
	if us.ParentID() != "m" || !strings.HasPrefix(us.ID, "m.f-") {
		t.Errorf("the feature: %+v", us)
	}
	out, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) != 1 || out.Goals[0].ServingUS != 1 || len(out.Stories) != 1 {
		t.Errorf("the shaped feature is read back: goals %+v stories %+v", out.Goals, out.Stories)
	}
	edges, _ := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: us.ID, Kind: "serves", Direction: cwmapi.DirectionOut})
	if len(edges.Edges) != 1 || edges.Edges[0].To != g {
		t.Errorf("the serves edge: %+v", edges)
	}
}

// A `feature` serving a goal (as Generate Features creates it) is the goal's
// want node: counted as serving it, listed with the criterion under it, and
// the goal is no GAP nor the criterion an ORPHAN. Its cost of delay is the
// `cod` of its `feature` capability. An object of another type serving the
// goal is no feature.
func TestOverviewCountsFeaturesServingAGoal(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "One run", nil)
	f := cwmapi.NewContainerObject("m", "f1", "feature", "m", "Run it", "")
	f.Capabilities["feature"] = map[string]any{"cod": 26}
	f.Edges = []cwmapi.ObjectEdge{{Kind: "serves", To: g}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: f}); err != nil {
		t.Fatal(err)
	}
	criterion(t, ctx, cwm, f.ID, "f1-AC1", "It runs", "pass")
	put(t, ctx, cwm, "x1", "deliverable", "Not a feature", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})

	out, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) != 1 || out.Goals[0].ServingUS != 1 || out.Goals[0].DeliveredUS != 1 {
		t.Errorf("the goal's serving features: %+v", out.Goals)
	}
	if len(out.Stories) != 1 || out.Stories[0].CoD != 26 || out.Overview.USTotal != 1 || out.Overview.USDelivered != 1 {
		t.Errorf("features: %+v overview %+v", out.Stories, out.Overview)
	}
	if len(out.LooseCriteria) != 0 {
		t.Errorf("the feature's criterion is no orphan: %+v", out.LooseCriteria)
	}
	for _, f := range out.Findings {
		if f.Kind == "GAP" || f.Kind == "ORPHAN" {
			t.Errorf("a served goal and a feature's criterion are aligned: %+v", f)
		}
	}
}

// Composition: the module's goals and features, each with its fields, a
// feature with the goals it serves titled (one of another module's included)
// — and nothing of another type.
func TestCompositionListsTheLensesWithTheirFields(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := put(t, ctx, cwm, "g1", "goal", "G", nil)
	cwmapp.Module(t, ctx, cwm, "o")
	og := cwmapi.NewContainerObject("o", "g9", "goal", "o", "Other G", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "o", Object: og}); err != nil {
		t.Fatal(err)
	}
	put(t, ctx, cwm, "f1", "feature", "F", map[string]any{"cod_note": "kept"},
		cwmapi.ObjectEdge{Kind: "serves", To: g}, cwmapi.ObjectEdge{Kind: "serves", To: og.ID})
	put(t, ctx, cwm, "x1", "deliverable", "Not a lens", nil)

	out, err := svc.Composition(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Goals) != 1 || len(out.Features) != 1 {
		t.Fatalf("composition: %+v", out)
	}
	if out.Features[0].Fields["cod_note"] != "kept" || out.Goals[0].Title != "G" {
		t.Errorf("fields not read whole: %+v", out)
	}
	f := out.Features[0]
	if f.Title != "F" || len(f.Serves) != 2 || f.Serves[0].Title != "G" || f.Serves[1].Title != "Other G" || f.ServesText != "Serves: G · Other G" {
		t.Errorf("the feature's served goals: %+v", f)
	}
	fr, err := svc.Composition(ctx, api.ObjectInput{ID: "m", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	if fr.Features[0].ServesText != "Sert : G · Other G" {
		t.Errorf("in French: %q", fr.Features[0].ServesText)
	}
	j := asJSON(t, out)
	for _, k := range []string{"goals", "features"} {
		if _, ok := j[k].([]any); !ok {
			t.Errorf("%q must be a list, even empty: %v", k, j)
		}
	}
}

// The shipped panels are sources: object-pane/Detail returns each with a verb
// this service publishes — What's Next answers its whole page (no content),
// the composition fills its declared page — and each with its refresh scope.
func TestTheShippedMethodPanelsAreSourcedFromThisService(t *testing.T) {
	panels := shippedPanels(t)
	published := map[string]bool{}
	for name := range api.MethodAssistantServiceDef.Methods {
		published["ktw-methods:"+api.MethodAssistantServiceDef.Name+"/"+reflect.ToKebab(name)] = true
	}
	for id, want := range map[string]struct {
		source, contentKind, scope string
	}{
		"method-next":          {"ktw-methods:method-assistant/next-page", "", paneapi.RefreshModule},
		"composition-overview": {"ktw-methods:method-assistant/composition", "page", paneapi.RefreshChildren},
	} {
		p, ok := panels[id]
		if !ok {
			t.Errorf("panel %s not on a module: %v", id, panels)
			continue
		}
		kind := ""
		if p.Content != nil {
			kind = p.Content.Kind
		}
		if p.Kind != paneapi.PanelKindSource || p.Source != want.source || kind != want.contentKind {
			t.Errorf("panel %s: kind %q source %q content %q", id, p.Kind, p.Source, kind)
		}
		if p.Refresh == nil || p.Refresh.Scope != want.scope {
			t.Errorf("panel %s: refresh %+v", id, p.Refresh)
		}
		if !published[p.Source] {
			t.Errorf("panel %s's source %q is no verb of the service (%v)", id, p.Source, published)
		}
	}
	if !published[api.ActURI] {
		t.Errorf("the act URI %q is no verb of the service", api.ActURI)
	}
}
