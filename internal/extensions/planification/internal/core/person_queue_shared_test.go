// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"

	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// « À faire »'s page and the rail's badge share one computation of the queue
// : a count asked while the page computes it answers
// with the page, in the time of one computation, not two; a read after it
// ended computes again (nothing stale is kept); the page never joins one
// already running.
func TestThePageAndItsCountShareOneComputationOfTheQueue(t *testing.T) {
	const took = 150 * time.Millisecond
	runs := &queueRuns{running: map[string]*queueRun{}}
	var computed atomic.Int64
	slow := func(n int) func() (queueBase, error) {
		return func() (queueBase, error) {
			computed.Add(1)
			time.Sleep(took)
			items := make([]api.PersonItem, n)
			for i := range items {
				items[i] = api.PersonItem{Key: "k", Action: api.ActionAnswer}
			}
			return queueBase{items: items}, nil
		}
	}
	ctx := context.Background()

	// The page and two counts asked at once: one computation, every answer
	// within about one computation's time.
	start := time.Now()
	var wg sync.WaitGroup
	var page queueBase
	counts := make([]int, 2)
	wg.Add(1)
	go func() { defer wg.Done(); page, _ = runs.run(ctx, "u|fr", false, slow(3)) }()
	time.Sleep(20 * time.Millisecond) // the page asked first, as the screen opens
	for i := range counts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b, _ := runs.run(ctx, "u|fr", true, slow(9))
			counts[i] = scopedQueue(b, api.PersonQueueInput{}).Count
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)
	if got := computed.Load(); got != 1 {
		t.Fatalf("the page and two counts asked together computed the queue %d times, want 1", got)
	}
	if want := scopedQueue(page, api.PersonQueueInput{}).Count; counts[0] != want || counts[1] != want {
		t.Errorf("counts %v, want the page's %d", counts, want)
	}
	if elapsed > took+took/2 {
		t.Errorf("the page and its counts answered in %v - more than one computation (%v)", elapsed, took)
	}

	// Ended, nothing is kept: a count asked now computes again.
	computed.Store(0)
	if _, err := runs.run(ctx, "u|fr", true, slow(1)); err != nil || computed.Load() != 1 {
		t.Errorf("a count after the page ended: computed %d, err %v - want a fresh computation", computed.Load(), err)
	}

	// The page never joins a computation in flight: it may predate the
	// person's last answer.
	computed.Store(0)
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = runs.run(ctx, "u|fr", true, slow(1)) }()
	time.Sleep(20 * time.Millisecond)
	go func() { defer wg.Done(); _, _ = runs.run(ctx, "u|fr", false, slow(1)) }()
	wg.Wait()
	if got := computed.Load(); got != 2 {
		t.Errorf("a page asked during a count's computation computed %d times in all, want 2 (its own)", got)
	}

	// Another universe or language never shares.
	computed.Store(0)
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = runs.run(ctx, "u|fr", false, slow(1)) }()
	time.Sleep(20 * time.Millisecond)
	go func() { defer wg.Done(); _, _ = runs.run(ctx, "v|fr", true, slow(1)) }()
	wg.Wait()
	if got := computed.Load(); got != 2 {
		t.Errorf("two universes computed %d times, want 2", got)
	}

	// A joined computation its caller gave up is computed again for a count
	// still waiting.
	computed.Store(0)
	gone, cancel := context.WithCancel(ctx)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = runs.run(gone, "u|fr", false, func() (queueBase, error) {
			computed.Add(1)
			<-gone.Done()
			return queueBase{}, gone.Err()
		})
	}()
	time.Sleep(20 * time.Millisecond)
	var err error
	var got queueBase
	go func() { defer wg.Done(); got, err = runs.run(ctx, "u|fr", true, slow(2)) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()
	if err != nil || len(got.items) != 2 || computed.Load() != 2 {
		t.Errorf("a count joining a given-up page: err %v, %d items, %d computations - want its own answer", err, len(got.items), computed.Load())
	}
}

// The scope never changes the shared queue: two callers scoping one base
// each get their own lists.
func TestScopingTheSharedQueueLeavesItWhole(t *testing.T) {
	base := queueBase{items: []api.PersonItem{
		{Key: "a", Action: api.ActionAnswer, Module: "m"},
		{Key: "b", Action: api.ActionAnswer, Module: "n"},
	}}
	m := scopedQueue(base, api.PersonQueueInput{Target: "m"})
	all := scopedQueue(base, api.PersonQueueInput{})
	if len(m.Items) != 1 || len(all.Items) != 2 || len(base.items) != 2 || base.items[0].Key != "a" {
		t.Fatalf("scoped %d, all %d, base %v", len(m.Items), len(all.Items), base.items)
	}
}

// « À faire »'s answer forms come already resolved:
// the ui block of each inline-schema form is laid over its schema here, so
// its host has nothing left to ask the object pane - no second round trip
// carrying the whole page back. A form naming its schema by reference is left
// for the host to resolve.
func TestÀFairesFormsComeResolved(t *testing.T) {
	form := render.AnswerForm(render.InlineSchema[holdForm]("hold", nil), render.Action{Label: "Hold", URI: requestHoldURI}, "pause", nil)
	tree := node{"kind": "vbox", "children": []node{{"kind": "card", "children": []render.Node{form}}}}
	got := selfContained(tree)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), `"ui":`) {
		t.Fatalf("a form still lays a ui block for its host to resolve: %s", raw)
	}
	if !strings.Contains(string(raw), `"ui-form"`) || !strings.Contains(string(raw), `"variant":"default"`) {
		t.Errorf("the ui block is not laid over the schema as the object pane lays it: %s", raw)
	}
	if !render.HasComponents(got) {
		t.Error("the form is gone")
	}
	// A schema by reference is left as it is.
	ref := node{"kind": "vbox", "children": []render.Node{render.ActionForm("Run", "ktw-agents:x/y"), form}}
	raw, _ = json.Marshal(selfContained(ref))
	if !strings.Contains(string(raw), `"ui":`) || !strings.Contains(string(raw), `"action":"ktw-agents:x/y"`) {
		t.Errorf("a tree naming a schema by reference changed: %s", raw)
	}
}

// A click on « À faire » - another tile, a kind chip, a card to show first -
// answers from the page's last queue while it is fresh; a re-read of the same
// view computes again, and so does a click once the queue is older than
// queryReuse.
func TestAClickReusesThePagesQueueAReReadComputesAgain(t *testing.T) {
	runs := &queueRuns{running: map[string]*queueRun{}}
	now := time.Now()
	all, answer := pageView(api.PersonQueueInput{}), pageView(api.PersonQueueInput{Action: api.ActionAnswer})
	if _, ok := runs.reuse("u|fr", answer, 0, now); ok {
		t.Fatal("nothing read yet, yet a queue was reused")
	}
	base := queueBase{items: []api.PersonItem{{Key: "k", Action: api.ActionAnswer}}}
	runs.keep("u|fr", all, base, now, 0)
	if _, ok := runs.reuse("u|fr", all, 0, now.Add(time.Second)); ok {
		t.Error("a re-read of the same view reused the queue; it must compute again")
	}
	got, ok := runs.reuse("u|fr", answer, 0, now.Add(time.Second))
	if !ok || len(got.items) != 1 {
		t.Fatalf("a click on a tile computed the queue again (reused %v)", ok)
	}
	// Shown in that view now: asking it again is a re-read.
	if _, ok := runs.reuse("u|fr", answer, 0, now.Add(2*time.Second)); ok {
		t.Error("a re-read of the tile's view reused the queue")
	}
	if _, ok := runs.reuse("u|fr", all, 0, now.Add(2*time.Second)); !ok {
		t.Error("« Tout » after a tile computed the queue again")
	}
	if _, ok := runs.reuse("u|fr", answer, 0, now.Add(queryReuse)); ok {
		t.Error("a click reused a queue older than queryReuse")
	}
	if _, ok := runs.reuse("u|en", answer, 0, now.Add(time.Second)); ok {
		t.Error("another language reused this one's queue")
	}
	// An older computation never replaces a later one.
	runs.keep("u|fr", all, queueBase{}, now.Add(-time.Second), 0)
	if got, _ := runs.reuse("u|fr", answer, 0, now.Add(time.Second)); len(got.items) != 1 {
		t.Error("an older computation replaced the later one")
	}
}

// Two readers with different views - the rail's « À faire » and a pane's
// « À faire » panel, two windows, the mobile companion - re-reading after a
// change never get the queue read before it, whether they ask fresh or not;
// a change between a read and a click makes the click compute again
func TestAChangeEndsTheKeptQueueForEveryReader(t *testing.T) {
	runs := &queueRuns{running: map[string]*queueRun{}}
	now := time.Now()
	rail := pageView(api.PersonQueueInput{})
	pane := pageView(api.PersonQueueInput{Target: "planification", ID: "planification"})
	before := queueBase{items: []api.PersonItem{{Key: "answered-since"}}}
	mark := uint64(7)
	runs.keep("u|fr", pane, before, now, mark)
	// The person answers: an object event marks the universe changed.
	mark++
	for _, view := range []string{rail, pane, rail} {
		if got, ok := runs.reuse("u|fr", view, mark, now.Add(2*time.Second)); ok {
			t.Fatalf("a reader re-reading after a change got the queue read before it: %v", got.items)
		}
	}
	// A click with no change since the read reuses it; one after a change
	// computes again.
	runs.keep("u|fr", rail, queueBase{items: []api.PersonItem{{Key: "now"}}}, now.Add(3*time.Second), mark)
	if _, ok := runs.reuse("u|fr", pane, mark, now.Add(4*time.Second)); !ok {
		t.Error("a click with nothing changed since the read computed the queue again")
	}
	if _, ok := runs.reuse("u|fr", rail, mark+1, now.Add(4*time.Second)); ok {
		t.Error("a click after a change was answered from the read before it")
	}
}

// QueueChanged - the CWM observer's call on each object event - ends the
// shared kept read for the clicks that follow.
func TestQueueChangedEndsTheKeptReadForAClick(t *testing.T) {
	t.Cleanup(func() { sharedQueue.mu.Lock(); sharedQueue.pages = nil; sharedQueue.mu.Unlock() })
	ctx := context.Background()
	key := queueKey(ctx, "fr")
	view := pageView(api.PersonQueueInput{Language: "fr"})
	kept := queueBase{items: []api.PersonItem{{Key: "kept", Action: api.ActionAnswer}}}
	sharedQueue.keep(key, view, kept, time.Now(), queueChanges.Load())
	click := api.PersonQueueInput{Language: "fr", Action: api.ActionAnswer}
	if _, ok := sharedQueue.reuse(key, pageView(click), queueChanges.Load(), time.Now()); !ok {
		t.Fatal("a click right after the read computed again")
	}
	sharedQueue.keep(key, view, kept, time.Now(), queueChanges.Load())
	QueueChanged()
	if _, ok := sharedQueue.reuse(key, pageView(click), queueChanges.Load(), time.Now()); ok {
		t.Error("a click after QueueChanged was answered from the read before it")
	}
}
