// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A LIVE WALK RUNS ONLY IN ITS BATCH: on a request
// that is not a GUI batch, a deliverable whose check is a live GUI walk
// (tests/live/*.sh) is never run - neither by requests check nor by the end
// gate - and is answered as counted by the final GUI batch; its Go check runs.
// The batch request (every checked deliverable a live walk) does not run them
// either: scripts/gui-batch.sh does, recording the
// verdicts the batch counts.
// The walk's check here passes whenever it runs: verified means it ran.
const liveWalkCheck = "test -n app/ktw-gui/frontend/tests/live/invoice-mark.sh"

func TestALiveWalkIsCountedByTheBatchNotRunByCheck(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Mark the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}, {Title: "the walk passes", Check: liveWalkCheck}}})
	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || !got.Results[0].Passed {
		t.Fatalf("results %+v; want the Go check run and passed", got.Results)
	}
	if walk := got.Results[1]; walk.Passed || !strings.Contains(walk.Error, "counted by the final GUI batch") {
		t.Fatalf("the live walk: %+v; want it not run, counted by the final GUI batch", walk)
	}
	if got.Request.Deliverables[1].Verified {
		t.Fatal("the live walk was run by requests check on a request that is not the GUI batch")
	}

	// The batch request runs no walk: scripts/gui-batch.sh does.
	b := file(t, ctx, req, api.FileInput{Title: "The final GUI batch", Module: "billing", Kind: api.KindReview, By: person,
		Deliverables: []api.Deliverable{{Title: "invoice-mark", Check: liveWalkCheck}}})
	got, err = req.Check(ctx, api.CheckInput{ID: b.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].Passed || got.Request.Deliverables[0].Verified || !strings.Contains(got.Results[0].Error, "scripts/gui-batch.sh") {
		t.Fatalf("the batch's walk: %+v; want it not run, left to scripts/gui-batch.sh", got.Results)
	}
}

func TestTheEndGateLeavesALiveWalkToTheBatch(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Mark the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}, {Title: "the walk passes", Check: liveWalkCheck}}})
	workFix(t, ctx, r.ID)
	if st := planState(t, ctx, r.ID); st == api.PlanDone {
		t.Fatal("the request ended: its end gate ran the live walk")
	}
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := cwmapi.DecodeCapability[api.RequestFields](o.Object, api.CapRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Deliverables[0].Verified || f.Deliverables[1].Verified {
		t.Fatalf("deliverables %+v; want the Go check verified at the end, the live walk not run", f.Deliverables)
	}
}
