// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CROSS-MODULE SUBSCRIPTIONS. A module subscribes to another
// module's PUBLISHED event types on its own module object (`on:`); in the
// emitter's objects' tables its entry is the farthest layer, no layer of the
// emitter can replace or remove it, and its reactions write only the modules
// it declares.

const (
	violationPoint = api.HookPoint("on_policy_violation")
	stockPoint     = api.HookPoint("on_stock")
	secReact       = "x:security/triage"
	secStock       = "x:security/stock"
	shopStock      = "x:shop/stock"
	secGhost       = "x:security/ghost"
)

// crossConfig: modules are `module` objects; `item` objects are what events
// are raised on. shop's policy.violated is published, policy.waived shares
// its point and is published too, stock.counted is private.
func crossConfig(t *testing.T) *CWMServiceConfig {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module", Capabilities: []string{"container"}},
		{Name: "item", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.EventTypes = []api.EventType{
		{Name: "policy.violated", Point: violationPoint, Published: true, Provider: "ktw-shop"},
		{Name: "policy.waived", Point: violationPoint, Published: true, Provider: "ktw-shop"},
		{Name: "stock.counted", Point: stockPoint, Provider: "ktw-shop"},
	}
	return cfg
}

func moduleObject(id string, hooks api.Hooks) api.Object {
	o := typed(id, "module")
	o.Hooks = hooks
	return o
}

// triage is security's subscription to shop's policy.violated, writing
// security and audit.
func triage() api.HookEntry {
	return api.HookEntry{URI: secReact, Async: &api.AsyncSpec{}, On: &api.EventSource{Module: "shop", Event: "policy.violated"},
		Writes: []string{"security", "audit"}}
}

// openCross opens the service with shop, security and shop.order written
// (no cause); security subscribes with triage.
func openCross(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	svc, ctx, _ := openAsync(t, crossConfig(t), newDurableActions())
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []struct {
		module string
		obj    api.Object
	}{
		{"shop", moduleObject("shop", nil)},
		{"security", moduleObject("security", api.Hooks{violationPoint: {triage()}})},
		{"shop", typed("shop.order", "item")},
		{"security", typed("security.desk", "item")},
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: o.module, Object: o.obj}); err != nil {
			t.Fatalf("create %s: %v", o.obj.ID, err)
		}
	}
	return svc, ctx
}

func raise(t *testing.T, svc *Service, ctx stdctx.Context, typ, id string) {
	t.Helper()
	if _, err := svc.EventRaise(ctx, api.EventRaiseInput{Type: typ, ObjectID: id}); err != nil {
		t.Fatalf("raise %s on %s: %v", typ, id, err)
	}
}

// setModuleHooks rewrites a module object's hooks (no cause).
func setModuleHooks(svc *Service, ctx stdctx.Context, id string, hooks api.Hooks) error {
	_, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: id, Object: moduleObject(id, hooks)})
	return err
}

// publish flips an event type's `published` in the declarations in force.
func publish(svc *Service, name string, published bool) {
	next := *svc.decl()
	next.EventTypes = slices.Clone(next.EventTypes)
	for i := range next.EventTypes {
		if next.EventTypes[i].Name == name {
			next.EventTypes[i].Published = published
		}
	}
	svc.decls.Store(&next)
}

// (1) security reacts to shop's published policy.violated raised on a shop
// object: one match, its subscriber security, its emitter shop, its writes
// the declared ones. Another type at the same point, and the same type raised
// in another module, match nothing.
func TestAModuleReactsToAnotherModulesPublishedEvent(t *testing.T) {
	svc, ctx := openCross(t)
	raise(t, svc, ctx, "policy.violated", "shop.order")
	ms := pendingOf(svc, secReact)
	if len(ms) != 1 {
		t.Fatalf("%d match(es) of security's subscription, want 1", len(ms))
	}
	m := ms[0]
	wantID := "module:security#on_policy_violation#" + secReact
	if m.Subscription.ID != wantID || m.Subscription.Source != "module:security" {
		t.Errorf("subscription = %s from %s, want %s from module:security", m.Subscription.ID, m.Subscription.Source, wantID)
	}
	if m.Subscription.Module != "security" || m.Module != "shop" || m.ObjectID != "shop.order" {
		t.Errorf("subscriber %q emitter %q object %q, want security shop shop.order", m.Subscription.Module, m.Module, m.ObjectID)
	}
	if !slices.Equal(m.Subscription.Writes, []string{"security", "audit"}) {
		t.Errorf("writes = %v, want [security audit]", m.Subscription.Writes)
	}
	if len(m.Events) != 1 || m.Events[0].Event != "policy.violated" {
		t.Errorf("events = %+v", m.Events)
	}

	// Counted as events: a second match on shop.order would coalesce into
	// the pending one.
	events := func() int {
		n := 0
		for _, m := range pendingOf(svc, secReact) {
			n += len(m.Events)
		}
		return n
	}
	raise(t, svc, ctx, "policy.waived", "shop.order") // the same point, another type
	if n := events(); n != 1 {
		t.Errorf("policy.waived matched the policy.violated subscription (%d events)", n)
	}
	raise(t, svc, ctx, "policy.violated", "security.desk") // the type, in another module
	if n := events(); n != 1 {
		t.Errorf("an event of security matched a subscription to shop (%d events)", n)
	}

	// The module object's own table does not hold its subscription to shop.
	own, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "security"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range own.Hooks {
		if h.URI == secReact {
			t.Errorf("security's own table lists its subscription to shop: %+v", h)
		}
	}
}

// (2) Another module's private event type is refused at the write; written
// while it was published and unpublished since, it is listed as not applying
// and matches nothing — published again, it matches.
func TestAPrivateEventTypeOfAnotherModuleIsNeverSubscribed(t *testing.T) {
	svc, ctx := openCross(t)
	stock := api.HookEntry{URI: secStock, Async: &api.AsyncSpec{}, On: &api.EventSource{Module: "shop", Event: "stock.counted"}}
	err := setModuleHooks(svc, ctx, "security", api.Hooks{violationPoint: {triage()}, stockPoint: {stock}})
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "stock.counted is not published") {
		t.Fatalf("subscribing to shop's private stock.counted: %v, want refused (not published)", err)
	}

	publish(svc, "stock.counted", true)
	if err := setModuleHooks(svc, ctx, "security", api.Hooks{violationPoint: {triage()}, stockPoint: {stock}}); err != nil {
		t.Fatalf("subscribing to a published type: %v", err)
	}
	publish(svc, "stock.counted", false)

	listed, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "shop.order", Point: stockPoint})
	if err != nil {
		t.Fatal(err)
	}
	var found *api.ResolvedHook
	for i := range listed.Hooks {
		if listed.Hooks[i].URI == secStock {
			found = &listed.Hooks[i]
		}
	}
	if found == nil || found.Applies || found.Reason != "event type stock.counted is not published" {
		t.Fatalf("listed = %+v, want security's entry not applying: not published", found)
	}
	raise(t, svc, ctx, "stock.counted", "shop.order")
	if n := len(pendingOf(svc, secStock)); n != 0 {
		t.Fatalf("a private type of shop matched security's subscription (%d)", n)
	}

	publish(svc, "stock.counted", true)
	raise(t, svc, ctx, "stock.counted", "shop.order")
	if n := len(pendingOf(svc, secStock)); n != 1 {
		t.Fatalf("published again: %d match(es), want 1", n)
	}

	// An update that leaves the entry as stored is not refused for it.
	publish(svc, "stock.counted", false)
	if err := setModuleHooks(svc, ctx, "security", api.Hooks{violationPoint: {triage()}, stockPoint: {stock}}); err != nil {
		t.Errorf("an update keeping a stored entry was refused: %v", err)
	}
}

// (3) A module subscribes to its own private event type; its reactions write
// its own module by default.
func TestAModuleSubscribesToItsOwnPrivateEventType(t *testing.T) {
	svc, ctx := openCross(t)
	own := api.HookEntry{URI: shopStock, Async: &api.AsyncSpec{}, On: &api.EventSource{Module: "shop", Event: "stock.counted"}}
	if err := setModuleHooks(svc, ctx, "shop", api.Hooks{stockPoint: {own}}); err != nil {
		t.Fatalf("shop subscribing to its own private type: %v", err)
	}
	raise(t, svc, ctx, "stock.counted", "shop.order")
	ms := pendingOf(svc, shopStock)
	if len(ms) != 1 {
		t.Fatalf("%d match(es), want 1", len(ms))
	}
	if s := ms[0].Subscription; s.Module != "shop" || !slices.Equal(s.Writes, []string{"shop"}) {
		t.Errorf("subscriber %q writes %v, want shop [shop]", s.Module, s.Writes)
	}
}

// (4) ModuleSubscribers lists who subscribes to a module: security's entry,
// applying, and one naming a type no pack declares — accepted at the write,
// listed with its reason.
func TestModuleSubscribersListsEachSubscriberAndWhyOneDoesNotApply(t *testing.T) {
	svc, ctx := openCross(t)
	ghost := api.HookEntry{URI: secGhost, Async: &api.AsyncSpec{Priority: api.PriorityP1}, On: &api.EventSource{Module: "shop", Event: "shop.ghost"}}
	if err := setModuleHooks(svc, ctx, "security", api.Hooks{violationPoint: {triage(), ghost}}); err != nil {
		t.Fatalf("an undeclared event type is accepted at the write: %v", err)
	}
	out, err := svc.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Subscribers) != 2 {
		t.Fatalf("subscribers = %+v, want 2", out.Subscribers)
	}
	got := map[string]api.ModuleSubscriber{}
	for _, s := range out.Subscribers {
		got[s.URI] = s
	}
	if s := got[secReact]; s.Subscriber != "security" || s.Event != "policy.violated" || s.Point != violationPoint || !s.Applies ||
		s.Subscription != "module:security#on_policy_violation#"+secReact || !slices.Equal(s.Writes, []string{"security", "audit"}) || s.Priority != api.PriorityP2 {
		t.Errorf("security's entry = %+v", s)
	}
	if s := got[secGhost]; s.Applies || !strings.Contains(s.Reason, "shop.ghost is not declared") || !slices.Equal(s.Writes, []string{"security"}) || s.Priority != api.PriorityP1 {
		t.Errorf("the undeclared type's entry = %+v, want listed not applying with its reason", s)
	}
	one, _ := svc.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "shop", Event: "policy.violated"})
	if len(one.Subscribers) != 1 || one.Subscribers[0].URI != secReact {
		t.Errorf("by event = %+v", one.Subscribers)
	}
	none, _ := svc.ModuleSubscribers(ctx, api.ModuleSubscribersInput{Module: "security"})
	if len(none.Subscribers) != 0 {
		t.Errorf("security has subscribers: %+v", none.Subscribers)
	}
}

// (5) A reaction of a cross-module subscription writes only the modules it
// declares — written or staged in its change set; an uncaused write, a hook's
// and a reaction of a module's own subscription are not concerned.
func TestACrossModuleReactionWritesOnlyTheModulesItDeclares(t *testing.T) {
	svc, ctx := openCross(t)
	reaction := api.ReactionID{Subscription: "module:security#on_policy_violation#" + secReact, EventID: "e-1"}.String()
	rctx := reactionCtx(ctx, reaction)

	_, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "shop", Object: typed("shop.fix", "item")})
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), `module "shop"`) ||
		!strings.Contains(err.Error(), "module:security#on_policy_violation#"+secReact) {
		t.Fatalf("a reaction writing undeclared shop: %v, want refused naming the module and the subscription", err)
	}
	order, _ := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "shop.order"})
	upd := order.Object
	upd.Description = "touched"
	if _, err := svc.ObjectUpdate(rctx, api.ObjectUpdateInput{Object: upd}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("a reaction updating shop.order: %v, want refused", err)
	}
	if _, err := svc.ObjectDelete(rctx, api.ObjectDeleteInput{ID: "shop.order"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("a reaction deleting shop.order: %v, want refused", err)
	}
	for _, m := range []string{"security", "audit"} {
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: m, Object: typed(m+".case", "item")}); err != nil {
			t.Errorf("a reaction writing declared %s: %v", m, err)
		}
	}
	// Not concerned: no cause, a hook's cause, a reaction of shop's own.
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "shop", Object: typed("shop.plain", "item")}); err != nil {
		t.Errorf("an uncaused write: %v", err)
	}
	hook := cause.WithCause(ctx, cause.Cause{Kind: cause.KindHook, ID: "h-1", Depth: 1})
	if _, err := svc.ObjectCreate(hook, api.ObjectCreateInput{Module: "shop", Object: typed("shop.hooked", "item")}); err != nil {
		t.Errorf("a hook's write: %v", err)
	}
	ownReaction := reactionCtx(ctx, api.ReactionID{Subscription: "type:item#on_stock#x:other", EventID: "e-2"}.String())
	if _, err := svc.ObjectCreate(ownReaction, api.ObjectCreateInput{Module: "billing", Object: typed("billing.any", "item")}); err != nil {
		t.Errorf("a reaction of a module's own subscription: %v", err)
	}

	t.Run("staged in its change set", func(t *testing.T) {
		if _, err := svc.ChangeSetBegin(ctx, api.ChangeSetBeginInput{Reaction: reaction}); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = svc.ChangeSetDiscard(ctx, api.ChangeSetDiscardInput{Reaction: reaction}) }()
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "shop", Object: typed("shop.staged", "item")}); !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("a staged write into shop: %v, want refused at the stage", err)
		}
		if _, err := svc.ObjectCreate(rctx, api.ObjectCreateInput{Module: "security", Object: typed("security.staged", "item")}); err != nil {
			t.Errorf("a staged write into security: %v", err)
		}
		list, _ := svc.ChangeSetList(ctx, api.ChangeSetListInput{})
		if len(list.Sets) != 1 || len(list.Sets[0].Writes) != 1 || list.Sets[0].Writes[0].ID != "security.staged" {
			t.Errorf("staged = %+v, want security.staged only", list.Sets)
		}
	})
}

// (6) The emitter cannot cancel another module's subscription: an entry of
// the same uri with `when: false` on the emitting object removes nothing.
func TestTheEmitterCannotRemoveASubscribersEntry(t *testing.T) {
	svc, ctx := openCross(t)
	order, _ := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "shop.order"})
	o := order.Object
	o.Hooks = api.Hooks{violationPoint: {{URI: secReact, When: "false"}}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Object: o}); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "shop.order", Point: violationPoint})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Hooks) != 1 || listed.Hooks[0].Source != "module:security" || !listed.Hooks[0].Applies {
		t.Fatalf("listed = %+v, want security's entry, applying", listed.Hooks)
	}
	raise(t, svc, ctx, "policy.violated", "shop.order")
	if n := len(pendingOf(svc, secReact)); n != 1 {
		t.Fatalf("%d match(es), want 1: the emitter's when: false removed the subscriber's entry", n)
	}
}

// (7) Where a cross-module entry may stand: only on a module object, async,
// at the point its type names, once per point and uri.
func TestACrossModuleEntryIsRefusedWhereItCannotStand(t *testing.T) {
	svc, ctx := openCross(t)
	cases := map[string]struct {
		id    string
		hooks api.Hooks
		want  string
	}{
		"not a module object": {"security.desk", api.Hooks{violationPoint: {triage()}}, "only a module object"},
		"not async": {"security", api.Hooks{violationPoint: {{URI: secReact, On: &api.EventSource{Module: "shop", Event: "policy.violated"}}}},
			"must be async"},
		"not its type's point": {"security", api.Hooks{stockPoint: {triage()}}, "is at on_policy_violation"},
		"twice":                {"security", api.Hooks{violationPoint: {triage(), triage()}}, "one cross-module entry per point and uri"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			obj := moduleObject(c.id, c.hooks)
			if c.id != "security" {
				obj.Type = "item"
			}
			_, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Object: obj})
			if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want refused: %s", err, c.want)
			}
		})
	}
}
