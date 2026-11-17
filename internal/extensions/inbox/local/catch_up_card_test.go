// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxcore "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/core"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// catchUpCard is the inbox's card of a held catch-up, if one is filed.
func catchUpCard(t *testing.T, ctx context.Context, svc api.InboxService) (api.Entry, int) {
	t.Helper()
	out, err := svc.Triage(ctx, api.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	var card api.Entry
	n := 0
	for _, e := range out.Entries {
		if strings.HasPrefix(e.RequestedBy, "cwm:catch-up/") {
			card, n = e, n+1
		}
	}
	return card, n
}

// eventually polls cond for a few seconds: the observers run on their own
// goroutines.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestAHeldCatchUpFilesOneCardWhoseApprovalReleasesIt drives end
// to end on a real CWM: 51 objects written by hand while CWM is closed are
// caught up at the next open, held - no per-object event goes out - and the
// inbox files exactly one pending card naming the count and the module;
// approving it makes CWM publish the 51 held per-object events.
func TestAHeldCatchUpFilesOneCardWhoseApprovalReleasesIt(t *testing.T) {
	n := cwmapi.CatchUpPerObjectMax + 1
	root := hubharness.TempDir(t)
	var caughtUp atomic.Int64
	counter := cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "caught-up-counter", Handle: func(_ context.Context, ev cwmapi.EventEnvelope) {
		if strings.Contains(ev.Topic, ".object.") && strings.Contains(string(ev.Payload), `"operation":"caught-up"`) {
			caughtUp.Add(1)
		}
	}})
	open := func() (context.Context, cwmapi.CWMService, api.InboxService) {
		ctx, cwm := cwmapp.NewAt(t, root,
			application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
				return eventbus.NewMemoryEventBus(), nil
			}),
			inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t), counter)
		svc, err := application.UseService[api.InboxService](ctx)
		if err != nil {
			t.Fatal(err)
		}
		return ctx, cwm, svc
	}

	// The baseline: the inbox and one object written through the verbs.
	ctx, cwm, _ := open()
	seedInbox(t, ctx, cwm)
	cwmapp.Module(t, ctx, cwm, "demo")
	seed := cwmapi.Object{ID: "demo.o0000", Type: "note", Name: "seed"}
	seed.Parent = "demo"
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "demo", Object: seed})
	if err != nil {
		t.Fatal(err)
	}
	appcore.CloseApplicationScope(ctx)

	// A bulk copy while CWM is closed: n new objects, plain files.
	seedPath := filepath.Join(root, filepath.FromSlash(made.Path))
	raw, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read the seed's file %s: %v", seedPath, err)
	}
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("demo.o%04d", i)
		body := strings.ReplaceAll(string(raw), "demo.o0000", id)
		path := strings.ReplaceAll(seedPath, "o0000", fmt.Sprintf("o%04d", i))
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cwm, svc := open()
	if _, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "demo"}); err != nil { // loads the index: the catch-up
		t.Fatal(err)
	}
	var card api.Entry
	eventually(t, "the held catch-up's card is filed", func() bool {
		var count int
		card, count = catchUpCard(t, ctx, svc)
		if count > 1 {
			t.Fatalf("%d cards for one catch-up, want one", count)
		}
		return count == 1
	})
	if !strings.HasPrefix(card.Title, fmt.Sprintf("%d objects changed", n)) || !strings.Contains(card.Description, "demo") ||
		card.Status != api.StatusPending || card.Expects != api.ExpectsVerdict {
		t.Fatalf("the card: %+v; want « %d objects changed ... », pending, a verdict, naming the module demo", card, n)
	}
	if got := caughtUp.Load(); got != 0 {
		t.Fatalf("%d per-object events went out before the card was answered, want none", got)
	}

	if _, err := svc.Answer(ctx, api.AnswerInput{ID: card.ID, By: "jdoe"}); err != nil {
		t.Fatalf("approve the card: %v", err)
	}
	eventually(t, fmt.Sprintf("the approval releases %d per-object events", n), func() bool { return caughtUp.Load() >= int64(n) })
	time.Sleep(100 * time.Millisecond)
	if got := caughtUp.Load(); got != int64(n) {
		t.Fatalf("the approval released %d per-object events, want %d", got, n)
	}
	if st := entryStatus(t, ctx, svc, card.ID); st != api.StatusApproved {
		t.Fatalf("the card is %s after its approval, want approved", st)
	}
}

// TestACatchUpCardIsFiledOncePerCommitAndOnlyAboveTheThreshold pins the
// card's rule on the grouped event itself: 50 objects file none (their events
// went out), 51 file one, a second catch-up of the same commit none more.
func TestACatchUpCardIsFiledOncePerCommitAndOnlyAboveTheThreshold(t *testing.T) {
	ctx, _, svc := setup(t)
	core := inboxcore.New()
	event := func(id string, count int) cwmapi.ObjectsCaughtUpEventPayload {
		return cwmapi.ObjectsCaughtUpEventPayload{EventID: id, Count: count, Held: count > cwmapi.CatchUpPerObjectMax,
			Sha: "0123456789abcdef", By: cwmapi.Actor{ID: "someone", Kind: cwmapi.ActorOutside},
			Modified: []string{"alpha.x", "beta.y"}, Cause: cwmapi.CaughtUpCause{Kind: cwmapi.CaughtUpCommits}}
	}
	if _, filed, err := core.FileCatchUp(ctx, event("e-50", cwmapi.CatchUpPerObjectMax)); err != nil || filed {
		t.Fatalf("a catch-up of %d objects filed a card (%v, %v), want none", cwmapi.CatchUpPerObjectMax, filed, err)
	}
	first, filed, err := core.FileCatchUp(ctx, event("e-51", cwmapi.CatchUpPerObjectMax+1))
	if err != nil || !filed {
		t.Fatalf("a held catch-up of 51 objects filed no card: %v", err)
	}
	if first.Title != "51 objects changed by commit 0123456789 (someone)" || !strings.Contains(first.Description, "alpha, beta") {
		t.Fatalf("the card: title %q description %q", first.Title, first.Description)
	}
	again, filed, err := core.FileCatchUp(ctx, event("e-51-again", cwmapi.CatchUpPerObjectMax+1))
	if err != nil || filed || again.ID != first.ID {
		t.Fatalf("a second catch-up of the same commit: filed %v (%s), %v; want the first card %s", filed, again.ID, err, first.ID)
	}
	if _, count := catchUpCard(t, ctx, svc); count != 1 {
		t.Fatalf("%d catch-up cards in the inbox, want one", count)
	}
	// Dismissed, the card releases nothing and stays the commit's one card.
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, filed, _ := core.FileCatchUp(ctx, event("e-51-third", cwmapi.CatchUpPerObjectMax+1)); filed {
		t.Fatal("a dismissed card's commit got a second card")
	}
}
