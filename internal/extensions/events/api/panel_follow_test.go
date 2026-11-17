// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

type ev struct {
	Seq int64 `json:"seq"`
}

func seqOf(e ev) int64 { return e.Seq }

func seqs(es []ev) []int64 {
	out := []int64{}
	for _, e := range es {
		out = append(out, e.Seq)
	}
	return out
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A follower that joins at a snapshot delivers only what follows it, in
// order, once each.
func TestAPanelFollowerDeliversWhatFollowsItsSnapshotOnce(t *testing.T) {
	f := api.NewPanelFollower(5, seqOf)
	var got []ev
	for _, s := range []int64{3, 5, 6, 6, 7, 4, 8} {
		ready, resync := f.Offer(ev{s})
		if resync {
			t.Fatalf("event %d: resync with no gap", s)
		}
		got = append(got, ready...)
	}
	if want := []int64{6, 7, 8}; !equal(seqs(got), want) {
		t.Fatalf("delivered %v, want %v", seqs(got), want)
	}
	if f.Seq() != 8 {
		t.Fatalf("seq %d, want 8", f.Seq())
	}
}

// An event ahead of a missing one waits for it; a gap - more events waiting
// than the reorder window - makes the follower read the snapshot again, and
// what came meanwhile is delivered after it, nothing twice.
func TestAPanelFollowerResyncsOnAGap(t *testing.T) {
	f := api.NewPanelFollower(2, seqOf)
	if ready, resync := f.Offer(ev{4}); resync || len(ready) != 0 {
		t.Fatalf("4 before 3: ready %v resync %v", seqs(ready), resync)
	}
	if ready, _ := f.Offer(ev{3}); !equal(seqs(ready), []int64{3, 4}) {
		t.Fatalf("3 then the waiting 4: %v", seqs(ready))
	}
	// 5 is lost: 6, 7, ... wait until the window is full.
	var resync bool
	last := int64(6 + api.PanelReorderWindow)
	for s := int64(6); s <= last; s++ {
		var ready []ev
		ready, resync = f.Offer(ev{s})
		if len(ready) != 0 {
			t.Fatalf("%d delivered past the missing 5", s)
		}
	}
	if !resync {
		t.Fatal("a full window of events after a missing one did not ask for the snapshot")
	}
	// The snapshot read again holds events up to 10.
	ready := f.Reset(10)
	if len(ready) == 0 || ready[0].Seq != 11 || ready[len(ready)-1].Seq != last {
		t.Fatalf("after the snapshot: %v, want 11..%d", seqs(ready), last)
	}
	if ready, resync := f.Offer(ev{last + 1}); resync || !equal(seqs(ready), []int64{last + 1}) {
		t.Fatalf("%d after the resync: %v %v", last+1, seqs(ready), resync)
	}
}

// Over the bus: a panel's events published on its out subject reach a late
// subscriber that starts from a snapshot, with no loss or duplicate; its
// input subject is another one.
func TestALateSubscriberFollowsAPanelOverTheBus(t *testing.T) {
	bus := teststub.New()
	ctx := context.Background()
	out := subjects.PanelOutSubject("t", "u", "dp-1")
	if in := subjects.PanelInSubject("t", "u", "dp-1"); in == out {
		t.Fatal("a panel's input comes back on its out subject")
	}
	publish := func(s int64) {
		b, _ := json.Marshal(ev{s})
		if err := bus.Publish(ctx, out, b); err != nil {
			t.Fatal(err)
		}
	}
	for s := int64(1); s <= 4; s++ {
		publish(s) // before the subscriber: in the snapshot
	}
	got := make(chan int64, 16)
	f := api.NewPanelFollower(4, seqOf)
	sub, err := bus.Subscribe(ctx, subjects.PanelsPattern("t", "u"), func(_ context.Context, m mbapi.Message) {
		var e ev
		if json.Unmarshal(m.Data(), &e) != nil {
			return
		}
		ready, _ := f.Offer(e)
		for _, r := range ready {
			got <- r.Seq
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	publish(4) // a repeat the snapshot holds
	for s := int64(5); s <= 7; s++ {
		publish(s)
	}
	var seen []int64
	for len(seen) < 3 {
		select {
		case s := <-got:
			seen = append(seen, s)
		case <-time.After(5 * time.Second):
			t.Fatalf("received %v, want [5 6 7]", seen)
		}
	}
	if !equal(seen, []int64{5, 6, 7}) {
		t.Fatalf("received %v, want [5 6 7]", seen)
	}
}
