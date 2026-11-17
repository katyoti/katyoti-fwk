// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	goalslocal "github.com/katyoti/katyoti-fwk/internal/extensions/goals/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.GoalsService) {
	t.Helper()
	return setupIn(t, cwmapp.New)
}

// setupRemote is the shape of the binary hosting this service: CWM is in
// another process, reached through the generated bus client.
func setupRemote(t *testing.T) (context.Context, cwmapi.CWMService, api.GoalsService) {
	t.Helper()
	return setupIn(t, cwmapp.Remote)
}

func setupIn(t *testing.T, boot func(testing.TB, ...application.Extension) (context.Context, cwmapi.CWMService)) (context.Context, cwmapi.CWMService, api.GoalsService) {
	t.Helper()
	ctx, cwm := boot(t, goalslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.GoalsService](ctx)
	if err != nil {
		t.Fatalf("resolve GoalsService: %v", err)
	}
	return ctx, cwm, svc
}

func add(t *testing.T, ctx context.Context, svc api.GoalsService, title, parent string) api.Goal {
	t.Helper()
	out, err := svc.Add(ctx, api.AddInput{Module: "m", Title: title, Parent: parent})
	if err != nil {
		t.Fatalf("Add %q: %v", title, err)
	}
	return out.Goal
}

func ids(goals []api.Goal) []string {
	out := make([]string, len(goals))
	for i, g := range goals {
		out[i] = g.ID
	}
	return out
}

// TestGoalsService drives every verb against a real CWM service: each one's
// effect is read back through the Object API, never from the service's reply.
func TestGoalsService(t *testing.T) {
	ctx, cwm, svc := setup(t)
	goalsScenario(t, ctx, cwm, svc)
}

// TestGoalsServiceOverTheBus runs the same scenario with CWM in another
// process, as in the binary that hosts this service.
func TestGoalsServiceOverTheBus(t *testing.T) {
	ctx, cwm, svc := setupRemote(t)
	goalsScenario(t, ctx, cwm, svc)
}

func goalsScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, svc api.GoalsService) {
	t.Helper()

	// Add — a real file, under the module, a goal with a want.
	out, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "Ship the thing"})
	if err != nil {
		t.Fatal(err)
	}
	a := out.Goal
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: a.ID})
	if err != nil {
		t.Fatalf("the goal is not readable through the Object API: %v", err)
	}
	if got.Object.Type != "goal" || got.Object.ParentID() != "m" || got.Object.Capabilities["want"]["reachability"] != "asymptotic" {
		t.Fatalf("stored goal: %+v", got.Object)
	}
	if out.Path == "" {
		t.Fatal("Add reported no path")
	}
	b := add(t, ctx, svc, "Keep it fast", "")
	c := add(t, ctx, svc, "Cold start under a second", b.ID)

	// List — every goal of the module.
	list, err := svc.List(ctx, api.ListInput{Module: "m"})
	if err != nil || len(list.Goals) != 3 {
		t.Fatalf("List: %v %+v", err, list.Goals)
	}

	// Reorder — b before a under the module, read back in that order.
	if _, err := svc.Reorder(ctx, api.ReorderInput{IDs: []string{b.ID, a.ID}}); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.List(ctx, api.ListInput{Module: "m"})
	var top []string
	for _, g := range list.Goals {
		if g.Parent == "m" {
			top = append(top, g.ID)
		}
	}
	if len(top) != 2 || top[0] != b.ID || top[1] != a.ID {
		t.Fatalf("order after Reorder: %v", top)
	}

	// Reparent — c moves from under b to under a.
	if _, err := svc.Reparent(ctx, api.ReparentInput{ID: c.ID, NewParent: a.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: c.ID})
	if got.Object.ParentID() != a.ID {
		t.Fatalf("parent after Reparent = %q", got.Object.ParentID())
	}

	// Uncovered — nothing serves any goal yet.
	unc, err := svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	if err != nil || len(unc.Goals) != 3 {
		t.Fatalf("Uncovered before linking: %v %v", err, ids(unc.Goals))
	}

	// LinkServer — a use-case serves a; the edge is on the use-case.
	uc := cwmapi.NewRecordObject("m", "UC-001", "use-case", "m", "A use-case", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: uc}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: a.ID, Server: uc.ID}); err != nil {
		t.Fatal(err)
	}
	edges, _ := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: a.ID, Kind: "serves", Direction: cwmapi.DirectionIn})
	if len(edges.Edges) != 1 || edges.Edges[0].From != uc.ID {
		t.Fatalf("serves edges into %s: %+v", a.ID, edges.Edges)
	}
	unc, _ = svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	for _, g := range unc.Goals {
		if g.ID == a.ID {
			t.Fatalf("%s is served yet still listed uncovered", a.ID)
		}
	}
	if len(unc.Goals) != 2 {
		t.Fatalf("Uncovered after linking: %v", ids(unc.Goals))
	}

	// Remove — b has no children (c moved away) and nothing serves it.
	if _, err := svc.Remove(ctx, api.RemoveInput{ID: b.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: b.ID}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("after Remove, ObjectGet err = %v, want ErrNotFound", err)
	}
}

func TestGoalsRemoveRefusesAGoalWithChildren(t *testing.T) {
	ctx, cwm, svc := setup(t)
	a := add(t, ctx, svc, "Parent", "")
	add(t, ctx, svc, "Child", a.ID)
	if _, err := svc.Remove(ctx, api.RemoveInput{ID: a.ID}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("Remove of a goal with a child: err = %v", err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: a.ID}); err != nil {
		t.Fatalf("the refused goal is gone: %v", err)
	}
}

func TestGoalsRemoveRefusesAServedGoal(t *testing.T) {
	ctx, cwm, svc := setup(t)
	a := add(t, ctx, svc, "Served", "")
	uc := cwmapi.NewRecordObject("m", "UC-001", "use-case", "m", "A use-case", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: uc}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: a.ID, Server: uc.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Remove(ctx, api.RemoveInput{ID: a.ID}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("Remove of a served goal: err = %v", err)
	}
}

func TestGoalsRefuseANonGoal(t *testing.T) {
	ctx, cwm, svc := setup(t)
	uc := cwmapi.NewRecordObject("m", "UC-001", "use-case", "m", "A use-case", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: uc}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Remove(ctx, api.RemoveInput{ID: uc.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("Remove of a use-case through the goals service: err = %v", err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "x", Parent: uc.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("Add under a use-case: err = %v", err)
	}
}

// A goal is always asymptotic: Add writes it so, and refuses a closable one
// (something a test can declare done is a feature).
func TestGoalsAreAlwaysAsymptotic(t *testing.T) {
	ctx, _, svc := setup(t)
	o, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "Trusted"})
	if err != nil || o.Goal.Reachability != "asymptotic" {
		t.Fatalf("Add: %+v, %v — want an asymptotic goal", o.Goal, err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "Ship it", Reachability: "closable"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("Add of a closable goal: err = %v, want refused", err)
	}
}

func TestGoalsReorderRefusesAPartialOrMixedList(t *testing.T) {
	ctx, _, svc := setup(t)
	a := add(t, ctx, svc, "A", "")
	b := add(t, ctx, svc, "B", "")
	c := add(t, ctx, svc, "C", a.ID)
	if _, err := svc.Reorder(ctx, api.ReorderInput{IDs: []string{a.ID}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("partial order: err = %v", err)
	}
	// Every goal under the module, plus one from under a: only the one-parent rule refuses it.
	if _, err := svc.Reorder(ctx, api.ReorderInput{IDs: []string{a.ID, b.ID, c.ID}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("two parents in one order: err = %v", err)
	}
}

func TestGoalsReparentRefusesACycle(t *testing.T) {
	ctx, _, svc := setup(t)
	a := add(t, ctx, svc, "A", "")
	c := add(t, ctx, svc, "C", a.ID)
	if _, err := svc.Reparent(ctx, api.ReparentInput{ID: a.ID, NewParent: c.ID}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("a goal under its own child: err = %v", err)
	}
}

func TestGoalsStayInTheirModule(t *testing.T) {
	ctx, cwm, svc := setup(t)
	cwmapp.Module(t, ctx, cwm, "other")
	o, err := svc.Add(ctx, api.AddInput{Module: "other", Title: "Elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, api.AddInput{Module: "m", Title: "A", Parent: o.Goal.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a goal of m under a goal of another module: err = %v", err)
	}
}

func TestGoalsLinkServerRefusesAMissingServer(t *testing.T) {
	ctx, _, svc := setup(t)
	a := add(t, ctx, svc, "A", "")
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: a.ID, Server: "m.nothing"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("link a missing server: err = %v", err)
	}
}

// THE GOALS TAB: every goal of the module's subtree — nested goals included,
// another module's left out — each with its state (initial when never moved),
// its reachability and what serves it; the state filter re-reads it.
func TestGoalsBoardListsTheWholeSubtree(t *testing.T) {
	ctx, cwm := cwmapp.New(t, goalslocal.Extension(), boardPack())
	goalsBoardScenario(t, ctx, cwm)
}

// The same board in the binary that hosts it: CWM in another process.
func TestGoalsBoardOverTheBus(t *testing.T) {
	ctx, cwm := cwmapp.RemoteHosting(t, []application.Extension{boardPack()}, goalslocal.Extension())
	goalsBoardScenario(t, ctx, cwm)
}

func boardPack() application.Extension {
	return cwmapp.Pack(fstest.MapFS{
		"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\nbadges: [{field: state, colors: {idea: gray, proposed: yellow}}]\n")},
		"workflows/goal.yaml": {Data: []byte("name: goal\ninitial: idea\nstates: [{name: idea}, {name: proposed}]\n" +
			"transitions: [{name: propose, from: [idea], to: proposed}]\n")},
		"object-types/goal.yaml": {Data: []byte("name: goal\ncapabilities: [want, lifecycle]\nworkflow: goal\n")},
	})
}

func goalsBoardScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) {
	t.Helper()
	cwmapp.Module(t, ctx, cwm, "m")
	cwmapp.Module(t, ctx, cwm, "n")
	svc, err := application.UseService[api.GoalsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	top := add(t, ctx, svc, "Ship the thing", "")
	sub := add(t, ctx, svc, "Ship the first half", top.ID)
	if _, err := svc.Add(ctx, api.AddInput{Module: "n", Title: "Elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: sub.ID, To: "proposed", By: cwmapi.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: top.ID, Server: sub.ID}); err != nil {
		t.Fatal(err)
	}

	page := boardJSON(t, ctx, svc, api.BoardInput{ID: "m"})
	for _, want := range []string{top.ID, sub.ID, `"text":"proposed"`, `"text":"idea"`, `"text":"asymptotic"`,
		`"text":"served by 1"`, `"text":"nothing serves it"`, `"create":{"type":"goal"}`, `"badge":2`} {
		if !strings.Contains(page, want) {
			t.Errorf("the board lacks %s", want)
		}
	}
	if strings.Contains(page, "Elsewhere") {
		t.Error("another module's goal is listed")
	}
	// The sub-goal comes right after the goal it sits under.
	if strings.Index(page, `"object":"`+top.ID+`"`) > strings.Index(page, `"object":"`+sub.ID+`"`) {
		t.Error("the tree is not read top to bottom")
	}
	page = boardJSON(t, ctx, svc, api.BoardInput{ID: "m", State: "proposed", Language: "fr"})
	if !strings.Contains(page, `"badge":1`) || !strings.Contains(page, "Objectifs") {
		t.Errorf("the state filter in French: %s", page)
	}
}

func boardJSON(t *testing.T, ctx context.Context, svc api.GoalsService, in api.BoardInput) string {
	t.Helper()
	d, err := svc.Board(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("the page is off the grammar: %v", err)
	}
	raw, _ := json.Marshal(d)
	return string(raw)
}

// What serves a goal is a want, or any object that is not work asked for: a
// request of its own pointing at the goal (serves) advances it — it does not
// serve it; a feature carrying a request still does.
func TestARequestServingAGoalIsNotOneOfItsServers(t *testing.T) {
	ctx, cwm := cwmapp.New(t, goalslocal.Extension(), cwmapp.Pack(fstest.MapFS{
		"capabilities/request.yaml": {Data: []byte("name: request\n")},
		"object-types/request.yaml": {Data: []byte("name: request\ncapabilities: [request]\n")},
		"object-types/feature.yaml": {Data: []byte("name: feature\ncapabilities: [want]\n")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.GoalsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	g := add(t, ctx, svc, "Ship the thing", "")
	create := func(typ, name string, caps map[string]map[string]any) string {
		o := cwmapi.Object{Type: typ, Capabilities: caps}
		o.Parent, o.Name = "m", name
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o})
		if err != nil {
			t.Fatal(err)
		}
		return out.Object.ID
	}
	rq := create("request", "Fix the thing", map[string]map[string]any{"request": {"kind": "fix"}})
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: g.ID, Server: rq}); err != nil {
		t.Fatal(err)
	}
	unc, err := svc.Uncovered(ctx, api.UncoveredInput{Module: "m"})
	if err != nil || len(unc.Goals) != 1 || unc.Goals[0].ID != g.ID {
		t.Fatalf("a goal only a request points at: uncovered %v, %v; want it uncovered", ids(unc.Goals), err)
	}
	f := create("feature", "The thing", map[string]map[string]any{"request": {"kind": "build"}, "want": {"reachability": "closable"}})
	if _, err := svc.LinkServer(ctx, api.LinkServerInput{Goal: g.ID, Server: f}); err != nil {
		t.Fatal(err)
	}
	if unc, err = svc.Uncovered(ctx, api.UncoveredInput{Module: "m"}); err != nil || len(unc.Goals) != 0 {
		t.Fatalf("a feature with work on it serves the goal: uncovered %v, %v; want none", ids(unc.Goals), err)
	}
	if _, err := svc.Remove(ctx, api.RemoveInput{ID: g.ID}); err == nil || !strings.Contains(err.Error(), rq) {
		t.Errorf("removing a goal a request points at: %v; want refused, naming it", err)
	}
}
