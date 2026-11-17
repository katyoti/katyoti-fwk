// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A SPLIT DEMAND ENDS WITH ITS PARTS: its own
// deliverables are carried by its parts and never checked on it, so once every
// part has ended it ends as they did — done when one is done, dropped when all
// were — though none of its own deliverables is verified. It used to stay
// split for ever, its end refused for them and the refusal ignored, holding
// its increment open.
func TestASplitDemandEndsWithItsPartsWhateverItsOwnDeliverables(t *testing.T) {
	t.Parallel()
	ctx, _, req, _, _ := planWorld(t)
	plans := plansOf(t, ctx)
	state := func(id string) string {
		t.Helper()
		g, err := req.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return g.Request.State
	}
	splitInTwo := func(title string) (string, []string) {
		t.Helper()
		r := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", By: person,
			Deliverables: []api.Deliverable{{Title: "every invoice is mailed"}, {Title: "the parcel carries it"}}})
		got, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Parts: []api.SplitPart{{Module: "billing"}, {Module: "shipping"}}})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID, []string{got.Parts[0].ID, got.Parts[1].ID}
	}

	done, parts := splitInTwo("Send invoices by mail")
	if _, err := plans.Finish(ctx, api.FinishInput{ID: parts[0], By: person}); err != nil {
		t.Fatal(err)
	}
	if s := state(done); s != api.PlanSplit {
		t.Fatalf("with one part still open the demand is %s, want split", s)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: parts[1], Outcome: api.PlanDropped, By: person}); err != nil {
		t.Fatal(err)
	}
	if s := state(done); s != api.PlanDone {
		t.Fatalf("its parts ended (one done), its own deliverables unverified: the demand is %s, want done", s)
	}

	dropped, parts := splitInTwo("Send invoices by fax")
	for _, p := range parts {
		if _, err := plans.Finish(ctx, api.FinishInput{ID: p, Outcome: api.PlanDropped, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	if s := state(dropped); s != api.PlanDropped {
		t.Fatalf("its parts all dropped: the demand is %s, want dropped", s)
	}
}
