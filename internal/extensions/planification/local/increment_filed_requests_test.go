// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT'S PANE SHOWS EVERY REQUEST FILED IN IT:
// a request filed straight into a running increment (requests file
// --increment) serves none of its delivery goals; the pane's Open requests
// tab lists it beside the request serving one, and its head counts what the
// increment's summary counts - a dropped one neither listed nor counted.
func TestAnIncrementsPaneShowsEveryRequestFiledInIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers")
	if _, err := inc.SetGoals(ctx, api.SetGoalsInput{ID: in.ID, Goals: []string{"A customer gets the invoice the day it is made"}, By: person}); err != nil {
		t.Fatal(err)
	}
	uc := usecase(t, ctx, cwm, "Mail the invoice")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	// Run: work is filed into it only once it runs (rule).
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	serving := file(t, ctx, req, api.FileInput{Title: "One sender for every channel", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: serving.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	polish := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by date", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	gone := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by name", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: gone.ID, Outcome: api.PlanDropped, By: person, Note: "test"}); err != nil {
		t.Fatal(err)
	}

	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	total := sum.Requests
	if n := sum.Awaiting + sum.Running + sum.ToDo + sum.WaitingOn + sum.Idle + sum.Blocked + sum.RequestsDone; n != total {
		t.Errorf("the summary's request total %d is not the sum of its buckets %d", total, n)
	}
	if total < 2 {
		t.Fatalf("the summary counts %d requests, want the serving one and the filed one at least: %+v", total, sum)
	}
	tab := func(name string) string {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the %s tab must display: %v", name, err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	// The head counts what the summary counts; the Open requests list holds
	// the filed request beside the serving one, never the dropped one.
	head := tab("open-requests")
	var counts struct {
		Done    int `json:"data-done"`
		Running int `json:"data-running"`
		OnYou   int `json:"data-on-you"`
		ToDo    int `json:"data-to-do"`
	}
	at := strings.Index(head, `"data-view":"increment-page-counts"`)
	if at < 0 {
		t.Fatalf("the Open requests tab has no head counts: %s", head)
	}
	start := strings.LastIndex(head[:at], "{")
	end := strings.Index(head[at:], "}") + at + 1
	if err := json.Unmarshal([]byte(head[start:end]), &counts); err != nil {
		t.Fatalf("the head counts %s: %v", head[start:end], err)
	}
	if n := counts.Done + counts.Running + counts.OnYou + counts.ToDo; n != total {
		t.Errorf("the head counts %d requests (%+v); the summary %d", n, counts, total)
	}
	list := tab("open-request-list")
	for _, r := range []api.Request{serving, polish} {
		if !strings.Contains(list, `"data-request":"`+r.ID+`"`) {
			t.Errorf("the Open requests list does not hold %s %q: %s", r.ID, r.Title, list)
		}
	}
	if strings.Contains(list, `"data-request":"`+gone.ID+`"`) {
		t.Errorf("the Open requests list holds the dropped %s", gone.ID)
	}
}
