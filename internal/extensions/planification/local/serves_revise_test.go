// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// An intake assesses a request and names the want it serves in one call, then
// records its deliverables and ends its step: the serves edge outlives both.
func TestTheWantARequestServesOutlivesItsRevision(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")
	out, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Kind: api.KindBuild, Priority: "P1", Depth: api.DepthFull, Serves: uc, By: intake})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Request.Serves, []string{uc}) {
		t.Fatalf("assessed: serves %v; want %s", out.Request.Serves, uc)
	}
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: intake, Note: "its deliverables",
		Deliverables: []api.Deliverable{{Title: "a mail is sent"}}}); err != nil {
		t.Fatal(err)
	}
	// Its intake step ends: the plan expands its template.
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "assessed"})
	got, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Request.Serves, []string{uc}) {
		t.Fatalf("after its revision: serves %v; want %s", got.Request.Serves, uc)
	}
}
