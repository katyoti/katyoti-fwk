// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A MATCH HANDED OVER AGAIN IS THE SAME REACTION: CWM may hand a match over
// twice (a crash before it forgot it, a retried handover); the to-do is keyed by
// the match's id, so the second Take returns the first to-do.
func TestAMatchTakenTwiceIsOneReaction(t *testing.T) {
	ctx, cwm := cwmapp.New(t, schedlocal.Extension(),
		// A to-do's module is an object carrying the module capability.
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}))
	cwmapp.Module(t, ctx, cwm, "m")
	m := cwmapi.Match{
		ID:           "type:item#on_update#x:react/mark@01a1",
		Subscription: cwmapi.SubscriptionRef{ID: "type:item#on_update#x:react/mark", URI: "x:react/mark", Priority: "P2", Attempts: 3},
		Module:       "m", ObjectID: "m.item", Key: "m.item", At: time.Now(), State: cwmapi.MatchPending,
	}
	r := NewReactions(0)
	for i := 0; i < 2; i++ {
		if err := r.Take(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	list, err := todos.List(ctx, schedapi.ListInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, td := range list.Todos {
		if td.Owner == api.Owner {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d reaction to-dos for one match taken twice; want 1", n)
	}
}
