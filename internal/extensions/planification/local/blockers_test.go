// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// WHAT HOLDS EACH PIECE OF WORK - one reader sorts every
// open piece of work of a scope into one kind, K1-K6, with who moves it and
// since when; `planning check` prints it and writes nothing.

func check(t *testing.T, ctx context.Context, in api.PlanningCheckInput) api.PlanningCheckOutput {
	t.Helper()
	pl, err := application.UseService[api.PlanningService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pl.Check(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// blockerOf is the one blocker the check gives a piece of work.
func blockerOf(t *testing.T, out api.PlanningCheckOutput, work string) api.Blocker {
	t.Helper()
	var got []api.Blocker
	for _, b := range out.Blockers {
		if b.Work == work {
			got = append(got, b)
		}
	}
	if len(got) != 1 {
		t.Fatalf("the check gives %s %d blocker(s): %+v; want one", work, len(got), got)
	}
	return got[0]
}

// wantKind checks a blocker's kind, its K-number, who acts and that it says
// since when.
func wantKind(t *testing.T, b api.Blocker, kind, actorKind, actor string) {
	t.Helper()
	if b.Kind != kind || b.Class != api.BlockerClass(kind) {
		t.Errorf("%s: %s %s (%s); want %s %s", b.Work, b.Class, b.Kind, b.Reason, api.BlockerClass(kind), kind)
	}
	if b.ActorKind != actorKind || !strings.Contains(b.Actor, actor) {
		t.Errorf("%s: acted on by %q (%s); want %q (%s)", b.Work, b.Actor, b.ActorKind, actor, actorKind)
	}
	if b.Since == "" {
		t.Errorf("%s: no since", b.Work)
	}
	if b.Reason == "" {
		t.Errorf("%s: no reason", b.Work)
	}
}

// reportBlocked pulls a to-do and reports it blocked, with a structured wait
// or none.
func reportBlocked(t *testing.T, ctx context.Context, id, summary string, wait *schedapi.HeldOn) {
	t.Helper()
	s := schedOf(t, ctx)
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull %s: %v %+v", id, err, got)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusBlocked, Summary: summary, By: session}, Wait: wait}); err != nil {
		t.Fatal(err)
	}
}

// personStep plans a person's step on a request: its intake reports blocked,
// adding it.
func personStep(t *testing.T, ctx context.Context, request, step, interaction string) {
	t.Helper()
	pullAndReport(t, ctx, stepOf(t, ctx, request, api.StepIntake).ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "a person's input is needed",
		More: map[string]any{"add": []map[string]any{{"step": step, "doer": "person", "interaction": interaction, "instructions": "Say which numbering."}}}})
}

func TestTheCheckSortsEachPieceOfWorkIntoItsKind(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	answer := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	personStep(t, ctx, answer.ID, "give-numbering", api.InteractionProvide)
	act := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	personStep(t, ctx, act.ID, "validate-rounding", api.InteractionValidate)
	ready := file(t, ctx, req, api.FileInput{Title: "Print the invoices", Module: "billing"})
	held := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing"})
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, held.ID, api.StepIntake).ID, By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull: %v %+v", err, got)
	}
	outside := file(t, ctx, req, api.FileInput{Title: "Show the invoice screen", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, outside.ID, api.StepIntake).ID, "its GUI check needs a runner",
		&schedapi.HeldOn{Kind: schedapi.HeldResource, Target: "gui-runner", Reason: "no GUI runner on the hub"})
	incident := file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, incident.ID, api.StepIntake).ID, "something is off", nil)

	revisions := func() map[string]int64 {
		got, err := schedOf(t, ctx).List(ctx, schedapi.ListInput{})
		if err != nil {
			t.Fatal(err)
		}
		r := map[string]int64{}
		for _, td := range got.Todos {
			r[td.ID] = td.Revision
		}
		return r
	}
	before := revisions()
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	if out.Scope != "census" {
		t.Errorf("scope %q; want census", out.Scope)
	}
	// The census is read-only: the stuck work it counts
	// is left as it was - no to-do written, none added or removed.
	if after := revisions(); !maps.Equal(before, after) {
		t.Errorf("the census changed the to-dos: %v before, %v after; want it read-only", before, after)
	}
	wantKind(t, blockerOf(t, out, answer.ID), api.BlockerAnswer, api.ActorPerson, "person")
	wantKind(t, blockerOf(t, out, act.ID), api.BlockerAct, api.ActorPerson, "person")
	wantKind(t, blockerOf(t, out, ready.ID), api.BlockerReady, api.ActorAgent, "agent")
	b := blockerOf(t, out, held.ID)
	wantKind(t, b, api.BlockerHeld, api.ActorAgent, another.ID)
	b = blockerOf(t, out, outside.ID)
	wantKind(t, b, api.BlockerOutside, api.ActorOutside, "gui-runner")
	if b.Reason != "no GUI runner on the hub" || b.On != "gui-runner" {
		t.Errorf("the outside wait: %q on %q; want its structured reason and target", b.Reason, b.On)
	}
	b = blockerOf(t, out, incident.ID)
	wantKind(t, b, api.BlockerIncident, api.ActorAgent, session.ID)
	if !strings.Contains(b.Reason, "unclassified") || !strings.Contains(b.Reason, "something is off") {
		t.Errorf("the blocked report with no kind: %q; want an unclassified wait with its summary", b.Reason)
	}
	if b.Cause != api.CauseUnclassified || b.Since == "" {
		t.Errorf("the blocked report with no kind: cause %q since %q; want unclassified with its age (since its report)", b.Cause, b.Since)
	}
	for i := 1; i < len(out.Blockers); i++ {
		if out.Blockers[i-1].Class > out.Blockers[i].Class {
			t.Fatalf("not sorted K1 first: %s before %s", out.Blockers[i-1].Class, out.Blockers[i].Class)
		}
	}
	for _, k := range api.BlockerKinds {
		if out.Counts[k] == 0 {
			t.Errorf("no %s counted: %v", k, out.Counts)
		}
	}
}

// A piece of work waiting on another names it and the chain to its root, and
// takes who acts from the root: here the worker holding the request it waits
// on.
func TestAWaitOnOtherWorkNamesItsChainAndItsRootsActor(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing", Kind: api.KindBuild})
	b := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing", Kind: api.KindBuild})
	c := file(t, ctx, req, api.FileInput{Title: "Mail the numbered invoices", Module: "billing", Kind: api.KindBuild})
	for _, d := range [][2]string{{b.ID, a.ID}, {c.ID, b.ID}} {
		if _, err := req.Depend(ctx, api.DependInput{ID: d[0], On: d[1]}); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []string{b.ID, c.ID} {
		pullAndReport(t, ctx, stepOf(t, ctx, r, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	}
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, a.ID, api.StepIntake).ID, By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull: %v %+v", err, got)
	}
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	got := blockerOf(t, out, c.ID)
	wantKind(t, got, api.BlockerHeld, api.ActorAgent, another.ID)
	if got.On != b.ID || len(got.Chain) < 2 || got.Chain[0] != b.ID || got.Chain[1] != a.ID {
		t.Errorf("the wait of %s: on %q, chain %v; want on %s, chain %s → %s", c.ID, got.On, got.Chain, b.ID, b.ID, a.ID)
	}
}

// The increment's check reads only its work, and writes nothing.
func TestTheIncrementsCheckReadsItsWorkAndWritesNothing(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	wantKind(t, blockerOf(t, check(t, ctx, api.PlanningCheckInput{Increment: in.ID}), f), api.BlockerReady, api.ActorAgent, "increment:"+in.ID)
	// Its build waits on work no increment holds: outside the increment.
	if _, err := req.Depend(ctx, api.DependInput{ID: f, On: other.ID}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	before := snapshot(t, ctx, cwm, f, other.ID, in.ID)

	out := check(t, ctx, api.PlanningCheckInput{Increment: in.ID})
	if out.Scope != in.ID {
		t.Errorf("scope %q; want %s", out.Scope, in.ID)
	}
	b := blockerOf(t, out, f)
	wantKind(t, b, api.BlockerOutside, api.ActorOutside, other.ID+", in no increment or release")
	if b.On != other.ID {
		t.Errorf("the outside wait is on %q; want %s", b.On, other.ID)
	}
	for _, b := range out.Blockers {
		if b.Work == other.ID {
			t.Errorf("the increment's check lists %s, which it does not hold", other.ID)
		}
	}
	if after := snapshot(t, ctx, cwm, f, other.ID, in.ID); after != before {
		t.Errorf("the check changed the stored state:\nbefore %s\nafter  %s", before, after)
	}
	pl, _ := application.UseService[api.PlanningService](ctx)
	for _, bad := range []api.PlanningCheckInput{{}, {Increment: in.ID, Census: true}} {
		if _, err := pl.Check(ctx, bad); err == nil {
			t.Errorf("a check of %+v: accepted; want one scope named", bad)
		}
	}
}

// snapshot is the stored state the check may not change: every to-do, open
// and done, and the objects named.
func snapshot(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, ids ...string) string {
	t.Helper()
	s := schedOf(t, ctx)
	open, err := s.List(ctx, schedapi.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	done, err := s.List(ctx, schedapi.ListInput{Done: true})
	if err != nil {
		t.Fatal(err)
	}
	var objs []cwmapi.Object
	for _, id := range ids {
		module, _ := cwmapi.ModuleOfObjectID(id)
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		objs = append(objs, got.Object)
	}
	raw, _ := json.Marshal([]any{open, done, objs})
	return string(raw)
}

// The release's check reads the work of the increments shipping in it.
func TestTheReleasesCheckReadsItsIncrementsWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	out := check(t, ctx, api.PlanningCheckInput{Release: r.ID})
	if out.Scope != r.ID {
		t.Errorf("scope %q; want %s", out.Scope, r.ID)
	}
	wantKind(t, blockerOf(t, out, f), api.BlockerReady, api.ActorAgent, "agent")
	for _, b := range out.Blockers {
		if b.Work == other.ID {
			t.Errorf("the release's check lists %s, which it does not ship", other.ID)
		}
	}
}
