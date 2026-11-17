// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// stamper is a third worker, a member of the holding list as another is.
var stamper = cwmapi.Actor{ID: "session-c", Kind: "agent"}

// planningHeldFor makes planification's task group a holding list of the
// members given: only they pull its to-dos.
func planningHeldFor(members ...cwmapi.Actor) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("planification-test", ".group-members", func(app *application.Application) error {
			return schedlocal.ContributeGroupMembers(app, schedapi.GroupMembers{Name: "test-lists", Members: func(_ context.Context, group string) ([]cwmapi.Actor, bool, error) {
				if group != schedapi.DefaultGroup {
					return nil, false, nil
				}
				return members, true, nil
			}})
		})
	}
}

// THE PULL NAMES A WAIT EXACTLY WHERE THE CHECK DOES (
// ): on one stored state holding a ready to-do behind a
// module held by a worker, one behind a module reserved by a ready to-do ahead,
// one in its retry back-off, one given to another worker and one held for a
// holding list, an unfiltered pull repeated until nothing is takeable lists a
// waiting line for each, and every line starts with the kind and holder
// planning check gives that to-do; every queue entry's structured wait is
// named so too - one mapping, never a line worded on its own.
func TestThePullNamesAWaitExactlyWhereTheCheckDoes(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldExtended(t, nil, nil, planningHeldFor(another, stamper))
	for _, m := range []string{"shipping", "stamps", "archive"} {
		cwmapp.Module(t, ctx, cwm, m)
	}
	s := schedOf(t, ctx)
	intake := func(request string) string { return stepOf(t, ctx, request, api.StepIntake).ID }
	// writing makes a request's intake write its module (a module lock and a
	// reservation are a writer's), retried up to attempts times by the
	// scheduler itself (a back-off is its own retry).
	writing := func(request string, attempts int) string {
		got, err := s.Get(ctx, schedapi.GetInput{ID: intake(request)})
		if err != nil {
			t.Fatal(err)
		}
		td := got.Todo
		td.Writes, td.MaxAttempts, td.BackoffSeconds = []string{td.Module}, attempts, 3600
		if _, err := s.Put(ctx, schedapi.PutInput{Todo: td}); err != nil {
			t.Fatal(err)
		}
		return td.ID
	}

	// billing: a worker holds a step; the next waits on the module's lock.
	holding := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing"}).ID
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: writing(holding, 1), By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull: %v %+v", err, got)
	}
	locked := file(t, ctx, req, api.FileInput{Title: "Print the invoices", Module: "billing"}).ID
	writing(locked, 1)
	// shipping: a step given to another worker, ready ahead in the queue,
	// reserves the module; the next waits on that reservation.
	given := file(t, ctx, req, api.FileInput{Title: "Pack the parcels", Module: "shipping"}).ID
	if _, err := s.Assign(ctx, schedapi.AssignInput{ID: writing(given, 1), To: another, By: cwmapi.Actor{ID: "jdoe", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	reserved := file(t, ctx, req, api.FileInput{Title: "Label the parcels", Module: "shipping"}).ID
	writing(reserved, 1)
	// stamps: a step failed once waits for its retry back-off.
	backoff := file(t, ctx, req, api.FileInput{Title: "Stamp the invoices", Module: "stamps"}).ID
	pullAndReport(t, ctx, writing(backoff, 3), stamper, schedapi.Result{Status: schedapi.StatusFail, Summary: "the stamp is missing"})
	// archive: a step only the holding list's members take.
	members := file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "archive"}).ID
	works := []string{locked, reserved, backoff, given, members}

	// An unfiltered pull repeated until nothing is takeable: its last answer
	// lists every waiting line.
	var last schedapi.PullOutput
	for i := 0; ; i++ {
		got, err := s.Pull(ctx, schedapi.PullInput{By: session})
		if err != nil {
			t.Fatal(err)
		}
		if got.Todo == nil {
			last = got
			break
		}
		// Every ready to-do of this state waits for session: a take is a
		// wait the pull no longer names.
		t.Errorf("the pull took %s: in this state every ready to-do waits for %s", got.Todo.ID, session.ID)
		if i > 10 {
			t.Fatalf("the pull keeps taking: %+v", got.Todo)
		}
	}
	lines := map[string]string{}
	for _, l := range last.Waiting {
		id, rest, _ := strings.Cut(l, ": ")
		lines[id] = rest
	}

	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	wantKinds := map[string]string{locked: api.BlockerHeld, reserved: api.BlockerHeld, backoff: api.BlockerHeld,
		given: api.BlockerReady, members: api.BlockerReady}
	wantActors := map[string]string{locked: another.ID + " (" + holding + ")", reserved: intake(given),
		backoff: "back-off", given: another.ID, members: "a member of " + schedapi.DefaultGroup}
	labels := map[string]string{}
	for _, work := range works {
		b := blockerOf(t, out, work)
		if b.Kind != wantKinds[work] || !strings.Contains(b.Actor, wantActors[work]) {
			t.Errorf("%s: the check says %s %s by %q; want %s by %q", work, b.Class, b.Kind, b.Actor, wantKinds[work], wantActors[work])
		}
		label := b.Class + " " + b.Kind + " by " + b.Actor
		labels[b.Todo] = label
		line, listed := lines[b.Todo]
		if !listed {
			t.Errorf("%s: the pull lists no waiting line for %s: %v", work, b.Todo, last.Waiting)
			continue
		}
		if !strings.HasPrefix(line, label+" - ") {
			t.Errorf("%s: the pull's line %q; want it to start with the check's %q", work, line, label)
		}
	}

	// Every queue entry's structured wait is named as the check names it.
	q, err := s.Queue(ctx, schedapi.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var namer *schedapi.WaitNamer
	for _, n := range application.UseExtensions[schedapi.WaitNamer](ctx) {
		if n.Owner == api.Owner {
			namer = &n
		}
	}
	if namer == nil {
		t.Fatal("planification contributes no wait namer")
	}
	named := 0
	for _, e := range q.Queue {
		want, ours := labels[e.Todo.ID]
		if !ours || e.Wait == nil {
			continue
		}
		named++
		if got := namer.Name(ctx, e.Todo, *e.Wait); got != want {
			t.Errorf("%s: its queue wait (%s) is named %q; the check says %q", e.Todo.ID, e.Wait.Cause, got, want)
		}
	}
	if named < 3 {
		t.Errorf("%d queue entries carry a structured wait; want the lock, the reservation and the back-off", named)
	}
}
