// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A DROPPED REQUEST COUNTS NOWHERE (F-095): a request
// dropped in an increment is counted in neither the open nor the done ones -
// by the summary and by the page's head alike - and never listed among its
// open requests.
func TestAnIncrementsDroppedRequestReadsDropped(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers")
	open := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by date", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	gone := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by name", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: gone.ID, Outcome: api.PlanDropped, By: person, Note: "test"}); err != nil {
		t.Fatal(err)
	}

	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Requests != 1 || sum.RequestsDone != 0 {
		t.Errorf("the summary counts %d requests, %d done; want 1, 0 - the dropped one in neither: %+v", sum.Requests, sum.RequestsDone, sum)
	}
	tab := func(name string) string {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: name})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	if head := tab("open-requests"); !strings.Contains(head, `"data-done":0`) || !strings.Contains(head, `"data-to-do":1`) {
		t.Errorf("the page's head counts the dropped request: %s", head)
	}
	list := tab("open-request-list")
	if !strings.Contains(list, `"data-request":"`+open.ID+`"`) || strings.Contains(list, `"data-request":"`+gone.ID+`"`) {
		t.Errorf("the Open requests list: want %s and never the dropped %s: %s", open.ID, gone.ID, list)
	}
}
