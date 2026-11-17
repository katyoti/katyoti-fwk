// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventslocal "github.com/katyoti/katyoti-fwk/internal/extensions/events/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A CROSS-MODULE REACTION: module security
// subscribes, from its module object, to shop's PUBLISHED event
// policy.violated; the reaction is security's work — a to-do in security's
// queue, reading shop and security, writing only what security declared — and
// a write outside it is refused by CWM.

var crossPack = fstest.MapFS{
	"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")},
	"object-types/node.yaml":   {Data: []byte("name: node\n")},
	"event-types/policy.violated.yaml": {Data: []byte(`name: policy.violated
label: Policy violated
point: on_policy_violation
published: true
`)},
}

// crossActions writes a node into the module `into` under the reaction's cause.
type crossActions struct {
	mu   sync.Mutex
	into string
}

func (a *crossActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a *crossActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	into := a.into
	a.mu.Unlock()
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	o := cwmapi.NewContainerObject(into, "finding", "node", into, "Finding", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: into, Object: o}); err != nil {
		return actionsapi.RunOutput{}, err
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{}`)}, nil
}

func newCrossWorld(t *testing.T, into string) (context.Context, cwmapi.CWMService, schedapi.TodoService, eventsapi.ReactionService) {
	t.Helper()
	acts := &crossActions{into: into}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(crossPack),
		schedlocal.Extension(),
		eventslocal.ReactionsExtensionWith(eventslocal.ReactionsOptions{BackoffSeconds: 1}),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "shop")
	cwmapp.Module(t, ctx, cwm, "security")
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "security", ID: "security"})
	if err != nil {
		t.Fatal(err)
	}
	sec := got.Object
	sec.Hooks = cwmapi.Hooks{"on_policy_violation": {{URI: "x:security/triage", Async: &cwmapi.AsyncSpec{},
		On: &cwmapi.EventSource{Module: "shop", Event: "policy.violated"}, Writes: []string{"security"}}}}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "security", Object: sec}); err != nil {
		t.Fatal(err)
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	react, err := application.UseService[eventsapi.ReactionService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, todos, react
}

// waitReaction answers the one reaction to-do of x:security/triage, wherever
// it was put.
func waitReaction(t *testing.T, ctx context.Context, todos schedapi.TodoService) schedapi.Todo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		list, err := todos.List(ctx, schedapi.ListInput{})
		if err != nil {
			t.Fatal(err)
		}
		for _, td := range list.Todos {
			if td.Owner == eventsapi.Owner && td.Work[eventsapi.WorkURI] == "x:security/triage" {
				return td
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no reaction to-do for security's subscription")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// security's reaction to shop's published event is a to-do of security, reads
// [shop security], writes [security]; run, it writes into security.
func TestACrossModuleReactionIsTheSubscribersWork(t *testing.T) {
	ctx, cwm, todos, react := newCrossWorld(t, "security")
	if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: "policy.violated", ObjectID: "shop"}); err != nil {
		t.Fatal(err)
	}
	td := waitReaction(t, ctx, todos)
	if td.Module != "security" || !slices.Equal(td.Reads, []string{"shop", "security"}) || !slices.Equal(td.Writes, []string{"security"}) || td.Target != "shop" {
		t.Fatalf("the reaction to-do: module %s reads %v writes %v target %s; want security's, reading shop and security, writing security, on shop", td.Module, td.Reads, td.Writes, td.Target)
	}
	if _, err := todos.Run(ctx, schedapi.RunInput{ID: td.ID, By: cwmapi.Actor{ID: "tester", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "security", ID: "security.finding"}); err != nil {
		t.Fatalf("its write into security: %v", err)
	}
	subs, err := react.Subscribers(ctx, eventsapi.ReactionSubscribersInput{Module: "shop"})
	if err != nil || len(subs.Subscribers) != 1 || subs.Subscribers[0].Subscriber != "security" {
		t.Fatalf("shop's subscribers: %+v %v; want security", subs, err)
	}
}

// Its action writing into shop — not a module it declared — is refused: the
// attempt fails, nothing is written.
func TestACrossModuleReactionCannotWriteTheEmitter(t *testing.T) {
	ctx, cwm, todos, _ := newCrossWorld(t, "shop")
	if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: "policy.violated", ObjectID: "shop"}); err != nil {
		t.Fatal(err)
	}
	td := waitReaction(t, ctx, todos)
	if _, err := todos.Run(ctx, schedapi.RunInput{ID: td.ID, By: cwmapi.Actor{ID: "tester", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: td.ID})
	if err != nil || got.Todo.Attempts != 1 {
		t.Fatalf("after its attempt: %+v %v; want failed, put back", got.Todo, err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "shop", ID: "shop.finding"}); err == nil {
		t.Fatal("a cross-module reaction wrote into the emitter")
	}
}
