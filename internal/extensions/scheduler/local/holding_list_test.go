// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
)

// HOLDING LISTS — a to-do's group can be a holding list
// whose members, persons or agents, are its workers: only a member pulls its
// to-dos, the claim says who took it, a member hands it back or it is given to
// another member. The to-do is the one record: no second queue.

var carol = cwmapi.Actor{ID: "session-c", Kind: "agent"}

// reviewers lists hl-reviewers: alice (an agent) and the person.
func reviewers() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scheduler-test", ".group-members", func(app *application.Application) error {
			return schedlocal.ContributeGroupMembers(app, api.GroupMembers{Name: "test-lists", Members: func(_ context.Context, group string) ([]cwmapi.Actor, bool, error) {
				if group != "hl-reviewers" {
					return nil, false, nil
				}
				return []cwmapi.Actor{alice, person}, true, nil
			}})
		})
	}
}

func pullID(t *testing.T, ctx context.Context, svc api.TodoService, in api.PullInput) (string, api.PullOutput) {
	t.Helper()
	out, err := svc.Pull(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo == nil {
		return "", out
	}
	return out.Todo.ID, out
}

func TestOnlyAMemberPullsAHoldingListsToDo(t *testing.T) {
	ctx, _, svc, _ := world(t, reviewers())
	listed := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Group: "hl-reviewers"})
	open := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "open work"})

	id, out := pullID(t, ctx, svc, api.PullInput{By: bob, ID: listed.ID})
	if id != "" {
		t.Fatalf("bob, no member of hl-reviewers, pulled %s", id)
	}
	if !strings.Contains(strings.Join(out.Waiting, " "), "held for the members of hl-reviewers") {
		t.Errorf("bob is not told why: %+v", out)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: bob}); id != open.ID {
		t.Errorf("bob pulled %q; want the open group's %s, never the list's", id, open.ID)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: alice}); id != listed.ID {
		t.Fatalf("alice, a member, pulled %q; want %s", id, listed.ID)
	}
	got, err := svc.Get(ctx, api.GetInput{ID: listed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Todo.Claim == nil || got.Todo.Claim.By != alice {
		t.Errorf("the claim does not say alice took it: %+v", got.Todo.Claim)
	}
}

func TestAMembersClaimHidesTheToDoFromTheOtherMembersAndHandingItBackReturnsIt(t *testing.T) {
	ctx, _, svc, _ := world(t, reviewers())
	listed := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Group: "hl-reviewers", For: api.ForPerson})

	if id, _ := pullID(t, ctx, svc, api.PullInput{By: person, ID: listed.ID}); id != listed.ID {
		t.Fatalf("the person, a member, could not pull %s", listed.ID)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: person, Filter: api.Filter{Group: "hl-reviewers", For: api.ForPerson}}); id != "" {
		t.Fatalf("a claimed to-do was pulled again: %s", id)
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: listed.ID, By: person, Note: "not mine after all"}); err != nil {
		t.Fatal(err)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: person, ID: listed.ID}); id != listed.ID {
		t.Errorf("handed back, %s is not pullable again", listed.ID)
	}
}

func TestAToDoGivenToAMemberIsOnlyTheirsUntilTheyHandItBack(t *testing.T) {
	ctx, _, svc, _ := world(t, reviewers())
	listed := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Group: "hl-reviewers"})

	if _, err := svc.Assign(ctx, api.AssignInput{ID: listed.ID, To: carol, By: person}); err == nil {
		t.Fatal("given to carol, no member of hl-reviewers")
	}
	if _, err := svc.Assign(ctx, api.AssignInput{ID: listed.ID, To: person, By: bob}); err == nil {
		t.Fatal("given by bob, neither a person, a member nor its holder")
	}
	// alice holds it; the person gives it to themselves: alice's claim ends.
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: alice, ID: listed.ID}); id != listed.ID {
		t.Fatalf("alice could not pull %s", listed.ID)
	}
	got, err := svc.Assign(ctx, api.AssignInput{ID: listed.ID, To: person, By: person, Note: "I take it"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Todo.State != api.StateReady || got.Todo.Claim != nil || got.Todo.Assignee == nil || *got.Todo.Assignee != person {
		t.Fatalf("given to the person: %+v; want ready, alice's claim ended, assignee the person", got.Todo)
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: listed.ID, By: alice}); err == nil {
		t.Error("alice released a to-do she no longer holds")
	}
	if id, out := pullID(t, ctx, svc, api.PullInput{By: alice, ID: listed.ID}); id != "" || !strings.Contains(strings.Join(out.Waiting, " "), "given to jdoe") {
		t.Fatalf("alice pulled %q (%+v); want it held for the person it was given to", id, out)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: person, ID: listed.ID}); id != listed.ID {
		t.Fatalf("the person could not pull what was given to them")
	}
	back, err := svc.Release(ctx, api.ReleaseInput{ID: listed.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if back.Todo.Assignee != nil {
		t.Errorf("handed back, it is still given to %+v", back.Todo.Assignee)
	}
	if id, _ := pullID(t, ctx, svc, api.PullInput{By: alice, ID: listed.ID}); id != listed.ID {
		t.Errorf("handed back to the list, alice cannot pull it")
	}
}

func TestAWaitEndsFinishedWhenOnlyAnotherListsToDosAreLeft(t *testing.T) {
	ctx, _, svc, _ := world(t, reviewers())
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Group: "hl-reviewers"})
	out, err := svc.Pull(ctx, api.PullInput{By: bob, Wait: "5s"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo != nil || out.Ended != api.EndedFinished {
		t.Errorf("bob's wait: %+v; want finished - only hl-reviewers' to-do is left, and bob is no member", out)
	}
}

// The queue marks who alone may take an entry, whoever looks - the worker it
// is given to or its holding list - from the rule a pull applies, so a reader
// of the queue never reads group membership again.
func TestTheQueueMarksAnEntryOnlyOneWorkerOrAHoldingListMayTake(t *testing.T) {
	ctx, _, svc, _ := world(t, reviewers())
	listed := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Group: "hl-reviewers"})
	given := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "given", Group: "hl-reviewers"})
	open := put(t, ctx, svc, api.Todo{Module: "billing", Title: "open work"})
	if _, err := svc.Assign(ctx, api.AssignInput{ID: given.ID, To: alice, By: person}); err != nil {
		t.Fatal(err)
	}
	q, err := svc.Queue(ctx, api.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]api.QueueEntry{}
	for _, e := range q.Queue {
		entries[e.Todo.ID] = e
	}
	if e := entries[listed.ID]; e.Only == nil || e.Only.Cause != api.WaitMembers || e.Only.Group != "hl-reviewers" || e.Only.Todo != listed.ID {
		t.Errorf("the holding list's to-do: only %+v; want members of hl-reviewers", e.Only)
	}
	if e := entries[given.ID]; e.Only == nil || e.Only.Cause != api.WaitAssigned || e.Only.Holder == nil || *e.Only.Holder != alice {
		t.Errorf("the to-do given to alice: only %+v; want assigned to alice", e.Only)
	}
	if e, ok := entries[open.ID]; !ok || e.Only != nil || !e.Takeable {
		t.Errorf("the open group's to-do: %+v; want takeable by any worker, no only", e)
	}
	for _, id := range []string{listed.ID, given.ID} {
		if e := entries[id]; !e.Takeable || e.Wait != nil {
			t.Errorf("%s: takeable %v wait %+v; the mark leaves takeable and wait as they were", id, e.Takeable, e.Wait)
		}
	}
}
