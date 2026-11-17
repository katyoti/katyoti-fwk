// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// decisionPack is what a decision needs from the catalog: its type, naming the
// `decision` workflow, and that workflow AS SHIPPED by ktw-methods.
func decisionPack(t testing.TB) application.Extension {
	return cwmapp.Pack(fstest.MapFS{
		"object-types/decision.yaml": {Data: []byte("name: decision\nfolder: decisions\ncapabilities: [container, content, decision, lifecycle]\nworkflow: decision\n")},
		"workflows/decision.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/decision.yaml")},
	})
}

// a person answering, as the GUI or an agent's inbox answer records it.
var who = cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.DecisionsService) {
	t.Helper()
	return setupIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		return cwmapp.New(tb, append(exts, decisionPack(tb))...)
	})
}

// setupRemote is the shape of the binary hosting this service: CWM is in
// another process, reached through the generated bus client.
func setupRemote(t *testing.T) (context.Context, cwmapi.CWMService, api.DecisionsService) {
	t.Helper()
	return setupIn(t, func(tb testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
		return cwmapp.RemoteHosting(tb, []application.Extension{decisionPack(tb)}, exts...)
	})
}

func setupIn(t *testing.T, boot func(testing.TB, ...application.Extension) (context.Context, cwmapi.CWMService)) (context.Context, cwmapi.CWMService, api.DecisionsService) {
	t.Helper()
	ctx, cwm := boot(t, decisionslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.DecisionsService](ctx)
	if err != nil {
		t.Fatalf("resolve DecisionsService: %v", err)
	}
	return ctx, cwm, svc
}

func open(t *testing.T, ctx context.Context, svc api.DecisionsService, q string, options ...string) api.Decision {
	t.Helper()
	out, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: q, Options: options})
	if err != nil {
		t.Fatalf("Open %q: %v", q, err)
	}
	return out.Decision
}

func stored(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, map[string]any) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	return got.Object, got.Object.Capabilities["decision"]
}

// TestDecisionsService drives every verb against a real CWM service and reads
// each effect back through the Object API.
func TestDecisionsService(t *testing.T) {
	ctx, cwm, svc := setup(t)
	decisionsScenario(t, ctx, cwm, svc)
}

// TestDecisionsServiceOverTheBus runs the same scenario with CWM in another
// process, as in the binary that hosts this service.
func TestDecisionsServiceOverTheBus(t *testing.T) {
	ctx, cwm, svc := setupRemote(t)
	decisionsScenario(t, ctx, cwm, svc)
}

func decisionsScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, svc api.DecisionsService) {
	t.Helper()

	// Open — a decision with a raised date and the question as prose.
	opened, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: "Which store?", Options: []string{"files"},
		Context: "The index must survive a restart."})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	d := opened.Decision
	obj, st := stored(t, ctx, cwm, d.ID)
	prose := obj.Body
	if obj.Type != "decision" || d.Status != api.StatusOpen || st["raised"] == nil || st["resolved"] != nil || !strings.Contains(prose, "## Question") {
		t.Fatalf("opened decision: type %q status %q state %v prose %q", obj.Type, d.Status, st, prose)
	}
	if _, has := obj.Capabilities["content"]; has {
		t.Fatalf("a decision's state is its lifecycle, never a content group: %v", obj.Capabilities["content"])
	}

	// ProposeOption — the option is stored.
	if _, err := svc.ProposeOption(ctx, api.ProposeOptionInput{ID: d.ID, Option: "sqlite"}); err != nil {
		t.Fatal(err)
	}
	_, st = stored(t, ctx, cwm, d.ID)
	if opts, _ := st["options"].([]any); len(opts) != 2 {
		t.Fatalf("options after ProposeOption: %v", st["options"])
	}

	// ListOpen — the decision is open.
	list, err := svc.ListOpen(ctx, api.ListOpenInput{Module: "m"})
	if err != nil || len(list.Decisions) != 1 || list.Decisions[0].ID != d.ID {
		t.Fatalf("ListOpen before resolving: %v %+v", err, list.Decisions)
	}

	// Resolve — resolved date, selected, status answered, a Decision section.
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}, Rationale: "Diffable."}); err != nil {
		t.Fatal(err)
	}
	obj, st = stored(t, ctx, cwm, d.ID)
	life, _ := obj.Lifecycle()
	prose = obj.Body
	if st["resolved"] == nil || life.State != api.StatusApplies || !strings.Contains(prose, "## Question") || !strings.Contains(prose, "## Decision\n\nfiles. Diffable.") {
		t.Fatalf("resolved decision: lifecycle %+v state %v prose %q", life, st, prose)
	}
	list, _ = svc.ListOpen(ctx, api.ListOpenInput{Module: "m"})
	if len(list.Decisions) != 0 {
		t.Fatalf("ListOpen after resolving: %+v", list.Decisions)
	}

	// Supersede — the edge on the newer decision, the older marked superseded.
	n := open(t, ctx, svc, "Which store, now that we have an index?")
	if _, err := svc.Supersede(ctx, api.SupersedeInput{ID: d.ID, By: n.ID, Actor: who}); err != nil {
		t.Fatal(err)
	}
	edges, _ := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: d.ID, Kind: "supersedes", Direction: cwmapi.DirectionIn})
	if len(edges.Edges) != 1 || edges.Edges[0].From != n.ID {
		t.Fatalf("supersedes edges into %s: %+v", d.ID, edges.Edges)
	}
	obj, _ = stored(t, ctx, cwm, d.ID)
	if l, _ := obj.Lifecycle(); l.State != api.StatusSuperseded {
		t.Fatalf("superseded decision lifecycle = %+v", l)
	}
	list, _ = svc.ListOpen(ctx, api.ListOpenInput{Module: "m"})
	if len(list.Decisions) != 1 || list.Decisions[0].ID != n.ID {
		t.Fatalf("ListOpen at the end: %+v", list.Decisions)
	}
}

func TestDecisionsAnsweredTakesNoNewOption(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Q", "a")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposeOption(ctx, api.ProposeOptionInput{ID: d.ID, Option: "b"}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("ProposeOption on an answered decision: err = %v", err)
	}
}

func TestDecisionsDuplicateOptionRefused(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Q", "a")
	if _, err := svc.ProposeOption(ctx, api.ProposeOptionInput{ID: d.ID, Option: "a"}); !errors.Is(err, cwmapi.ErrAlreadyExists) {
		t.Fatalf("a duplicate option: err = %v", err)
	}
}

func TestDecisionsResolveOnlyOnce(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Q")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"y"}}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("a second Resolve: err = %v", err)
	}
}

func TestDecisionsResolveRefusesAnUnproposedChoice(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Q", "a", "b")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"c"}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("resolving with an option nobody proposed: err = %v", err)
	}
}

func TestDecisionsSupersedeRefusesAnOpenDecision(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Old")
	n := open(t, ctx, svc, "New")
	if _, err := svc.Supersede(ctx, api.SupersedeInput{ID: d.ID, By: n.ID, Actor: who}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("superseding an open decision: err = %v", err)
	}
}

func TestDecisionsSupersedeOnlyOnce(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Old")
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	n1, n2 := open(t, ctx, svc, "New 1"), open(t, ctx, svc, "New 2")
	if _, err := svc.Supersede(ctx, api.SupersedeInput{ID: d.ID, By: n1.ID, Actor: who}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Supersede(ctx, api.SupersedeInput{ID: d.ID, By: n2.ID, Actor: who}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Fatalf("superseding a superseded decision: err = %v", err)
	}
}

func TestDecisionsRefuseANonDecision(t *testing.T) {
	ctx, cwm, svc := setup(t)
	g := cwmapi.NewRecordObject("m", "G-001", "goal", "m", "A goal", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: g}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposeOption(ctx, api.ProposeOptionInput{ID: g.ID, Option: "a"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("ProposeOption on a goal: err = %v", err)
	}
}
