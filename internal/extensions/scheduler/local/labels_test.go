// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// LABELS, RANK, PREFER — the owner's opaque labels filter the queue (every one
// listed is carried), its rank orders a priority (set first, lower first), and
// a worker's preferred module only breaks a tie of priority and rank
// . The scheduler gives labels and rank no meaning.

func queueIDs(t *testing.T, ctx context.Context, svc api.TodoService, f api.Filter) []string {
	t.Helper()
	q, err := svc.Queue(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, e := range q.Queue {
		ids = append(ids, e.Todo.ID)
	}
	return ids
}

func TestALabelFilterKeepsTheToDosCarryingEveryLabel(t *testing.T) {
	ctx, _, svc, _ := world(t)
	a := put(t, ctx, svc, api.Todo{Module: "billing", Title: "a", Labels: []string{"release:A"}})
	b := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "b", Labels: []string{"release:B"}})
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "unlabelled"})
	d := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "d", Labels: []string{"team:x", "release:A"}})

	if got := queueIDs(t, ctx, svc, api.Filter{Labels: []string{"release:A"}}); !slices.Equal(got, []string{a.ID, d.ID}) {
		t.Errorf("release:A queue %v; want %s and %s, not the other release nor the unlabelled", got, a.ID, d.ID)
	}
	if got := queueIDs(t, ctx, svc, api.Filter{Labels: []string{"release:A", "team:x"}}); !slices.Equal(got, []string{d.ID}) {
		t.Errorf("release:A + team:x queue %v; want only %s (both labels)", got, d.ID)
	}
	out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: []string{"release:B"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo == nil || out.Todo.ID != b.ID {
		t.Errorf("pull release:B: %+v; want %s", out, b.ID)
	}
	if none, _ := svc.Pull(ctx, api.PullInput{By: bob, Filter: api.Filter{Labels: []string{"release:C"}}}); none.Todo != nil {
		t.Errorf("pull release:C: %+v; want nothing", none)
	}
}

func TestRankOrdersAPriority(t *testing.T) {
	ctx, _, svc, _ := world(t)
	unset := put(t, ctx, svc, api.Todo{Module: "billing", Title: "unset"})
	two := put(t, ctx, svc, api.Todo{Module: "billing", Title: "two", Rank: 2})
	one := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "one", Rank: 1})
	urgent := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "urgent unranked", Priority: "P1"})

	want := []string{urgent.ID, one.ID, two.ID, unset.ID}
	if got := queueIDs(t, ctx, svc, api.Filter{}); !slices.Equal(got, want) {
		t.Errorf("queue %v; want %v (priority, then a set rank lower first, then unset)", got, want)
	}
}

func TestPreferBreaksOnlyATie(t *testing.T) {
	pull := func(t *testing.T, ctx context.Context, svc api.TodoService, prefer string) string {
		t.Helper()
		out, err := svc.Pull(ctx, api.PullInput{By: alice, Prefer: prefer})
		if err != nil {
			t.Fatal(err)
		}
		if out.Todo == nil {
			t.Fatalf("nothing pulled: %+v", out)
		}
		return out.Todo.ID
	}
	t.Run("a tie", func(t *testing.T) {
		ctx, _, svc, _ := world(t)
		first := put(t, ctx, svc, api.Todo{Module: "billing", Title: "billing", Rank: 1})
		pref := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "shipping", Rank: 1})
		if got := queueIDs(t, ctx, svc, api.Filter{}); got[0] != first.ID {
			t.Fatalf("queue %v; want billing first without a preference", got)
		}
		if got := pull(t, ctx, svc, "shipping"); got != pref.ID {
			t.Errorf("prefer shipping pulled %s; want %s", got, pref.ID)
		}
	})
	t.Run("not over a rank", func(t *testing.T) {
		ctx, _, svc, _ := world(t)
		ranked := put(t, ctx, svc, api.Todo{Module: "billing", Title: "billing", Rank: 1})
		put(t, ctx, svc, api.Todo{Module: "shipping", Title: "shipping", Rank: 2})
		if got := pull(t, ctx, svc, "shipping"); got != ranked.ID {
			t.Errorf("prefer shipping pulled %s; want the lower rank %s", got, ranked.ID)
		}
	})
	t.Run("not over a priority", func(t *testing.T) {
		ctx, _, svc, _ := world(t)
		urgent := put(t, ctx, svc, api.Todo{Module: "billing", Title: "billing", Priority: "P1"})
		put(t, ctx, svc, api.Todo{Module: "shipping", Title: "shipping", Rank: 1})
		if got := pull(t, ctx, svc, "shipping"); got != urgent.ID {
			t.Errorf("prefer shipping pulled %s; want the more urgent %s", got, urgent.ID)
		}
	})
}

func TestALockHeldOutsideTheLabelStillBlocks(t *testing.T) {
	ctx, _, svc, _ := world(t)
	writer := put(t, ctx, svc, api.Todo{Module: "billing", Title: "unlabelled writer", Priority: "P1", Writes: []string{"billing"}})
	labelled := put(t, ctx, svc, api.Todo{Module: "billing", Title: "labelled writer", Labels: []string{"release:A"}, Writes: []string{"billing"}})
	if out, _ := svc.Pull(ctx, api.PullInput{By: alice}); out.Todo == nil || out.Todo.ID != writer.ID {
		t.Fatalf("first pull: %+v; want the unlabelled writer", out)
	}
	out, err := svc.Pull(ctx, api.PullInput{By: bob, Filter: api.Filter{Labels: []string{"release:A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo != nil || len(out.Waiting) != 1 || !strings.HasPrefix(out.Waiting[0], labelled.ID) {
		t.Errorf("release:A pull: %+v; want %s waiting on billing, written outside the label", out, labelled.ID)
	}
}

func TestPullsOfOneLabelNeverTakeTheSameToDo(t *testing.T) {
	ctx, _, svc, _ := world(t)
	want := map[string]bool{}
	for i := range 6 {
		m := []string{"billing", "shipping"}[i%2]
		want[put(t, ctx, svc, api.Todo{Module: m, Title: fmt.Sprintf("step %d", i), Labels: []string{"release:A"}}).ID] = true
	}
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "other release", Labels: []string{"release:B"}})
	workers := make([]cwmapi.Actor, 9)
	got := make([]*api.Todo, len(workers))
	var wg sync.WaitGroup
	for i := range workers {
		workers[i] = cwmapi.Actor{ID: fmt.Sprintf("claude-%d", i), Kind: "agent"}
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := svc.Pull(ctx, api.PullInput{By: workers[i], Filter: api.Filter{Labels: []string{"release:A"}}})
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = out.Todo
		}()
	}
	wg.Wait()
	seen := map[string]string{}
	for i, td := range got {
		if td == nil {
			continue
		}
		if other, dup := seen[td.ID]; dup {
			t.Fatalf("%s pulled by %s and %s", td.ID, other, workers[i].ID)
		}
		if !want[td.ID] {
			t.Errorf("%s pulled %s, not of release:A", workers[i].ID, td.ID)
		}
		seen[td.ID] = workers[i].ID
	}
	if len(seen) != len(want) {
		t.Errorf("pulled %v; want each of the %d release:A to-dos once", seen, len(want))
	}
}

// A Put replaces the whole to-do, its labels and rank with it: an owner
// rewriting a to-do gives every label it keeps.
func TestAPutReplacesTheLabels(t *testing.T) {
	ctx, _, svc, _ := world(t)
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "a", Labels: []string{"release:A", "team:x"}, Rank: 3})
	td.Labels, td.Rank = []string{"release:B"}, 0
	put(t, ctx, svc, td)
	got, err := svc.Get(ctx, api.GetInput{ID: td.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Todo.Labels, []string{"release:B"}) || got.Todo.Rank != 0 {
		t.Errorf("after the put: labels %v rank %d; want [release:B] and 0", got.Todo.Labels, got.Todo.Rank)
	}
	td.Labels = nil
	put(t, ctx, svc, td)
	if got, _ := svc.Get(ctx, api.GetInput{ID: td.ID}); len(got.Todo.Labels) != 0 {
		t.Errorf("after a put without labels: %v; want none", got.Todo.Labels)
	}
	if _, err := svc.Put(ctx, api.PutInput{Todo: api.Todo{Module: "billing", Owner: "test", Title: "bad", For: api.ForAgent, Start: api.StartAuto, Priority: "P2", Rank: -1}}); err == nil {
		t.Error("a negative rank was put")
	}
}

// AN AGENT HOLDS ONE TO-DO AT A TIME (OD-006) — a second pull by an agent
// holding one answers nothing and names it, even for another to-do by id;
// released, the next pull answers; a person is not bound.
func TestAnAgentHoldsOneToDoAtATime(t *testing.T) {
	ctx, _, svc, _ := world(t)
	a := put(t, ctx, svc, api.Todo{Module: "billing", Title: "first"})
	b := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "second"})
	agent := cwmapi.Actor{ID: "claude-one", Kind: "agent"}
	first, err := svc.Pull(ctx, api.PullInput{By: agent, ID: a.ID})
	if err != nil || first.Todo == nil {
		t.Fatalf("first pull: %+v, %v", first, err)
	}
	for _, in := range []api.PullInput{{By: agent}, {By: agent, ID: b.ID}} {
		again, err := svc.Pull(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if again.Todo != nil || !strings.Contains(again.Nothing, a.ID) {
			t.Errorf("a pull (id %q) while holding %s: %+v; want nothing, naming the one held", in.ID, a.ID, again)
		}
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: a.ID, By: agent, Note: "given back"}); err != nil {
		t.Fatal(err)
	}
	if next, err := svc.Pull(ctx, api.PullInput{By: agent, ID: b.ID}); err != nil || next.Todo == nil {
		t.Errorf("after releasing %s, a pull of %s: %+v, %v; want it", a.ID, b.ID, next, err)
	}
	person := cwmapi.Actor{ID: "jdoe", Kind: "person"}
	c := put(t, ctx, svc, api.Todo{Module: "billing", Title: "third", For: api.ForPerson})
	d := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "fourth", For: api.ForPerson})
	for _, id := range []string{c.ID, d.ID} {
		if got, err := svc.Pull(ctx, api.PullInput{By: person, ID: id}); err != nil || got.Todo == nil {
			t.Errorf("a person's pull of %s while holding another: %+v, %v; want it (a person is not bound)", id, got, err)
		}
	}
}

// : a ready writer the filter does not keep reserves nothing -
// an older P0 to-do outside any release, nobody taking it, never stops a pull
// filtered on a release from taking that release's to-do on the same module.
func TestAReadyWriterOutsideTheLabelReservesNothing(t *testing.T) {
	ctx, _, svc, _ := world(t)
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "older P0 outside any release", Priority: "P0", Writes: []string{"billing"}})
	labelled := put(t, ctx, svc, api.Todo{Module: "billing", Title: "release work", Priority: "P2", Labels: []string{"release:A"}, Writes: []string{"billing"}})
	q, err := svc.Queue(ctx, api.Filter{Labels: []string{"release:A"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Queue) != 1 || q.Queue[0].Todo.ID != labelled.ID || !q.Queue[0].Takeable {
		t.Fatalf("release:A queue %+v; want %s takeable", q.Queue, labelled.ID)
	}
	out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: []string{"release:A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo == nil || out.Todo.ID != labelled.ID {
		t.Errorf("release:A pull: %+v; want %s, the older writer outside the release reserving nothing", out, labelled.ID)
	}
}
