// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE LIFECYCLE HISTORY IS THE JOURNAL'S: an object stores only its state;
// each move is a `transitioned` record — who, why, forced, which lifecycle —
// and Transitions reads them back, oldest first, per lifecycle, through a
// move of the object.

func movesFixture(t *testing.T) (context.Context, cwmapi.CWMService, api.JournalService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""),
		cwmapp.SystemModules(t),
		journallocal.Extension(),
		cwmapp.Pack(fstest.MapFS{
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
			"capabilities/review.yaml":    {Data: []byte("name: review\nworkflow: {name: review-flow, state: status}\n")},
			"object-types/goal.yaml":      {Data: []byte("name: goal\ncapabilities: [lifecycle]\nworkflow: flow\n")},
			"workflows/flow.yaml": {Data: []byte(`name: flow
initial: idea
states: [{name: idea}, {name: proposed}, {name: accepted}]
transitions:
  - {name: propose, from: [idea], to: proposed}
  - {name: rush, from: [proposed], to: accepted, gate: "false", force: true}
`)},
			"workflows/review-flow.yaml": {Data: []byte(`name: review-flow
initial: open
states: [{name: open}, {name: closed}]
transitions:
  - {name: close, from: [open], to: closed}
`)},
		}),
	)
	// The journal records on its own goroutine: drain it before the
	// workspace is removed.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })
	svc, err := application.UseService[api.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "billing")
	cwmapp.Module(t, ctx, cwm, "payments")
	g := cwmapi.Object{ID: "billing.g", Type: "goal"}
	g.Name, g.Parent = "G", "billing"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

// moves waits for the observer to have recorded n moves of id's lifecycle.
func moves(t *testing.T, ctx context.Context, svc api.JournalService, id, capability string, n int) []cwmapi.LifecycleEntry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := svc.Transitions(ctx, api.TransitionsInput{ID: id, Capability: capability})
		if err != nil {
			t.Fatalf("Transitions(%s): %v", id, err)
		}
		if len(out.Entries) >= n || time.Now().After(deadline) {
			return out.Entries
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTheLifecycleHistoryIsTheJournals(t *testing.T) {
	ctx, cwm, svc := movesFixture(t)
	ana := cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "billing.g", To: "proposed", By: ana, Note: "worth it"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "billing.g", To: "accepted", By: ana, Force: true, Note: "demo"}); err != nil {
		t.Fatal(err)
	}
	// A capability's own workflow is another lifecycle.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: "billing.g"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "billing.g", Capability: "review"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "billing.g", Capability: "review", To: "closed", By: ana}); err != nil {
		t.Fatal(err)
	}
	// The object stores the state alone.
	if g := got.Object.Capabilities[cwmapi.CapLifecycle]; len(g) != 1 || g["state"] != "accepted" {
		t.Fatalf("stored lifecycle = %v, want the state alone", g)
	}
	// Moved to another module: its history follows.
	out, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.g", NewModule: "payments", NewParent: "payments", Reason: "payments owns it"})
	if err != nil {
		t.Fatal(err)
	}
	own := moves(t, ctx, svc, out.ID, "", 2)
	if len(own) != 2 || own[0].From != "idea" || own[0].To != "proposed" || own[0].By != ana || own[0].Note != "worth it" || own[0].Forced || own[0].At == "" {
		t.Fatalf("own lifecycle = %+v, want idea → proposed by ana first", own)
	}
	if e := own[1]; e.To != "accepted" || !e.Forced || e.Transition != "rush" {
		t.Fatalf("the forced move = %+v", e)
	}
	review := moves(t, ctx, svc, out.ID, "review", 1)
	if len(review) != 1 || review[0].From != "open" || review[0].To != "closed" {
		t.Fatalf("review lifecycle = %+v, want open → closed alone", review)
	}
	// The record itself says forced and which lifecycle.
	q, err := svc.Query(ctx, api.QueryInput{ObjectID: "billing.g", Actions: []string{api.ActionTransitioned}})
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]int{}
	forced := 0
	for _, r := range q.Records {
		caps[r.Capability]++
		if r.Forced {
			forced++
		}
	}
	if caps[""] != 2 || caps["review"] != 1 || forced != 1 {
		t.Errorf("transitioned records: per lifecycle %v, forced %d", caps, forced)
	}
}
