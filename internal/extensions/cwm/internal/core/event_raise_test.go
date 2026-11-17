// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// RAISING AN EVENT THAT IS NOT AN OBJECT WRITE. A pack declares `code.changed`
// at the point on_code_change — no hook-type — and raises it on the module
// object it concerns: stamped like a write's event, published, matched at the
// point under the cascade guards; the hook vocabulary reads it as `event`.

const (
	codePoint     = api.HookPoint("on_code_change")
	subCode       = "x:react/code"
	subCodeCaused = "x:react/code-caused"
	subGoMod      = "x:react/go-mod"
	afterCode     = "x:after/code"
	afterUpdated  = "x:after/updated-event"
)

// raiseConfig: a `code-module` subscribes to code.changed three ways (plain,
// accepting caused writes, only when go.mod changed) and runs a plain on_*
// entry at the point; on_update it runs an entry whose `when` reads the write's
// event. The event type is the only declaration of the point.
func raiseConfig(t *testing.T) *CWMServiceConfig {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "code-module", Capabilities: []string{"container"}, Hooks: api.Hooks{
			codePoint: {
				{URI: subCode, Async: &api.AsyncSpec{}},
				{URI: subCodeCaused, Async: &api.AsyncSpec{AcceptsCaused: true}},
				{URI: subGoMod, When: "'go.mod' in event.payload.paths", Async: &api.AsyncSpec{}},
				{URI: afterCode, Input: map[string]any{"event": "${event.type}", "first": "${event.payload.paths[0]}"}},
			},
			api.HookOnUpdate: {{URI: afterUpdated, When: "event.type == 'object.updated'"}},
		}},
	}
	cfg.CatalogTypes.EventTypes = []api.EventType{{Name: "code.changed", Point: codePoint, Provider: "ktw-gocode"}}
	return cfg
}

// inputActions records what each action received.
type inputActions struct {
	*durableActions
	mu     sync.Mutex
	inputs map[string]json.RawMessage
}

func newInputActions() *inputActions {
	return &inputActions{durableActions: newDurableActions(), inputs: map[string]json.RawMessage{}}
}

func (a *inputActions) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	a.inputs[in.URI] = in.Input
	a.mu.Unlock()
	return a.durableActions.Run(ctx, in)
}

func (a *inputActions) inputOf(uri string) json.RawMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.inputs[uri]
}

func paths(ps ...string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"paths": ps})
	return raw
}

// (1) A raised event is stamped (id, time, the object's revision, the cause),
// published for the journal, and matched at its point on the object; a plain
// entry there runs with the event as its input. Under a reaction it is staged
// in the outbox; at the depth limit it matches nothing.
func TestARaisedEventIsStampedPublishedAndMatchedAtItsPoint(t *testing.T) {
	cfg := raiseConfig(t)
	acts := newInputActions()
	svc, ctx, bus := openAsync(t, cfg, acts)
	defer svc.Close()
	created, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.code", "code-module")})
	if err != nil {
		t.Fatal(err)
	}
	var published captured
	_ = bus.Subscribe(api.ObjectEventRaisedSubject("t", "u", "w", api.SubjectToken("billing.code")), published.on)

	who := cause.Cause{Kind: cause.KindPerson, ID: "cs"}
	out, err := svc.EventRaise(cause.WithCause(ctx, who), api.EventRaiseInput{Type: "code.changed", ObjectID: "billing.code", Payload: paths("go.mod", "main.go")})
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	if out.EventID == "" || out.At.IsZero() || time.Since(out.At) > time.Minute || out.Cause == nil || *out.Cause != who {
		t.Errorf("stamped = %+v, want an id, the time and the cause %v", out.EventMeta, who)
	}
	if out.Revision != created.Revision {
		t.Errorf("revision %q, want the object's current %q (read, not changed)", out.Revision, created.Revision)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.code"})
	if got.Revision != created.Revision {
		t.Errorf("raising changed the object: revision %q -> %q", created.Revision, got.Revision)
	}

	pub := published.all()
	if len(pub) != 1 {
		t.Fatalf("%d event(s) published, want 1", len(pub))
	}
	p := pub[0].(api.ObjectRaisedEventPayload)
	if p.EventID != out.EventID || p.Event != "code.changed" || p.ObjectID != "billing.code" || p.Module != "billing" ||
		p.Type != "code-module" || p.Summary != `{"paths":["go.mod","main.go"]}` {
		t.Errorf("published = %+v", p)
	}

	ms := pendingOf(svc, subCode)
	if len(ms) != 1 {
		t.Fatalf("%d match(es) of %s, want 1", len(ms), subCode)
	}
	m := ms[0]
	if m.Subscription.Point != codePoint || m.ObjectID != "billing.code" || m.Module != "billing" || m.Depth != 0 {
		t.Errorf("match = %+v", m)
	}
	ev := m.Events[0]
	if ev.EventID != out.EventID || ev.Action != api.RaisedEventAction || ev.Event != "code.changed" ||
		string(ev.Payload) != `{"paths":["go.mod","main.go"]}` || ev.Cause == nil || *ev.Cause != who {
		t.Errorf("matched event = %+v", ev)
	}
	if m.ID != (api.ReactionID{Subscription: m.Subscription.ID, EventID: out.EventID}).String() {
		t.Errorf("match id %q is not the reaction id of the raised event", m.ID)
	}

	waitFor(t, "the plain entry at the point runs", func() bool { return acts.callsTo(afterCode) == 1 })
	var in map[string]any
	_ = json.Unmarshal(acts.inputOf(afterCode), &in)
	if in["event"] != "code.changed" || in["first"] != "go.mod" {
		t.Errorf("the entry's input = %v, want the event type and its first path", in)
	}
	if n := acts.callsTo(subCode); n != 0 {
		t.Errorf("the async entry ran %d time(s); CWM never runs it", n)
	}

	t.Run("under a reaction, staged in its outbox", func(t *testing.T) {
		reaction := api.ReactionID{Subscription: api.SubscriptionID("type:other", codePoint, "x:other"), EventID: "e-1"}.String()
		rctx := cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: reaction, Depth: 1})
		before := len(pendingOf(svc, subCodeCaused))
		if _, err := svc.EventRaise(rctx, api.EventRaiseInput{Type: "code.changed", ObjectID: "billing.code", Payload: paths("a.go")}); err != nil {
			t.Fatal(err)
		}
		var box *api.OutboxSummary
		for _, o := range svc.outbox().list() {
			if o.Reaction == reaction {
				o := o
				box = &o
			}
		}
		if box == nil || box.Matches != 1 || box.Writes != 1 {
			t.Fatalf("outbox = %+v, want the raised event with its one caused match", box)
		}
		if n := len(pendingOf(svc, subCodeCaused)); n != before {
			t.Errorf("a reaction's raised event was recorded at once (%d -> %d)", before, n)
		}
	})
	t.Run("at the depth limit, no match", func(t *testing.T) {
		deep := cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "h", Depth: api.ReactionDepthLimit})
		before := len(pendingOf(svc, subCodeCaused))
		if _, err := svc.EventRaise(deep, api.EventRaiseInput{Type: "code.changed", ObjectID: "billing.code", Payload: paths("go.mod")}); err != nil {
			t.Fatal(err)
		}
		if n := len(pendingOf(svc, subCodeCaused)); n != before {
			t.Errorf("an event raised at the depth limit matched (%d -> %d)", before, n)
		}
	})
}

// (2) A subscription's `when` reads the event's payload: only a change that
// touches go.mod matches it. An object write's event reads as `event` too.
func TestASubscriptionReadsTheRaisedEventsPayload(t *testing.T) {
	cfg := raiseConfig(t)
	acts := newInputActions()
	svc, ctx, _ := openAsync(t, cfg, acts)
	defer svc.Close()
	o := typed("billing.code", "code-module")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EventRaise(ctx, api.EventRaiseInput{Type: "code.changed", ObjectID: o.ID, Payload: paths("x.go")}); err != nil {
		t.Fatal(err)
	}
	if n := len(pendingOf(svc, subGoMod)); n != 0 {
		t.Fatalf("a change to x.go matched the go.mod subscription (%d)", n)
	}
	if n := len(pendingOf(svc, subCode)); n != 1 {
		t.Fatalf("the plain subscription: %d match(es), want 1", n)
	}
	if _, err := svc.EventRaise(ctx, api.EventRaiseInput{Type: "code.changed", ObjectID: o.ID, Payload: paths("go.mod")}); err != nil {
		t.Fatal(err)
	}
	if ms := pendingOf(svc, subGoMod); len(ms) != 1 || string(ms[0].Events[0].Payload) != `{"paths":["go.mod"]}` {
		t.Errorf("a change to go.mod: %+v, want one match carrying it", ms)
	}

	// Listed with no event, the entry is "evaluated at the event" — not an error.
	hooks, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: o.ID, Point: codePoint})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hooks.Hooks {
		if h.URI == subGoMod && (!h.Applies || !strings.Contains(h.Reason, "evaluated at the event")) {
			t.Errorf("listed = applies %v reason %q, want applying, evaluated at the event", h.Applies, h.Reason)
		}
	}

	// An object write: on_update's entry reads `event.type`.
	o.Description = "changed"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the on_update entry reading event.type runs", func() bool { return acts.callsTo(afterUpdated) == 1 })
	var call api.HookCall
	if err := json.Unmarshal(acts.inputOf(afterUpdated), &call); err != nil || call.Event == nil || call.Event.Event != "object.updated" || call.Event.Action != "updated" {
		t.Errorf("the hook call = %s, want it to carry the write's event", acts.inputOf(afterUpdated))
	}
}

// (3) An undeclared type is refused: nothing published, nothing matched. So
// are CWM's own types, a type with no point, an unknown object, a payload that
// is not JSON.
func TestAnUndeclaredEventTypeIsRefused(t *testing.T) {
	cfg := raiseConfig(t)
	cfg.CatalogTypes.EventTypes = append(cfg.CatalogTypes.EventTypes, api.EventType{Name: "code.pointless"})
	svc, ctx, bus := openAsync(t, cfg, newDurableActions())
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.code", "code-module")}); err != nil {
		t.Fatal(err)
	}
	var published captured
	_ = bus.Subscribe(api.ObjectEventRaisedSubject("t", "u", "w", api.SubjectToken("billing.code")), published.on)
	for _, in := range []api.EventRaiseInput{
		{Type: "code.unknown", ObjectID: "billing.code", Payload: paths("go.mod")},
		{Type: "object.updated", ObjectID: "billing.code"},
		{Type: "code.pointless", ObjectID: "billing.code"},
		{Type: "", ObjectID: "billing.code"},
		{Type: "code.changed", ObjectID: ""},
		{Type: "code.changed", ObjectID: "billing.code", Payload: json.RawMessage(`{not json`)},
	} {
		if _, err := svc.EventRaise(ctx, in); !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("raise %+v = %v, want ErrInvalidInput", in, err)
		}
	}
	if _, err := svc.EventRaise(ctx, api.EventRaiseInput{Type: "code.changed", ObjectID: "billing.nowhere"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("raise on an unknown object = %v, want not found", err)
	}
	if n := len(published.all()); n != 0 {
		t.Errorf("%d refused event(s) were published", n)
	}
	if n := len(svc.matches().pending()); n != 0 {
		t.Errorf("%d refused event(s) matched", n)
	}
}

// (4) A point only an event type names is a point: listed, after the event,
// not run on demand; a hook-type of the same name with another timing is a
// declaration finding.
func TestAPointNamedOnlyByAnEventTypeResolves(t *testing.T) {
	cfg := raiseConfig(t)
	svc, ctx, _ := openAsync(t, cfg, newDurableActions())
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.code", "code-module")}); err != nil {
		t.Fatal(err)
	}
	hooks, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "billing.code", Point: codePoint})
	if err != nil {
		t.Fatalf("a point named only by an event type: %v", err)
	}
	applying := 0
	for _, h := range hooks.Hooks {
		if h.Applies {
			applying++
		}
	}
	if applying != 4 {
		t.Errorf("%d entries apply at %s, want 4: %+v", applying, codePoint, hooks.Hooks)
	}
	if _, err := svc.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: codePoint, ID: "billing.code"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("run on demand = %v, want refused (it fires after the event)", err)
	}
	if f, _ := svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{}); countRule(f.Findings, api.FindingEventPoint) != 0 {
		t.Errorf("findings = %+v, want no event-point finding", f.Findings)
	}

	next := svc.Config.CatalogTypes
	next.HookTypes = []api.HookType{{Name: codePoint, Timing: api.HookTimingOnDemand, Answer: api.HookAnswerNone}}
	svc.RefreshCatalogTypes(next)
	f, _ := svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{})
	if countRule(f.Findings, api.FindingEventPoint) != 1 {
		t.Errorf("a hook-type on_code_change on demand: findings = %+v, want one event-point", f.Findings)
	}
}

func countRule(fs []api.DeclarationFinding, rule string) int {
	n := 0
	for _, f := range fs {
		if f.Rule == rule {
			n++
		}
	}
	return n
}
