// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	featureslocal "github.com/katyoti/katyoti-fwk/internal/extensions/features/local"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// featurePack is what a managed feature needs from the catalog: its type, in
// the `feature` workflow AS SHIPPED by ktw-methods, the criterion type, and
// the decision a split writes its trail as (declared types are enforced).
func featurePack(t testing.TB) application.Extension {
	return cwmapp.Pack(fstest.MapFS{
		"object-types/feature.yaml":              {Data: []byte("name: feature\nfolder: features\ncapabilities: [container, content, want, feature, lifecycle]\nworkflow: feature\n")},
		"capabilities/feature.yaml":              {Data: []byte("name: feature\n")},
		"workflows/feature.yaml":                 {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/feature.yaml")},
		"object-types/acceptance-criterion.yaml": {Data: []byte("name: acceptance-criterion\nfolder: use-cases\ncapabilities: [want, checkable]\n")},
		"object-types/decision.yaml":             {Data: []byte("name: decision\nfolder: decisions\ncapabilities: [container, content]\n")},
	})
}

// manageHost is what the process holding CWM runs: the feature pack, and the
// journal — the lifecycle history, where a retire's or a split's note is.
func manageHost(tb testing.TB) []application.Extension {
	return []application.Extension{
		featurePack(tb),
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), journallocal.Extension(), cwmapp.SystemModules(tb),
	}
}

// who retires or splits, as recorded in the lifecycle history.
var who = cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}

func setupManage(t *testing.T) (context.Context, cwmapi.CWMService, api.FeaturesService) {
	t.Helper()
	return setupManageIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		ctx, cwm := cwmapp.New(tb, append(exts, manageHost(tb)...)...)
		// The journal records on its own goroutine: drain it before the
		// workspace is removed.
		tb.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
		return ctx, cwm
	})
}

// setupManageRemote is the shape of the binary hosting this service: CWM is
// in another process, reached through the generated bus client.
func setupManageRemote(t *testing.T) (context.Context, cwmapi.CWMService, api.FeaturesService) {
	t.Helper()
	return setupManageIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		return cwmapp.RemoteHosting(tb, manageHost(tb), exts...)
	})
}

func setupManageIn(t *testing.T, boot func(testing.TB, ...application.Extension) (context.Context, cwmapi.CWMService)) (context.Context, cwmapi.CWMService, api.FeaturesService) {
	t.Helper()
	ctx, cwm := boot(t, featureslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.FeaturesService](ctx)
	if err != nil {
		t.Fatalf("resolve FeaturesService: %v", err)
	}
	return ctx, cwm, svc
}

func mkGoal(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, local string) string {
	t.Helper()
	g := cwmapi.NewContainerObject("m", local, "goal", "m", "Goal "+local, "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: g}); err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func add(t *testing.T, ctx context.Context, svc api.FeaturesService, in api.AddInput) api.Feature {
	t.Helper()
	if in.Module == "" {
		in.Module = "m"
	}
	out, err := svc.Add(ctx, in)
	if err != nil {
		t.Fatalf("Add %q: %v", in.Title, err)
	}
	return out.Feature
}

func readObj(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) cwmapi.Object {
	t.Helper()
	m, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	return got.Object
}

// state is the feature's lifecycle state; none is the workflow's initial.
func state(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return api.StateInitial
}

// lastNote is the note of the feature's last transition, as the journal
// recorded it, once it starts with want.
func lastNote(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		note := ""
		read, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: journalapi.DayID(time.Now()), Name: journalapi.EntriesFile})
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(read.Content))
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				var r journalapi.Record
				if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID == id && r.Action == journalapi.ActionTransitioned {
					note = r.Note
				}
			}
		}
		if strings.HasPrefix(note, want) || time.Now().After(deadline) {
			return note
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func uncoveredIDs(t *testing.T, ctx context.Context, svc api.FeaturesService) []string {
	t.Helper()
	unc, err := svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	if err != nil {
		t.Fatalf("Uncovered: %v", err)
	}
	ids := []string{}
	for _, f := range unc.Features {
		ids = append(ids, f.ID)
	}
	return ids
}

// MANAGING FEATURES: every verb the use-cases service had, on features, driven
// against a real CWM and read back through the Object API — locally and with
// CWM in another process, as in the binary hosting it.
func TestFeaturesManage(t *testing.T) {
	ctx, cwm, svc := setupManage(t)
	manageScenario(t, ctx, cwm, svc)
}

func TestFeaturesManageOverTheBus(t *testing.T) {
	ctx, cwm, svc := setupManageRemote(t)
	manageScenario(t, ctx, cwm, svc)
}

func manageScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, svc api.FeaturesService) {
	t.Helper()
	g1, g2 := mkGoal(t, ctx, cwm, "G-001"), mkGoal(t, ctx, cwm, "G-002")

	// Add — a closable feature in the workflow's initial state, serving g1.
	f := add(t, ctx, svc, api.AddInput{Title: "Export a report", Serves: []string{g1}})
	o := readObj(t, ctx, cwm, f.ID)
	if o.Type != "feature" || o.ParentID() != "m" || state(o) != api.StateInitial || o.Capabilities["want"]["reachability"] != "closable" ||
		len(o.Edges) != 1 || o.Edges[0].Kind != "serves" || o.Edges[0].To != g1 {
		t.Fatalf("stored feature: %+v", o)
	}

	// Goals — what it serves, with the goal's type and title.
	gs, err := svc.Goals(ctx, api.GoalsInput{ID: f.ID})
	if err != nil || len(gs.Goals) != 1 || gs.Goals[0].ID != g1 || gs.Goals[0].Type != "goal" || gs.Goals[0].Title != "Goal G-001" {
		t.Fatalf("Goals: %v %+v", err, gs.Goals)
	}

	// Uncovered — no criterion yet.
	if ids := uncoveredIDs(t, ctx, svc); len(ids) != 1 || ids[0] != f.ID {
		t.Fatalf("Uncovered before a criterion: %v", ids)
	}

	// AttachCriterion — a criterion under it with a command probe; one with no
	// probe is declared unmeasurable.
	ac, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{Feature: f.ID, Title: "The report opens", Probe: "true"})
	if err != nil {
		t.Fatal(err)
	}
	co := readObj(t, ctx, cwm, ac.Criterion.ID)
	if co.Type != "acceptance-criterion" || co.ParentID() != f.ID || co.Capabilities["checkable"]["kind"] != "command" ||
		co.Capabilities["checkable"]["probe"] != "true" || co.Capabilities["checkable"]["severity"] != "core" || ac.Criterion.Feature != f.ID {
		t.Fatalf("stored criterion: %+v", co)
	}
	ac2, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{Feature: f.ID, Title: "Not measurable yet"})
	if err != nil {
		t.Fatal(err)
	}
	if k := readObj(t, ctx, cwm, ac2.Criterion.ID).Capabilities["checkable"]["kind"]; k != "none" {
		t.Fatalf("a criterion with no probe: checkable.kind = %v, want none", k)
	}
	if ids := uncoveredIDs(t, ctx, svc); len(ids) != 0 {
		t.Fatalf("Uncovered after a criterion: %v", ids)
	}

	// Split — two successors, each serving g1, each adopting one criterion; the
	// source stays, dropped, the note saying it was split.
	sp, err := svc.Split(ctx, api.SplitInput{ID: f.ID, Actor: who, Reason: "Two wants in one", Successors: []api.SplitSuccessor{
		{Title: "Export as PDF", Children: []string{ac.Criterion.ID}},
		{Title: "Export as CSV", Children: []string{ac2.Criterion.ID}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Successors) != 2 {
		t.Fatalf("Split: %+v", sp)
	}
	for _, s := range sp.Successors {
		so := readObj(t, ctx, cwm, s.ID)
		if so.Type != "feature" || state(so) != api.StateInitial || so.Name == "" || so.Capabilities["want"]["reachability"] != "closable" ||
			len(so.Edges) != 1 || so.Edges[0].To != g1 {
			t.Fatalf("successor %s: %+v", s.ID, so)
		}
	}
	if got := readObj(t, ctx, cwm, ac.Criterion.ID).ParentID(); got != sp.Successors[0].ID {
		t.Fatalf("the adopted criterion hangs under %s, want %s", got, sp.Successors[0].ID)
	}
	if src := readObj(t, ctx, cwm, f.ID); state(src) != api.StateDropped || !strings.HasPrefix(lastNote(t, ctx, cwm, f.ID, "split"), "split") {
		t.Fatal("the split source is not dropped as split")
	}

	// Retire — a second feature, retired with a reason; it leaves Uncovered.
	other := add(t, ctx, svc, api.AddInput{Title: "Print it", Serves: []string{g2}})
	if _, err := svc.Retire(ctx, api.RetireInput{ID: other.ID, Actor: who, Reason: "Nobody prints."}); err != nil {
		t.Fatal(err)
	}
	ro := readObj(t, ctx, cwm, other.ID)
	if state(ro) != api.StateDropped || !strings.HasPrefix(lastNote(t, ctx, cwm, other.ID, "retired"), "retired") || !strings.Contains(ro.Body, "## Retired\n\nNobody prints.") {
		t.Fatalf("retired feature: state %q body %q", state(ro), ro.Body)
	}
	for _, id := range uncoveredIDs(t, ctx, svc) {
		if id == other.ID || id == f.ID {
			t.Fatalf("a retired or split feature is listed uncovered: %s", id)
		}
	}
}

// A feature is recursive: a criterion under a sub-feature checks its parent
// too, at any depth; the sub-feature with no criterion is the one reported; a
// dropped sub-feature's criterion checks nothing.
func TestFeaturesUncoveredReadsSubFeatures(t *testing.T) {
	ctx, cwm, svc := setupManage(t)
	parent := add(t, ctx, svc, api.AddInput{Title: "Reports"})
	mid := add(t, ctx, svc, api.AddInput{Title: "Exports", Parent: parent.ID})
	leaf := add(t, ctx, svc, api.AddInput{Title: "Export as PDF", Parent: mid.ID})
	bare := add(t, ctx, svc, api.AddInput{Title: "Export as CSV", Parent: mid.ID})
	if leaf.Parent != mid.ID || readObj(t, ctx, cwm, leaf.ID).ParentID() != mid.ID {
		t.Fatalf("a sub-feature sits under %s, want %s", leaf.Parent, mid.ID)
	}
	if ids := uncoveredIDs(t, ctx, svc); len(ids) != 4 {
		t.Fatalf("nothing checked yet, Uncovered = %v", ids)
	}
	if _, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{Feature: leaf.ID, Title: "The PDF opens", Probe: "true"}); err != nil {
		t.Fatal(err)
	}
	if ids := uncoveredIDs(t, ctx, svc); len(ids) != 1 || ids[0] != bare.ID {
		t.Fatalf("a criterion two levels down checks its ancestors; Uncovered = %v, want only %s", ids, bare.ID)
	}
	if _, err := svc.Retire(ctx, api.RetireInput{ID: leaf.ID, Actor: who}); err != nil {
		t.Fatal(err)
	}
	if ids := uncoveredIDs(t, ctx, svc); len(ids) != 3 {
		t.Fatalf("a dropped sub-feature's criterion still checks its parents; Uncovered = %v", ids)
	}
}

func TestFeaturesAddRefusesServingANonGoal(t *testing.T) {
	ctx, _, svc := setupManage(t)
	f := add(t, ctx, svc, api.AddInput{Title: "A"})
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "B", Serves: []string{f.ID}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("serving a feature: err = %v", err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "C", Serves: []string{"m.G-404"}}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("serving a missing goal: err = %v", err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "  "}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("no title: err = %v", err)
	}
}

func TestFeaturesSplitNeedsTwoAReasonAndAnActor(t *testing.T) {
	ctx, cwm, svc := setupManage(t)
	f := add(t, ctx, svc, api.AddInput{Title: "A"})
	two := []api.SplitSuccessor{{Title: "A1"}, {Title: "A2"}}
	if _, err := svc.Split(ctx, api.SplitInput{ID: f.ID, Actor: who, Reason: "r", Successors: two[:1]}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split into one: err = %v", err)
	}
	if _, err := svc.Split(ctx, api.SplitInput{ID: f.ID, Actor: who, Successors: two}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split with no reason: err = %v", err)
	}
	if _, err := svc.Split(ctx, api.SplitInput{ID: f.ID, Reason: "r", Successors: two}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split by nobody: err = %v", err)
	}
	if state(readObj(t, ctx, cwm, f.ID)) != api.StateInitial {
		t.Fatal("a refused split moved the source")
	}
}

func TestFeaturesRetiredIsFinal(t *testing.T) {
	ctx, _, svc := setupManage(t)
	f := add(t, ctx, svc, api.AddInput{Title: "A"})
	if _, err := svc.Retire(ctx, api.RetireInput{ID: f.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("retiring by nobody: err = %v", err)
	}
	if _, err := svc.Retire(ctx, api.RetireInput{ID: f.ID, Actor: who}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Retire(ctx, api.RetireInput{ID: f.ID, Actor: who}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("retiring twice: err = %v", err)
	}
	if _, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{Feature: f.ID, Title: "x"}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("a criterion on a retired feature: err = %v", err)
	}
	if _, err := svc.Split(ctx, api.SplitInput{ID: f.ID, Actor: who, Reason: "r", Successors: []api.SplitSuccessor{{Title: "1"}, {Title: "2"}}}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("splitting a retired feature: err = %v", err)
	}
}

func TestFeaturesCriterionOnlyUnderAFeature(t *testing.T) {
	ctx, cwm, svc := setupManage(t)
	g := mkGoal(t, ctx, cwm, "G-001")
	if _, err := svc.AttachCriterion(ctx, api.AttachCriterionInput{Feature: g, Title: "x"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a criterion under a goal: err = %v", err)
	}
	if _, err := svc.Goals(ctx, api.GoalsInput{ID: g}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("the goals of a goal: err = %v", err)
	}
}
