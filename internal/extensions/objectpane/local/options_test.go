// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TRANSITION OPTIONS, on a real CWM: each lifecycle of an object — its own,
// then a capability's — with the transitions from its state, each judged for
// who would move by CWM's dry-run; a guard calling a costly function, or one
// that cannot answer now, is left to the move; nothing is written.

// actions answers the capabilities' functions: score() is 3, heavy() is
// counted, broken() fails.
type actions struct {
	mu    sync.Mutex
	calls map[string]int
}

func (a *actions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func (a *actions) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	a.calls[in.URI]++
	a.mu.Unlock()
	switch in.URI {
	case "x:broken/get":
		return actionsapi.RunOutput{}, errors.New("not reachable now")
	case "x:heavy/get":
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`true`)}, nil
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`3`)}, nil
}

func (a *actions) count(uri string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls[uri]
}

var pack = fstest.MapFS{
	"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\nbadges: [{field: state, colors: {proposed: yellow, accepted: teal}}]\n")},
	"capabilities/scores.yaml": {Data: []byte(`name: scores
functions:
  - {name: score, action: 'x:score/get', returns: number}
  - {name: heavy, action: 'x:heavy/get', returns: boolean, costly: true}
  - {name: broken, action: 'x:broken/get', returns: boolean}
`)},
	"capabilities/review.yaml": {Data: []byte("name: review\nworkflow: {name: review, state: status}\nbadges: [{field: status, colors: {done: green}}]\n")},
	"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")},
	"object-types/goal.yaml":   {Data: []byte("name: goal\ntree: node\ncapabilities: [lifecycle, scores]\nworkflow: approval\n")},
	"workflows/approval.yaml": {Data: []byte(`name: approval
label: Approval
initial: proposed
states: [{name: proposed}, {name: accepted}, {name: closed, terminal: true}]
transitions:
  - {name: accept, from: [proposed], to: accepted, gate: "by.kind == 'person' && score() > 2"}
  - {name: rush, from: [proposed], to: accepted, gate: "score() > 5", force: person}
  - {name: audit, from: [proposed], to: accepted, gate: "heavy()"}
  - {name: probe, from: [proposed], to: accepted, gate: "broken()"}
  - {name: plain, from: [proposed], to: closed}
  - {name: reopen, from: [accepted], to: proposed}
`)},
	"workflows/review.yaml": {Data: []byte(`name: review
initial: open
states: [{name: open}, {name: done, terminal: true}]
transitions:
  - {name: finish, from: [open], to: done}
`)},
}

func world(t *testing.T) (context.Context, cwmapi.CWMService, api.ObjectPaneService, *actions) {
	t.Helper()
	acts := &actions{calls: map[string]int{}}
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(pack), panelocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
		// The history is the journal's: CWM's events, recorded.
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), journallocal.Extension(), cwmapp.SystemModules(t),
	)
	// The journal records on its own goroutine: drain it before the
	// workspace is removed.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	cwmapp.Module(t, ctx, cwm, "m")
	g := cwmapi.NewContainerObject("m", "g1", "goal", "m", "g1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: g}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, acts
}

func byName(lo api.LifecycleOptions) map[string]api.TransitionOption {
	out := map[string]api.TransitionOption{}
	for _, tr := range lo.Transitions {
		out[tr.Name] = tr
	}
	return out
}

func TestEachTransitionIsJudgedByTheDryRun(t *testing.T) {
	ctx, cwm, svc, acts := world(t)
	out, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.g1"})
	if err != nil || len(out.Lifecycles) != 1 {
		t.Fatalf("options: %+v, %v", out, err)
	}
	lo := out.Lifecycles[0]
	if lo.Workflow != "approval" || lo.WorkflowLabel != "Approval" || lo.State != "proposed" || len(lo.States) != 3 || lo.Terminal {
		t.Fatalf("lifecycle: %+v", lo)
	}
	tr := byName(lo)
	if len(tr) != 5 {
		t.Fatalf("transitions from proposed: %v — reopen leaves accepted, not proposed", tr)
	}
	if o := tr["accept"]; o.GuardStatus != api.GuardHolds || !o.Allowed {
		t.Errorf("accept for a person: %+v", o)
	}
	if o := tr["rush"]; o.GuardStatus != api.GuardFails || o.Allowed || o.Reason == "" || o.Force != cwmapi.ForcePerson {
		t.Errorf("rush: %+v", o)
	} else if a, ok := cwmapi.AnswerOf(o.Calls, "score"); !ok || a.Answer != 3.0 {
		t.Errorf("rush carries what score() answered: %+v", o.Calls)
	}
	if o := tr["plain"]; o.GuardStatus != api.GuardNone || !o.Allowed {
		t.Errorf("plain: %+v", o)
	}
	if o := tr["audit"]; o.GuardStatus != api.GuardAtMove || !o.Allowed {
		t.Errorf("audit (costly): %+v", o)
	}
	if n := acts.count("x:heavy/get"); n != 0 {
		t.Errorf("listing the options called the costly function %d times", n)
	}
	if a, ok := cwmapi.AnswerOf(tr["audit"].Calls, "heavy"); !ok || !a.AtMove || a.Error == "" {
		t.Errorf("audit: heavy() answers decided at the move: %+v", tr["audit"].Calls)
	}
	if o := tr["probe"]; o.GuardStatus != api.GuardAtMove || !o.Allowed {
		t.Errorf("probe (a function that cannot answer): %+v", o)
	}
	// The reason is the human part: the function's own message, not the
	// guard prefix or the expression's position and caret.
	if r := tr["probe"].Reason; !strings.Contains(r, "not reachable now") || strings.Contains(r, "\n") || strings.Contains(r, "(1:1)") || strings.HasPrefix(r, "the guard") {
		t.Errorf("probe's reason = %q, want the function's message alone", r)
	}

	// Nothing was written: the object is still where it was.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.g1"})
	if err != nil {
		t.Fatal(err)
	}
	if life, _ := got.Object.Lifecycle(); life.State != "" && life.State != "proposed" {
		t.Errorf("the options moved the object: %+v", life)
	}
}

// The move itself runs the costly function the options left to it.
func TestTheMoveRunsTheCostlyFunctionTheOptionsLeft(t *testing.T) {
	ctx, cwm, svc, acts := world(t)
	if _, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.g1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.g1", To: "accepted", Transition: "audit", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatalf("audit: heavy() holds at the move: %v", err)
	}
	if n := acts.count("x:heavy/get"); n != 1 {
		t.Errorf("the move ran heavy() %d times, want once", n)
	}
}

// A guard is judged for who would move: it holds for a person and fails for
// an agent.
func TestTheOptionsAreJudgedForWhoMoves(t *testing.T) {
	ctx, _, svc, _ := world(t)
	out, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.g1", By: cwmapi.Actor{ID: "bot", Kind: cwmapi.ActorAgent}})
	if err != nil {
		t.Fatal(err)
	}
	if o := byName(out.Lifecycles[0])["accept"]; o.GuardStatus != api.GuardFails || o.Allowed || o.Reason == "" {
		t.Errorf("accept for an agent: %+v", o)
	}
}

// A capability following its own workflow is a second lifecycle, after the
// object's own; a terminal state offers no transition.
func TestACapabilitysWorkflowIsASecondLifecycle(t *testing.T) {
	ctx, cwm, svc, _ := world(t)
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m.g1", Capability: "review"}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.g1"})
	if err != nil || len(out.Lifecycles) != 2 {
		t.Fatalf("options: %+v, %v", out, err)
	}
	rv := out.Lifecycles[1]
	if rv.Capability != "review" || rv.State != "open" || byName(rv)["finish"].GuardStatus != api.GuardNone {
		t.Fatalf("review lifecycle: %+v", rv)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.g1", Capability: "review", To: "done", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatal(err)
	}
	// The history is the journal's (recorded by its own observer): wait for it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ = svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.g1"})
		if len(out.Lifecycles[1].History) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rv := out.Lifecycles[1]; !rv.Terminal || len(rv.Transitions) != 0 || rv.State != "done" || len(rv.History) != 1 || rv.History[0].To != "done" {
		t.Errorf("after finish: %+v", rv)
	}
	if own := out.Lifecycles[0]; len(own.History) != 0 {
		t.Errorf("the object's own lifecycle shows the review's move: %+v", own.History)
	}
	// Each lifecycle's states coloured as its capability's badge colours them.
	if own, rv := out.Lifecycles[0], out.Lifecycles[1]; own.StateColors["accepted"] != "teal" || rv.StateColors["done"] != "green" {
		t.Errorf("state colours: own %v, review %v", own.StateColors, rv.StateColors)
	}
}

func TestOptionsOfAnObjectThatDoesNotExist(t *testing.T) {
	ctx, _, svc, _ := world(t)
	if _, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: "m.ghost"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("a missing object: %v", err)
	}
}

// TestTheHeadersTimesAreTheJournals: the pane's created and changed times are
// the journal's derived read — no object stores them — and a transition is a
// change.
func TestTheHeadersTimesAreTheJournals(t *testing.T) {
	ctx, cwm, svc, _ := world(t)
	waitFor := func(ok func(api.DetailOutput) bool) api.DetailOutput {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			d, err := svc.Detail(ctx, api.DetailInput{ID: "m.g1"})
			if err != nil {
				t.Fatal(err)
			}
			if ok(d) || time.Now().After(deadline) {
				return d
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	d := waitFor(func(d api.DetailOutput) bool { return !d.CreatedAt.IsZero() })
	if d.CreatedAt.IsZero() || !d.UpdatedAt.Equal(d.CreatedAt) {
		t.Fatalf("after the create: created %s changed %s", d.CreatedAt, d.UpdatedAt)
	}
	created := d.CreatedAt
	time.Sleep(10 * time.Millisecond)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.g1", To: "closed", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatal(err)
	}
	d = waitFor(func(d api.DetailOutput) bool { return d.UpdatedAt.After(created) })
	if !d.CreatedAt.Equal(created) || !d.UpdatedAt.After(created) {
		t.Errorf("after a transition: created %s (was %s) changed %s", d.CreatedAt, created, d.UpdatedAt)
	}
	if _, ok := d.Object.Capability("provenance"); ok {
		t.Error("the object stores its times")
	}
}
