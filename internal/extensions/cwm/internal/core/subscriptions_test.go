// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// ASYNCHRONOUS SUBSCRIPTIONS. An on_* entry that declares `async` is matched at
// the write and handed to a MatchTaker — never run by CWM — under the cascade
// guards; a reaction's writes stage what they trigger in its outbox until the
// collector releases it.

const (
	subCreate      = "x:react/create"
	subUpdate      = "x:react/update"
	subChainCreate = "x:react/chain-create"
	subChain       = "x:react/chain"
	afterChain     = "x:after/chain"
)

// asyncConfig: `watched` objects subscribe (async, default spec) on create and
// update; `chained` ones accept caused writes and run a plain on_update hook.
func asyncConfig(t *testing.T) *CWMServiceConfig {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	caused := &api.AsyncSpec{AcceptsCaused: true}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "watched", Capabilities: []string{"container"}, Hooks: api.Hooks{
			api.HookOnCreate: {{URI: subCreate, Async: &api.AsyncSpec{}}},
			api.HookOnUpdate: {{URI: subUpdate, Async: &api.AsyncSpec{}}},
		}},
		{Name: "chained", Capabilities: []string{"container"}, Hooks: api.Hooks{
			api.HookOnCreate: {{URI: subChainCreate, Async: caused}},
			api.HookOnUpdate: {{URI: subChain, Async: caused}, {URI: afterChain}},
		}},
	}
	return cfg
}

// takerFake takes matches; it refuses the first `fail` it is handed.
type takerFake struct {
	mu    sync.Mutex
	fail  int
	calls int
	got   []api.Match
}

func (f *takerFake) taker() api.MatchTaker {
	return api.MatchTaker{Name: "fake", Take: func(_ stdctx.Context, m api.Match) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if f.calls <= f.fail {
			return errors.New("the queue is not there yet")
		}
		f.got = append(f.got, m)
		return nil
	}}
}

func (f *takerFake) taken() []api.Match {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.Match(nil), f.got...)
}

func (f *takerFake) takenOf(uri string) []api.Match {
	var out []api.Match
	for _, m := range f.taken() {
		if m.Subscription.URI == uri {
			out = append(out, m)
		}
	}
	return out
}

// openAsync opens the service with the actions given, its object events on a
// memory bus the test can watch.
func openAsync(t *testing.T, cfg *CWMServiceConfig, actions actionsapi.ActionService) (*Service, stdctx.Context, *eventbus.MemoryEventBus) {
	t.Helper()
	svc, ctx := openDurable(t, cfg, actions)
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	return svc, ctx, bus
}

func typed(id, typ string) api.Object {
	o := api.Object{ID: id, Type: typ}
	o.Name = id
	return o
}

func pendingOf(svc *Service, uri string) []api.Match {
	var out []api.Match
	for _, m := range svc.matches().pending() {
		if m.Subscription.URI == uri {
			out = append(out, m)
		}
	}
	return out
}

func matchFiles(t *testing.T, cfg *CWMServiceConfig) int {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(storage.NewPaths(cfg.WorkspaceRoot).CWMDataDir(), "matches"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// (1) An async hook matched by a write is handed over — never run — with the
// write's event (the one published), its cause and its depth.
func TestAnAsyncHookMatchedByAWriteIsHandedOverWithItsCauseAndDepth(t *testing.T) {
	cfg := asyncConfig(t)
	acts := newDurableActions()
	svc, ctx, bus := openAsync(t, cfg, acts)
	defer svc.Close()
	fake := &takerFake{}
	svc.AttachMatchTakers(ctx, []api.MatchTaker{fake.taker()})

	var published captured
	_ = bus.Subscribe(api.ObjectCreatedSubject("t", "u", "w", api.SubjectToken("billing.w")), published.on)
	person := cause.Cause{Kind: cause.KindPerson, ID: "cs"}
	if _, err := svc.ObjectCreate(cause.WithCause(ctx, person), api.ObjectCreateInput{Module: "billing", Object: typed("billing.w", "watched")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFor(t, "the match is taken", func() bool { return len(fake.takenOf(subCreate)) == 1 })
	m := fake.takenOf(subCreate)[0]
	ev := published.all()[0].(events.ObjectPayload)
	wantSub := api.SubscriptionID("type:watched", api.HookOnCreate, subCreate)
	if m.Subscription.ID != wantSub || m.Subscription.Priority != api.PriorityP2 || m.Subscription.Attempts != 3 {
		t.Errorf("subscription = %+v, want %s P2 x3", m.Subscription, wantSub)
	}
	if m.ID != (api.ReactionID{Subscription: wantSub, EventID: ev.EventID}).String() {
		t.Errorf("match id %q is not the reaction id of the published event %s", m.ID, ev.EventID)
	}
	if len(m.Events) != 1 || m.Events[0].EventID != ev.EventID || m.Events[0].Action != "created" || m.Events[0].ObjectID != "billing.w" {
		t.Errorf("events = %+v, want the one published (%s)", m.Events, ev.EventID)
	}
	if m.Depth != 0 || m.Cause == nil || *m.Cause != person || m.Module != "billing" || m.Key != "billing.w" {
		t.Errorf("match = depth %d cause %v module %q key %q, want 0 %v billing billing.w", m.Depth, m.Cause, m.Module, m.Key, person)
	}
	if rc := api.ReactionCause(m); rc != (cause.Cause{Kind: cause.KindReaction, ID: m.ID, Depth: 1}) {
		t.Errorf("reaction cause = %v", rc)
	}
	if n := acts.callsTo(subCreate); n != 0 {
		t.Errorf("the async entry ran %d time(s) through the action service; CWM never runs it", n)
	}
	waitFor(t, "a taken match leaves the store", func() bool { return len(svc.matches().pending()) == 0 && matchFiles(t, cfg) == 0 })

	// A write one level deep (a hook's) is matched with its depth and cause by
	// a subscription that accepts caused writes.
	hook := cause.Cause{Kind: cause.KindHook, ID: "h-1", Depth: 1}
	if _, err := svc.ObjectCreate(cause.WithCause(ctx, hook), api.ObjectCreateInput{Module: "billing", Object: typed("billing.c", "chained")}); err != nil {
		t.Fatalf("create chained: %v", err)
	}
	waitFor(t, "the caused match is taken", func() bool { return len(fake.takenOf(subChainCreate)) == 1 })
	if c := fake.takenOf(subChainCreate)[0]; c.Depth != 1 || c.Cause == nil || *c.Cause != hook {
		t.Errorf("caused match = depth %d cause %v, want 1 %v", c.Depth, c.Cause, hook)
	}
}

// (2) The guards refuse a cascade: the depth limit, no self-reaction, and a
// caused write matching only what accepts caused writes.
func TestTheCascadeGuardsRefuseAMatch(t *testing.T) {
	t.Run("depth limit", func(t *testing.T) {
		cfg := asyncConfig(t)
		svc, ctx, _ := openAsync(t, cfg, newDurableActions())
		defer svc.Close()
		below := cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "h", Depth: api.ReactionDepthLimit - 1})
		if _, err := svc.ObjectCreate(below, api.ObjectCreateInput{Module: "billing", Object: typed("billing.below", "chained")}); err != nil {
			t.Fatal(err)
		}
		if n := len(pendingOf(svc, subChainCreate)); n != 1 {
			t.Fatalf("a write below the limit: %d match(es), want 1", n)
		}
		at := cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "h", Depth: api.ReactionDepthLimit})
		if _, err := svc.ObjectCreate(at, api.ObjectCreateInput{Module: "billing", Object: typed("billing.at", "chained")}); err != nil {
			t.Fatal(err)
		}
		if n := len(pendingOf(svc, subChainCreate)); n != 1 {
			t.Errorf("a write at the depth limit matched (%d matches): the cascade goes on", n)
		}
	})
	t.Run("no self-reaction", func(t *testing.T) {
		cfg := asyncConfig(t)
		svc, ctx, _ := openAsync(t, cfg, newDurableActions())
		defer svc.Close()
		self := api.SubscriptionID("type:chained", api.HookOnCreate, subChainCreate)
		own := api.ReactionID{Subscription: self, EventID: "e-1"}.String()
		other := api.ReactionID{Subscription: api.SubscriptionID("type:watched", api.HookOnCreate, subCreate), EventID: "e-2"}.String()
		for _, r := range []struct{ reaction, id string }{{own, "billing.own"}, {other, "billing.other"}} {
			c := cause.Cause{Kind: cause.KindReaction, ID: r.reaction, Depth: 1}
			if _, err := svc.ObjectCreate(cause.WithCause(ctx, c), api.ObjectCreateInput{Module: "billing", Object: typed(r.id, "chained")}); err != nil {
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
			t.Errorf("a reaction's write matched its own subscription (%d staged): it reacts to itself", staged[own])
		}
	})
	t.Run("accepts caused", func(t *testing.T) {
		cfg := asyncConfig(t)
		svc, ctx, _ := openAsync(t, cfg, newDurableActions())
		defer svc.Close()
		hook := cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "h", Depth: 1})
		if _, err := svc.ObjectCreate(hook, api.ObjectCreateInput{Module: "billing", Object: typed("billing.caused", "watched")}); err != nil {
			t.Fatal(err)
		}
		if n := len(pendingOf(svc, subCreate)); n != 0 {
			t.Errorf("a caused write matched a subscription that does not accept caused writes (%d)", n)
		}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.plain", "watched")}); err != nil {
			t.Fatal(err)
		}
		if n := len(pendingOf(svc, subCreate)); n != 1 {
			t.Errorf("an uncaused write: %d match(es), want 1", n)
		}
	})
}

// (3) Two writes on one object, no taker: one pending match carrying both
// events — and it survives a restart, handed over when a taker attaches.
func TestTwoWritesOnOneObjectCoalesceIntoOnePendingMatch(t *testing.T) {
	cfg := asyncConfig(t)
	svc1, ctx1, _ := openAsync(t, cfg, newDurableActions())
	o := typed("billing.w", "watched")
	if _, err := svc1.ObjectCreate(ctx1, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		o.Description = strings.Repeat("x", i+1)
		if _, err := svc1.ObjectUpdate(ctx1, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	ups := pendingOf(svc1, subUpdate)
	if len(ups) != 1 {
		t.Fatalf("two updates made %d pending matches, want 1 (coalesced)", len(ups))
	}
	if ev := ups[0].Events; len(ev) != 2 || ev[0].EventID == ev[1].EventID || ev[0].Action != "updated" {
		t.Errorf("the coalesced match carries %+v, want both updates' events", ev)
	}
	if ups[0].Cause != nil {
		t.Errorf("an uncaused write's match carries a cause: %v", ups[0].Cause)
	}
	if n := matchFiles(t, cfg); n != 2 {
		t.Errorf("%d match files on disk, want 2 (create, update)", n)
	}
	_ = svc1.Close() // the process stops with the matches pending

	svc2, ctx2, _ := openAsync(t, cfg, newDurableActions())
	defer svc2.Close()
	fake := &takerFake{fail: 1} // the first handover fails: retried
	svc2.AttachMatchTakers(ctx2, []api.MatchTaker{fake.taker()})
	waitFor(t, "the pending matches are handed over after the restart", func() bool { return len(fake.taken()) == 2 })
	if got := fake.takenOf(subUpdate); len(got) != 1 || len(got[0].Events) != 2 {
		t.Errorf("replayed update match = %+v", got)
	}
	waitFor(t, "taken matches leave the disk", func() bool { return matchFiles(t, cfg) == 0 })
}

// (4) A reaction's writes stage what they trigger: the collector that does not
// release (two failed attempts) hands nothing on, while the object events go
// out at once; the release hands the matches and hook calls on, once.
func TestTheOutboxReleasesAReactionsMatchesOnlyOnRelease(t *testing.T) {
	cfg := asyncConfig(t)
	acts := newDurableActions()
	svc, ctx, bus := openAsync(t, cfg, acts)
	defer svc.Close()
	fake := &takerFake{}
	svc.AttachMatchTakers(ctx, []api.MatchTaker{fake.taker()})

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.src", "watched")}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the triggering match is taken", func() bool { return len(fake.takenOf(subCreate)) == 1 })
	trigger := fake.takenOf(subCreate)[0]
	target := typed("billing.tgt", "chained")
	if _, err := svc.ObjectCreate(cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "setup", Depth: api.ReactionDepthLimit}),
		api.ObjectCreateInput{Module: "billing", Object: target}); err != nil {
		t.Fatal(err)
	}
	var updated captured
	_ = bus.Subscribe(api.ObjectUpdatedSubject("t", "u", "w", api.SubjectToken(target.ID)), updated.on)

	// The reaction runs under its cause; a fake collector releases only after
	// the third attempt.
	rctx := cause.WithCause(ctx, api.ReactionCause(trigger))
	for attempt := 1; attempt <= 3; attempt++ {
		target.Description = strings.Repeat("y", attempt)
		if _, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Module: "billing", Object: target}); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if attempt < 3 {
			continue // failed: the collector does not release
		}
		time.Sleep(20 * time.Millisecond) // what a premature handover would need to land
		if n := len(fake.takenOf(subChain)); n != 0 {
			t.Fatalf("before the release the taker saw %d match(es) of the reaction's writes", n)
		}
		if n := acts.callsTo(afterChain); n != 0 {
			t.Fatalf("before the release the on_update hook ran %d time(s)", n)
		}
		boxes := svc.outbox().list()
		if len(boxes) != 1 || boxes[0].Reaction != trigger.ID || boxes[0].Writes != 3 || boxes[0].Matches != 1 || boxes[0].Hooks != 1 {
			t.Fatalf("outbox = %+v, want the reaction's 3 writes with 1 match and 1 hook call (coalesced)", boxes)
		}
		out, err := svc.OutboxRelease(ctx, api.OutboxReleaseInput{Reaction: trigger.ID})
		if err != nil || out.Matches != 1 || out.Hooks != 1 {
			t.Fatalf("release = %+v, %v", out, err)
		}
	}
	if n := len(updated.all()); n != 3 {
		t.Errorf("%d updated events went out, want 3 at once (one per write, never staged)", n)
	}
	waitFor(t, "the released match is taken", func() bool { return len(fake.takenOf(subChain)) == 1 })
	m := fake.takenOf(subChain)[0]
	if m.Depth != 1 || m.Cause == nil || m.Cause.Kind != cause.KindReaction || m.Cause.ID != trigger.ID || len(m.Events) != 3 {
		t.Errorf("released match = depth %d cause %v with %d events, want depth 1 under the reaction with 3", m.Depth, m.Cause, len(m.Events))
	}
	waitFor(t, "the released hook call runs", func() bool { return acts.callsTo(afterChain) == 1 })
	if c := acts.causeOf(afterChain); c.Kind != cause.KindHook || c.Depth != 2 {
		t.Errorf("the released hook ran under %v, want a hook cause at depth 2", c)
	}
	if boxes := svc.outbox().list(); len(boxes) != 0 {
		t.Errorf("outbox after release = %+v", boxes)
	}
	time.Sleep(20 * time.Millisecond)
	if n := len(fake.takenOf(subChain)); n != 1 {
		t.Errorf("the taker saw the released match %d times, want once", n)
	}
}

// A reaction given up drops its outbox: nothing it would have triggered is
// handed on, and its writes are listed.
func TestADroppedOutboxHandsNothingOnAndListsTheWrites(t *testing.T) {
	cfg := asyncConfig(t)
	acts := newDurableActions()
	svc, ctx, _ := openAsync(t, cfg, acts)
	defer svc.Close()
	fake := &takerFake{}
	svc.AttachMatchTakers(ctx, []api.MatchTaker{fake.taker()})
	reaction := api.ReactionID{Subscription: api.SubscriptionID("type:watched", api.HookOnCreate, subCreate), EventID: "e-9"}.String()
	rctx := cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: reaction, Depth: 1})
	if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.t", "chained")}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.OutboxDrop(ctx, api.OutboxDropInput{Reaction: reaction})
	if err != nil || len(out.Writes) != 1 || out.Writes[0].ObjectID != "billing.t" || out.Writes[0].Action != "created" {
		t.Fatalf("drop = %+v, %v", out, err)
	}
	if rel, _ := svc.OutboxRelease(ctx, api.OutboxReleaseInput{Reaction: reaction}); rel.Matches != 0 {
		t.Errorf("a dropped outbox still released %d match(es)", rel.Matches)
	}
	time.Sleep(20 * time.Millisecond)
	if n := len(fake.taken()); n != 0 {
		t.Errorf("a dropped reaction handed %d match(es) on", n)
	}
}

// An async entry at a point no event type names never applies, and is a
// declaration finding.
func TestAnAsyncEntryAtAPointNoEventTypeNamesNeverApplies(t *testing.T) {
	cfg := asyncConfig(t)
	cfg.CatalogTypes.Objects = append(cfg.CatalogTypes.Objects, api.ObjectType{Name: "odd", Capabilities: []string{"container"}, Hooks: api.Hooks{
		api.HookLint:                    {{URI: "x:lint/async", Async: &api.AsyncSpec{}}},
		api.HookPoint("on_code_change"): {{URI: "x:react/code", Async: &api.AsyncSpec{}}},
	}})
	svc, ctx, _ := openAsync(t, cfg, newDurableActions())
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.odd", "odd")}); err != nil {
		t.Fatal(err)
	}
	applies := func() map[string]api.ResolvedHook {
		out, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "billing.odd"})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]api.ResolvedHook{}
		for _, h := range out.Hooks {
			m[h.URI] = h
		}
		return m
	}
	h := applies()
	if h["x:lint/async"].Applies || h["x:react/code"].Applies {
		t.Errorf("async entries at points no event type names apply: %+v", h)
	}
	findings, _ := svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{})
	n := 0
	for _, f := range findings.Findings {
		if f.Rule == api.FindingAsyncPoint {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d async-point findings, want 2", n)
	}

	// Another pack's event type naming the point makes it a point — no
	// hook-type declares it — and subscribable.
	next := svc.Config.CatalogTypes
	next.EventTypes = []api.EventType{{Name: "code.changed", Point: "on_code_change"}}
	svc.RefreshCatalogTypes(next)
	h = applies()
	if r := h["x:react/code"]; !r.Applies || r.Async == nil || r.Async.Key != api.DefaultAsyncKey ||
		r.Subscription != api.SubscriptionID("type:odd", "on_code_change", "x:react/code") {
		t.Errorf("once an event type names on_code_change: %+v", r)
	}
}

// AN OBJECT'S OWN SUBSCRIPTION IS ITS OWN: two objects declaring the same
// action at the same point are two subscriptions — the id names the object.
// A write on the second, made by a reaction of the first's subscription, is
// staged for the second (not refused as a self-reaction); found by the
// reactions scenario case.
func TestTwoObjectsOwnSubscriptionsAreDistinct(t *testing.T) {
	cfg := asyncConfig(t)
	cfg.CatalogTypes.Objects = append(cfg.CatalogTypes.Objects, api.ObjectType{Name: "plain", Capabilities: []string{"container"}})
	svc, ctx, _ := openAsync(t, cfg, newDurableActions())
	defer svc.Close()
	own := func(id string) api.Object {
		o := typed(id, "plain")
		o.Hooks = api.Hooks{api.HookOnCreate: {{URI: subCreate, Async: &api.AsyncSpec{AcceptsCaused: true}}}}
		return o
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: own("billing.first")}); err != nil {
		t.Fatal(err)
	}
	first := pendingOf(svc, subCreate)
	if len(first) != 1 {
		t.Fatalf("the first object's own subscription: %d match(es), want 1", len(first))
	}
	c := cause.Cause{Kind: cause.KindReaction, ID: first[0].ID, Depth: 1}
	if _, err := svc.ObjectCreate(cause.WithCause(ctx, c), api.ObjectCreateInput{Module: "billing", Object: own("billing.second")}); err != nil {
		t.Fatal(err)
	}
	staged := 0
	for _, o := range svc.outbox().list() {
		if o.Reaction == first[0].ID {
			staged = o.Matches
		}
	}
	if staged != 1 {
		t.Fatalf("the second object's own subscription staged %d match(es) under the first's reaction, want 1 — taken for a self-reaction", staged)
	}
}

// A module's own subscription names the module written as its subscriber and
// declares no writes: its reactions are not held to a list of modules (only a
// cross-module subscription's are, module_subscriptions_test.go).
func TestAModulesOwnSubscriptionIsItsReflexWithNoDeclaredWrites(t *testing.T) {
	cfg := asyncConfig(t)
	svc, ctx, _ := openAsync(t, cfg, newDurableActions())
	defer svc.Close()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: typed("billing.w", "watched")}); err != nil {
		t.Fatal(err)
	}
	ms := pendingOf(svc, subCreate)
	if len(ms) != 1 {
		t.Fatalf("%d match(es), want 1", len(ms))
	}
	if s := ms[0].Subscription; s.Module != "billing" || len(s.Writes) != 0 || ms[0].Module != "billing" {
		t.Errorf("subscriber %q writes %v emitter %q, want billing, none, billing", s.Module, s.Writes, ms[0].Module)
	}
	// Its reaction writes another module: not a cross-module subscription, not held.
	rctx := cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: ms[0].ID, Depth: 1})
	if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "shipping", Object: typed("shipping.x", "watched")}); err != nil {
		t.Errorf("a reaction of a module's own subscription writing another module: %v", err)
	}
}
