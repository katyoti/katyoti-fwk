// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT A PERSON IS ASKED SAYS WHAT IS EXPECTED — the human task names the
// answer expected in plain words, never the step's name twice; a validation's
// card says what the work before it did; a decision's card lists the points
// raised against its proposal.
func TestWhatAPersonIsAskedSaysWhatIsExpected(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "seen on paper", Check: "go version"}}})
	toReview(t, ctx, r.ID)
	want := "Validate the result: Print invoices (step review)"
	if ask := stepOf(t, ctx, r.ID, "review").Packet.Ask; ask == nil || ask.Title != want {
		t.Errorf("the review asks %+v; want %q", ask, want)
	}
	items := itemsOf(personQueue(t, ctx, ""), r.ID)
	i := slices.IndexFunc(items, func(it api.PersonItem) bool { return it.Action == api.ActionValidate })
	if i < 0 || !strings.Contains(items[i].Done, "module-update") || !strings.Contains(items[i].Done, "module-update done") {
		t.Fatalf("the validation's card: %+v; want what the work before it did", items)
	}

	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: "Keep the totals"}
	_ = o.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"},
		"proposal":  map[string]any{"option": "keep", "why": "it costs nothing"},
		"challenge": map[string]any{"by": "ana", "at": "2026-10-09", "points": []string{"the ledger rounds differently"}}})
	d, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatal(err)
	}
	dec := itemsOf(personQueue(t, ctx, ""), d.Object.ID)
	if len(dec) != 1 || !slices.Contains(dec[0].Challenged, "the ledger rounds differently — ana") {
		t.Fatalf("the decision's card: %+v; want the point raised against its proposal", dec)
	}
	page := queuePage(t, ctx)
	for _, w := range []string{"Ce qui a été fait", "module-update done", "Contesté", "the ledger rounds differently"} {
		if !strings.Contains(page, w) {
			t.Errorf("the page lacks %s", w)
		}
	}
}
