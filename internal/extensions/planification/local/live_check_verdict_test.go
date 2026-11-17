// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A LIVE CHECK COUNTS BY ITS CRITERION'S VERDICT: on a
// request that is not a GUI batch, a tests/live deliverable whose check is the
// probe of a criterion whose recorded verdict passed is verified from that
// verdict, never run; with a failed verdict or none, a GUI walk still waits on
// the batch. A tests/live script driving no GUI is a CLI probe: run like any
// check. The checks below fail whenever they run (the scripts do not exist):
// verified means counted from the verdict.
const (
	walkNeverRun  = "bash app/ktw-gui/frontend/tests/live/no-such-invoice-walk.sh"
	probeNeverRun = "bash tests/live/no-such-billing-probe.sh"
	probeRuns     = "test -n tests/live/billing-probe.sh" // passes when run
)

// verdictOf records the verdict of a criterion with the status given.
func verdictOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, criterion, status string) {
	t.Helper()
	_, local, _ := strings.Cut(criterion, ".")
	v := cwmapi.Object{ID: "billing.verdict-" + local, Type: "verdict", Parent: criterion}
	v.Name = "Verdict"
	_ = v.SetCapability("verdict", map[string]any{"status": status, "kind": "command"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: v}); err != nil {
		t.Fatal(err)
	}
}

func TestALiveCheckCountsByItsCriterionsPassedVerdict(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice is marked")
	walk := criterionFor(t, ctx, cwm, "billing", f, walkNeverRun)
	probe := criterionFor(t, ctx, cwm, "billing", f, probeNeverRun)
	failed := criterionFor(t, ctx, cwm, "billing", f, "bash app/ktw-gui/frontend/tests/live/no-such-invoice-print.sh")
	verdictOf(t, ctx, cwm, walk, "pass")
	verdictOf(t, ctx, cwm, probe, "pass")
	verdictOf(t, ctx, cwm, failed, "fail")

	r := file(t, ctx, req, api.FileInput{Title: "Mark the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{
			{Title: "it builds", Check: "go version"},
			{Title: "the walk passed", Check: walkNeverRun},
			{Title: "the probe passed", Check: probeNeverRun},
			{Title: "the print walk failed", Check: "bash app/ktw-gui/frontend/tests/live/no-such-invoice-print.sh"},
		}})
	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 4 || !got.Results[0].Passed {
		t.Fatalf("results %+v; want the Go check run and passed", got.Results)
	}
	for i, crit := range map[int]string{1: walk, 2: probe} {
		d := got.Request.Deliverables[i]
		if !got.Results[i].Passed || !d.Verified || !strings.Contains(d.Evidence, crit) {
			t.Fatalf("deliverable %d: %+v (result %+v); want it verified from the passed verdict of %s, not run", i, d, got.Results[i], crit)
		}
	}
	if res := got.Results[3]; res.Passed || got.Request.Deliverables[3].Verified || !strings.Contains(res.Error, "counted by the final GUI batch") {
		t.Fatalf("the walk whose verdict failed: %+v; want it left to the batch", res)
	}
}

func TestACLIProbeUnderTestsLiveIsRunByCheck(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Probe the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}, {Title: "the probe passes", Check: probeRuns}}})
	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || !got.Results[1].Passed || !got.Request.Deliverables[1].Verified {
		t.Fatalf("results %+v; want the CLI probe (no GUI) run and verified, not left to the GUI batch", got.Results)
	}
}

func TestTheEndGateCountsALiveWalkByItsPassedVerdict(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice is marked")
	verdictOf(t, ctx, cwm, criterionFor(t, ctx, cwm, "billing", f, walkNeverRun), "pass")
	r := file(t, ctx, req, api.FileInput{Title: "Mark the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}, {Title: "the walk passed", Check: walkNeverRun}}})
	workFix(t, ctx, r.ID)
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("plan %q; want done: the live walk counts by its criterion's passed verdict", st)
	}
}

// verdictAt records a criterion's verdict with the status and time given.
func verdictAt(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, criterion, status, ranAt string) {
	t.Helper()
	_, local, _ := strings.Cut(criterion, ".")
	v := cwmapi.Object{ID: "billing.verdict-" + local, Type: "verdict", Parent: criterion}
	v.Name = "Verdict"
	_ = v.SetCapability("verdict", map[string]any{"status": status, "kind": "command", "ran_at": ranAt})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: v}); err != nil {
		t.Fatal(err)
	}
}

// A GUI BATCH REQUEST COUNTS WHAT ITS BATCH RECORDED (
// RQ-1153): checking it counts each walk whose criterion has a pass recorded
// since the request was filed, and never runs a walk - a pass older than the
// request does not count, a walk with none stays unverified for
// scripts/gui-batch.sh. The unrun walk would pass if it were run.
func TestAGUIBatchRequestCountsThePassesRecordedSinceItWasFiled(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice is marked")
	const (
		fresh = "bash app/ktw-gui/frontend/tests/live/no-such-fresh-walk.sh"
		stale = "bash app/ktw-gui/frontend/tests/live/no-such-stale-walk.sh"
		none  = "test -n app/ktw-gui/frontend/tests/live/no-such-unrun-walk.sh" // passes when run
	)
	cFresh := criterionFor(t, ctx, cwm, "billing", f, fresh)
	cStale := criterionFor(t, ctx, cwm, "billing", f, stale)
	criterionFor(t, ctx, cwm, "billing", f, none)
	verdictAt(t, ctx, cwm, cStale, "pass", "2000-01-01T00:00:00Z")
	r := file(t, ctx, req, api.FileInput{Title: "Run the GUI batch", Module: "billing", Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "fresh", Check: fresh}, {Title: "stale", Check: stale}, {Title: "none", Check: none}}})
	verdictAt(t, ctx, cwm, cFresh, "pass", time.Now().UTC().Add(time.Minute).Format(time.RFC3339))
	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 3 {
		t.Fatalf("results %+v; want three", got.Results)
	}
	if d := got.Request.Deliverables[0]; !got.Results[0].Passed || !d.Verified || !strings.Contains(d.Evidence, cFresh) {
		t.Fatalf("the walk passed since filing: %+v (%+v); want it counted from %s, not run", d, got.Results[0], cFresh)
	}
	for i := 1; i < 3; i++ {
		if got.Results[i].Passed || got.Request.Deliverables[i].Verified || !strings.Contains(got.Results[i].Error, "scripts/gui-batch.sh") {
			t.Fatalf("deliverable %d: %+v; want it not run, left to scripts/gui-batch.sh: no pass recorded since the request was filed", i, got.Results[i])
		}
	}
}
