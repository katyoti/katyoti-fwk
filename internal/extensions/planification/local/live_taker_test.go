// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A READY ROOT NAMES ITS LIVE TAKER OR NONE - K3 says
// whether a loop pulls the ready work: the worker of a recorded pull whose
// filter would take it (here one ended within the lease), else « no live
// taker » since the pull record started. A pull of another label is no taker.
func TestAReadyRootNamesItsLiveTakerOrNone(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	quiet := increment(t, ctx, "Invoices reach their customers by mail")
	looped := increment(t, ctx, "Invoices are printed")
	mail := want(t, ctx, cwm, "feature", "Mail the invoices")
	print := want(t, ctx, cwm, "feature", "Print the invoices")
	for f, in := range map[string]string{mail: quiet.ID, print: looped.ID} {
		accepted(t, ctx, cwm, f)
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in}); err != nil {
			t.Fatal(err)
		}
	}
	// A loop on the printing increment pulls before its work is ready: it
	// takes nothing, and stays its taker for the lease.
	looper := cwmapi.Actor{ID: "loop-on-print", Kind: "agent"}
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{By: looper, Filter: schedapi.Filter{Labels: []string{"increment:" + looped.ID}}}); err != nil || got.Todo != nil {
		t.Fatalf("the loop's pull: %v %+v; want nothing", err, got)
	}
	for _, f := range []string{mail, print} {
		if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
			t.Fatal(err)
		}
	}

	b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Increment: quiet.ID}), mail)
	wantKind(t, b, api.BlockerReady, api.ActorAgent, "an agent: a loop on increment:"+quiet.ID)
	if !strings.Contains(b.Reason, "any agent may take it now - no live taker since ") {
		t.Errorf("ready with only another label's pull: %q; want « no live taker since <t> »", b.Reason)
	}

	b = blockerOf(t, check(t, ctx, api.PlanningCheckInput{Increment: looped.ID}), print)
	wantKind(t, b, api.BlockerReady, api.ActorAgent, looper.ID+" (pulled ")
	if strings.Contains(b.Reason, "no live taker") {
		t.Errorf("ready with a live taker: %q; want no « no live taker »", b.Reason)
	}
}
