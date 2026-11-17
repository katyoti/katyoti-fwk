// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// TRANSITIONS CAN BE SUBSCRIBED TO. object.transitioned and
// object.transition-refused name the after-points on_transitioned and
// on_transition_refused — declared by the event types alone: an async entry
// there is matched under the cascade guards, a plain one journaled and run;
// on_transition stays the synchronous gate, and the move's own write still
// fires on_update, marked operation transition.

const (
	subMoved       = "x:react/moved"
	subMovedToX    = "x:react/moved-to-x"
	subMovedCaused = "x:react/moved-caused"
	subRefused     = "x:react/refused"
	subEdited      = "x:react/edited"
	subPackMoved   = "x:react/pack-moved"
	afterMoved     = "x:after/moved"
)

// moveFlow: idea → proposed freely; proposed → accepted under a guard that
// calls score() (which answers 3), forcible by anyone.
func moveFlow() api.Workflow {
	return api.Workflow{Name: "moveflow", Initial: "idea",
		States: []api.WorkflowState{{Name: "idea"}, {Name: "proposed"}, {Name: "accepted"}},
		Transitions: []api.WorkflowTransition{
			{Name: "propose", From: []string{"idea"}, To: "proposed"},
			{Name: "accept", From: []string{"proposed"}, To: "accepted", Gate: "score() > 5", Force: api.ForceAnyone},
		}}
}

// transitionSubsConfig: a `tracked` type subscribes to its moves (plain, only
// to 'x', accepting caused moves), to its refused moves and to its edits, and
// runs a plain entry on_transitioned; a pack's `hook` contribution subscribes
// to every tracked object's moves; an `untracked` type follows the same
// workflow and subscribes to nothing.
func transitionSubsConfig(t *testing.T) *CWMServiceConfig {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "lifecycle"},
		{Name: "gates", Functions: []api.CapabilityFunction{
			{Name: "score", Action: "x:score/get", Returns: api.FunctionReturnsNumber},
		}},
	}
	caps := []string{"container", "lifecycle", "gates"}
	cfg.CatalogTypes.Workflows = []api.Workflow{moveFlow()}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "tracked", Capabilities: caps, Workflow: "moveflow", Hooks: api.Hooks{
			api.HookOnTransitioned: {
				{URI: subMoved, Async: &api.AsyncSpec{}},
				{URI: subMovedToX, When: "event.payload.to == 'x'", Async: &api.AsyncSpec{}},
				{URI: subMovedCaused, Async: &api.AsyncSpec{AcceptsCaused: true}},
				{URI: afterMoved, Input: map[string]any{"to": "${event.payload.to}", "by": "${event.payload.by.id}"}},
			},
			api.HookOnTransitionRefused: {{URI: subRefused, Async: &api.AsyncSpec{}}},
			api.HookOnUpdate:            {{URI: subEdited, Async: &api.AsyncSpec{}}},
		}},
		{Name: "untracked", Capabilities: caps, Workflow: "moveflow"},
	}
	cfg.CatalogTypes.Hooks = []api.Hook{{Name: "tracked-moves", Provider: "ktw-x", Type: api.HookOnTransitioned,
		URI: subPackMoved, Filter: api.ObjectFilter{Types: []string{"tracked"}}, Async: &api.AsyncSpec{}}}
	return cfg
}

// scoringActions answers score() with 3 and records every other call.
type scoringActions struct{ *inputActions }

func scoreActions() *inputActions { return newInputActions() }

func (a scoringActions) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI == "x:score/get" {
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage("3")}, nil
	}
	return a.inputActions.Run(ctx, in)
}

func (a scoringActions) Describe(_ stdctx.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func openTransitionSubs(t *testing.T, actions actionsapi.ActionService, objs ...api.Object) (*Service, stdctx.Context) {
	t.Helper()
	svc, ctx, _ := openAsync(t, transitionSubsConfig(t), actions)
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range objs {
		module, _, _ := strings.Cut(o.ID, ".")
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return svc, ctx
}

func moveTo(ctx stdctx.Context, svc *Service, id, to string) error {
	_, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: id, To: to, By: agent, Note: "because"})
	return err
}

func payloadOf(t *testing.T, ev api.MatchedEvent) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("event payload %s: %v", ev.Payload, err)
	}
	return p
}

// (a) An async on_transitioned entry gives one match carrying the transitioned
// event as published (from, to, by); a `when` on event.payload.to that does
// not hold gives none.
func TestAnAsyncOnTransitionedEntryMatchesAMove(t *testing.T) {
	svc, ctx, bus := openAsync(t, transitionSubsConfig(t), scoringActions{scoreActions()})
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.t1", "tracked")}); err != nil {
		t.Fatal(err)
	}
	var published captured
	_ = bus.Subscribe(api.ObjectTransitionedSubject("t", "u", "w", api.SubjectToken("billing.t1")), published.on)
	if err := moveTo(ctx, svc, "billing.t1", "proposed"); err != nil {
		t.Fatal(err)
	}
	ms := pendingOf(svc, subMoved)
	if len(ms) != 1 || len(ms[0].Events) != 1 {
		t.Fatalf("%d match(es) of %s, want 1: %+v", len(ms), subMoved, ms)
	}
	ev := ms[0].Events[0]
	p := payloadOf(t, ev)
	by, _ := p["by"].(map[string]any)
	if p["from"] != "idea" || p["to"] != "proposed" || by["id"] != agent.ID || p["transition"] != "propose" || p["workflow"] != "moveflow" {
		t.Errorf("the matched event's payload = %v, want from idea to proposed by %s", p, agent.ID)
	}
	if ev.Action != api.TransitionedEventAction || ev.Event != "object.transitioned" || ms[0].Subscription.Point != api.HookOnTransitioned {
		t.Errorf("matched event = %+v at %s", ev, ms[0].Subscription.Point)
	}
	pub := published.all()
	if len(pub) != 1 || pub[0].(events.ObjectTransitionedPayload).EventID != ev.EventID {
		t.Errorf("published %+v; the match must name the published event %s", pub, ev.EventID)
	}
	if n := len(pendingOf(svc, subMovedToX)); n != 0 {
		t.Errorf("when event.payload.to == 'x': %d match(es) for a move to proposed, want none", n)
	}
}

// (b) A refused gated move gives an on_transition_refused match carrying the
// guard and what its functions answered; a dry run and a forced move give none.
func TestARefusedMoveMatchesOnTransitionRefused(t *testing.T) {
	svc, ctx := openTransitionSubs(t, scoringActions{scoreActions()}, typed("billing.t1", "tracked"))
	if err := moveTo(ctx, svc, "billing.t1", "proposed"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.t1"})
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.t1", To: "accepted", By: agent, DryRun: true}); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("dry run: %v", err)
	}
	if n := len(pendingOf(svc, subRefused)); n != 0 {
		t.Fatalf("a dry run matched on_transition_refused (%d)", n)
	}
	if err := moveTo(ctx, svc, "billing.t1", "accepted"); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("the gated move: %v, want refused", err)
	}
	ms := pendingOf(svc, subRefused)
	if len(ms) != 1 {
		t.Fatalf("%d match(es) of %s, want 1", len(ms), subRefused)
	}
	ev := ms[0].Events[0]
	p := payloadOf(t, ev)
	calls, _ := p["calls"].([]any)
	if p["guard"] != "score() > 5" || p["from"] != "proposed" || p["to"] != "accepted" || p["reason"] == "" || len(calls) != 1 {
		t.Errorf("the refusal's payload = %v, want the guard and score()'s answer", p)
	}
	if c, _ := calls[0].(map[string]any); c["function"] != "score" || c["answer"] != 3.0 {
		t.Errorf("the call = %v, want score() answering 3", calls[0])
	}
	if ev.Action != api.TransitionRefusedEventAction || ev.Event != "object.transition-refused" || ev.EventID == "" || ev.Revision != got.Revision {
		t.Errorf("matched event = %+v, want stamped with the object's current revision %s", ev, got.Revision)
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.t1", To: "accepted", By: agent, Force: true}); err != nil {
		t.Fatalf("forced: %v", err)
	}
	if ms := pendingOf(svc, subRefused); len(ms) != 1 || len(ms[0].Events) != 1 {
		t.Errorf("a forced move matched on_transition_refused: %+v", ms)
	}
}

// (c) A pack's `hook` contribution at on_transitioned filtered on a type
// matches that type's objects in two modules, and no object of another type.
func TestAPackSubscriptionToMovesFollowsItsFilter(t *testing.T) {
	svc, ctx := openTransitionSubs(t, scoringActions{scoreActions()},
		typed("billing.t1", "tracked"), typed("sales.t2", "tracked"), typed("billing.u1", "untracked"))
	for _, id := range []string{"billing.t1", "sales.t2", "billing.u1"} {
		if err := moveTo(ctx, svc, id, "proposed"); err != nil {
			t.Fatalf("move %s: %v", id, err)
		}
	}
	var got []string
	for _, m := range pendingOf(svc, subPackMoved) {
		got = append(got, m.Module+"/"+m.ObjectID)
		if m.Subscription.Source != "hook:ktw-x/tracked-moves" {
			t.Errorf("match source %q", m.Subscription.Source)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"billing/billing.t1", "sales/sales.t2"}) {
		t.Errorf("the pack's subscription matched %v, want the two tracked objects", got)
	}
}

// (d) A move made by a reaction of S does not match S; another reaction's
// move does (staged in its outbox).
func TestAMoveByASubscriptionsReactionDoesNotMatchIt(t *testing.T) {
	svc, ctx := openTransitionSubs(t, scoringActions{scoreActions()}, typed("billing.t1", "tracked"), typed("billing.t2", "tracked"))
	self := api.SubscriptionID("type:tracked", api.HookOnTransitioned, subMovedCaused)
	own := api.ReactionID{Subscription: self, EventID: "e-1"}.String()
	other := api.ReactionID{Subscription: api.SubscriptionID("type:tracked", api.HookOnUpdate, subEdited), EventID: "e-2"}.String()
	for _, r := range []struct{ reaction, id string }{{own, "billing.t1"}, {other, "billing.t2"}} {
		c := cause.Cause{Kind: cause.KindReaction, ID: r.reaction, Depth: 1}
		if err := moveTo(cause.WithCause(ctx, c), svc, r.id, "proposed"); err != nil {
			t.Fatal(err)
		}
	}
	staged := map[string]int{}
	for _, o := range svc.outbox().list() {
		staged[o.Reaction] = o.Matches
	}
	if staged[other] != 1 {
		t.Fatalf("another subscription's reaction staged %d match(es), want 1", staged[other])
	}
	if staged[own] != 0 {
		t.Errorf("a reaction's move matched its own subscription (%d staged): it reacts to itself", staged[own])
	}
}

// (e) An uncaused move still fires on_update on its own write — one match, as
// before — and that write's event says operation transition; an edit's says
// none.
func TestAMovesOwnWriteStillMatchesOnUpdateMarkedTransition(t *testing.T) {
	svc, ctx := openTransitionSubs(t, scoringActions{scoreActions()}, typed("billing.t1", "tracked"), typed("billing.t2", "tracked"))
	if err := moveTo(ctx, svc, "billing.t1", "proposed"); err != nil {
		t.Fatal(err)
	}
	ms := pendingOf(svc, subEdited)
	if len(ms) != 1 || len(ms[0].Events) != 1 {
		t.Fatalf("on_update after a move: %+v, want exactly one match", ms)
	}
	if p := payloadOf(t, ms[0].Events[0]); p["operation"] != api.OperationTransition {
		t.Errorf("the move's write: event.payload = %v, want operation transition", p)
	}
	o := typed("billing.t2", "tracked")
	o.Description = "edited"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	for _, m := range pendingOf(svc, subEdited) {
		if m.ObjectID == "billing.t2" && len(m.Events[0].Payload) != 0 {
			t.Errorf("an edit's event payload = %s, want empty", m.Events[0].Payload)
		}
	}
}

// (f) A plain on_transitioned entry is journaled — kept for the next start
// when no action service answers — and run, with the event as its input.
func TestAPlainOnTransitionedEntryIsJournaledAndRuns(t *testing.T) {
	t.Run("journaled", func(t *testing.T) {
		svc, ctx := openTransitionSubs(t, nil, typed("billing.t1", "tracked"))
		if err := moveTo(ctx, svc, "billing.t1", "proposed"); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range svc.hooks().pending() {
			if e.Point == api.HookOnTransitioned && e.URI == afterMoved && e.ObjectID == "billing.t1" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d journaled on_transitioned call(s), want 1", n)
		}
	})
	t.Run("runs", func(t *testing.T) {
		acts := scoreActions()
		svc, ctx := openTransitionSubs(t, scoringActions{acts}, typed("billing.t1", "tracked"))
		if err := moveTo(ctx, svc, "billing.t1", "proposed"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the on_transitioned entry runs", func() bool { return acts.callsTo(afterMoved) == 1 })
		var in map[string]any
		_ = json.Unmarshal(acts.inputOf(afterMoved), &in)
		if in["to"] != "proposed" || in["by"] != agent.ID {
			t.Errorf("the entry's input = %v, want the move's to and by", in)
		}
		if n := acts.callsTo(subMoved); n != 0 {
			t.Errorf("the async entry ran %d time(s); CWM never runs it", n)
		}
	})
}
