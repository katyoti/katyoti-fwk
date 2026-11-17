// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE SUBSCRIPTIONS TO AN EVENT TYPE. EventSubscriptions lists
// with no object, every async entry at an event type's point across the
// layers — module, hook, capability, policy, type, object — farthest first,
// with what is decided statically decided and the rest said.

const (
	subHook   = "x:hook/watch"
	subCap    = "x:cap/react"
	subPolicy = "x:policy/audit"
	subIdle   = "x:policy/idle"
	subType   = "x:type/react"
	subSync   = "x:type/sync"
	subOwn    = "x:object/own"
	subLater  = "x:object/later"
)

// subscriptionsConfig is crossConfig with an async entry at
// on_policy_violation in every static layer: a hook contribution, the
// container capability, a policy whose `when` reads the event, a policy that
// applies to no object, and the item type (a `when` over the object, and a
// sync entry that is no subscription).
func subscriptionsConfig(t *testing.T) *CWMServiceConfig {
	cfg := crossConfig(t)
	async := &api.AsyncSpec{}
	cfg.CatalogTypes.Hooks = []api.Hook{{Name: "watch", Type: violationPoint, URI: subHook, Async: async, Provider: "ktw-watch"}}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container", Provider: "ktw-core",
		Hooks: api.Hooks{violationPoint: {{URI: subCap, Async: &api.AsyncSpec{Priority: "P1"}}}}}}
	cfg.CatalogTypes.Policies = []api.Policy{
		{Name: "audit", Provider: "ktw-audit", Applies: api.ShapeFilter{Types: []string{"*"}},
			Hooks: api.Hooks{violationPoint: {{URI: subPolicy, Async: async, When: "event.payload.severity == 'high'"}}}},
		{Name: "idle", Provider: "ktw-audit",
			Hooks: api.Hooks{violationPoint: {{URI: subIdle, Async: async}}}},
	}
	for i := range cfg.CatalogTypes.Objects {
		if cfg.CatalogTypes.Objects[i].Name == "item" {
			cfg.CatalogTypes.Objects[i].Provider = "ktw-shop"
			cfg.CatalogTypes.Objects[i].Hooks = api.Hooks{violationPoint: {
				{URI: subType, Async: async, When: "type == 'item'"},
				{URI: subSync},
			}}
		}
	}
	return cfg
}

// openSubscriptions writes shop, security (subscribed to shop's published
// policy.violated and — while it was published — to its stock.counted, then
// unpublished) and shop.order, which declares its own subscription.
func openSubscriptions(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	svc, ctx, _ := openAsync(t, subscriptionsConfig(t), newDurableActions())
	t.Cleanup(func() { _ = svc.Close() })
	publish(svc, "stock.counted", true)
	stock := api.HookEntry{URI: secStock, Async: &api.AsyncSpec{}, On: &api.EventSource{Module: "shop", Event: "stock.counted"}}
	order := typed("shop.order", "item")
	order.Hooks = api.Hooks{violationPoint: {{URI: subOwn, Async: &api.AsyncSpec{}}}}
	for _, o := range []struct {
		module string
		obj    api.Object
	}{
		{"shop", moduleObject("shop", nil)},
		{"security", moduleObject("security", api.Hooks{violationPoint: {triage()}, stockPoint: {stock}})},
		{"shop", order},
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: o.module, Object: o.obj}); err != nil {
			t.Fatalf("create %s: %v", o.obj.ID, err)
		}
	}
	publish(svc, "stock.counted", false)
	return svc, ctx
}

func subscriptionsOf(t *testing.T, svc *Service, ctx stdctx.Context, event string) api.EventTypeSubscriptions {
	t.Helper()
	out, err := svc.EventSubscriptions(ctx, api.EventSubscriptionsInput{Event: event})
	if err != nil {
		t.Fatalf("EventSubscriptions %s: %v", event, err)
	}
	if len(out.Events) != 1 || out.Events[0].Event != event {
		t.Fatalf("EventSubscriptions %s = %+v, want that one type", event, out.Events)
	}
	return out.Events[0]
}

// Every layer's subscription is listed once, farthest first, with its source,
// its id, whether it applies and why.
func TestEventSubscriptionsListEveryLayer(t *testing.T) {
	svc, ctx := openSubscriptions(t)
	shared := "fires for every event at " + string(violationPoint)
	want := []struct {
		source, declaredOn, uri string
		applies                 bool
		reason                  string
	}{
		{"module:security", "security", secReact, true, ""},
		{"hook:ktw-watch/watch", "watch", subHook, true, shared},
		{"capability:container", "container", subCap, true, shared},
		{"policy:audit", "audit", subPolicy, true, "evaluated at the event; " + shared},
		{"policy:idle", "idle", subIdle, false, "applies to no object (applies is empty)"},
		{"type:item", "item", subType, true, "decided per object: when type == 'item'; " + shared},
		{"object:shop.order", "shop.order", subOwn, true, shared},
	}
	got := subscriptionsOf(t, svc, ctx, "policy.violated")
	if got.Point != violationPoint || !got.Published || got.Provider != "ktw-shop" {
		t.Errorf("type = %s %v %s, want %s published ktw-shop", got.Point, got.Published, got.Provider, violationPoint)
	}
	if len(got.Subscriptions) != len(want) {
		for _, s := range got.Subscriptions {
			t.Logf("listed %s", s.Subscription)
		}
		t.Fatalf("%d subscriptions, want %d", len(got.Subscriptions), len(want))
	}
	for i, w := range want {
		s := got.Subscriptions[i]
		id := api.SubscriptionID(w.source, violationPoint, w.uri)
		if s.Source != w.source || s.Subscription != id || s.DeclaredOn != w.declaredOn || s.URI != w.uri {
			t.Errorf("#%d = %s from %s on %s, want %s from %s on %s", i, s.Subscription, s.Source, s.DeclaredOn, id, w.source, w.declaredOn)
		}
		if s.Applies != w.applies || s.Reason != w.reason {
			t.Errorf("%s: applies %v %q, want %v %q", id, s.Applies, s.Reason, w.applies, w.reason)
		}
		if s.Async.Priority == "" || s.Async.Attempts == 0 {
			t.Errorf("%s: async %+v, want its defaults filled", id, s.Async)
		}
	}
	mod := got.Subscriptions[0]
	if mod.On == nil || mod.On.Module != "shop" || !slices.Equal(mod.Writes, []string{"security", "audit"}) {
		t.Errorf("module subscription on %+v writes %v, want shop policy.violated writing [security audit]", mod.On, mod.Writes)
	}
	if got.Subscriptions[2].Async.Priority != "P1" || got.Subscriptions[2].Provider != "ktw-core" {
		t.Errorf("capability subscription = %+v, want P1 from ktw-core", got.Subscriptions[2])
	}

	// A cross-module entry on another module's private type: listed, never
	// applying, its reason the refusal.
	stock := subscriptionsOf(t, svc, ctx, "stock.counted")
	if len(stock.Subscriptions) != 1 {
		t.Fatalf("stock.counted: %+v, want security's refused entry", stock.Subscriptions)
	}
	if s := stock.Subscriptions[0]; s.Source != "module:security" || s.Applies || s.Reason != "event type stock.counted is not published" {
		t.Errorf("stock.counted: %s applies %v %q, want refused: not published", s.Subscription, s.Applies, s.Reason)
	}

	// Every declared type: CWM's own first; one with no point is not
	// subscribable.
	all, err := svc.EventSubscriptions(ctx, api.EventSubscriptionsInput{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range all.Events {
		names = append(names, e.Event)
		if e.Event == "object.moved" && (e.Reason != subReasonNoPoint || len(e.Subscriptions) != 0) {
			t.Errorf("object.moved = %+v, want not subscribable", e)
		}
	}
	if len(names) < 3 || names[0] != "object.created" || !slices.Contains(names, "policy.waived") {
		t.Errorf("every type = %v, want CWM's first then the declared ones", names)
	}

	if _, err := svc.EventSubscriptions(ctx, api.EventSubscriptionsInput{Event: "no.such"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("an undeclared type: %v, want ErrInvalidInput", err)
	}
}

// An object written after a first listing is listed (the index keeps the
// objects carrying hooks on every write), and leaves the list when its hooks
// go.
func TestEventSubscriptionsFollowTheWrites(t *testing.T) {
	svc, ctx := openSubscriptions(t)
	has := func(uri string) bool {
		for _, s := range subscriptionsOf(t, svc, ctx, "policy.violated").Subscriptions {
			if s.URI == uri {
				return true
			}
		}
		return false
	}
	if has(subLater) {
		t.Fatal("listed before it was written")
	}
	later := typed("shop.later", "item")
	later.Hooks = api.Hooks{violationPoint: {{URI: subLater, Async: &api.AsyncSpec{}}}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "shop", Object: later}); err != nil {
		t.Fatal(err)
	}
	if !has(subLater) {
		t.Error("an object created after a listing is not listed")
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "shop", Object: typed("shop.later", "item")}); err != nil {
		t.Fatal(err)
	}
	if has(subLater) {
		t.Error("an object whose hooks went is still listed")
	}
}
