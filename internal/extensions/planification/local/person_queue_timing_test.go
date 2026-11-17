// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// « À faire » on a queue of some eighty items - the smoke case holds 82 -
// answers its open and each click (a tile, a kind chip) within its budget
// (serving: the first card and
// each click under two seconds from the person's gesture, the page's server
// read a part of it).
func TestPersonQueuePageOnEightyItemsAnswersWithinItsBudget(t *testing.T) {
	long.Test(t)
	ctx, cwm, req := world(t)
	const asked, decisions = 60, 20
	for i := 0; i < asked; i++ {
		r := file(t, ctx, req, api.FileInput{Title: fmt.Sprintf("Request %02d", i), Module: "billing", Kind: api.KindBuild})
		if _, err := req.Ask(ctx, api.AskInput{ID: r.ID, Question: "Which one?", By: intake}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < decisions; i++ {
		openDecision(t, ctx, cwm, fmt.Sprintf("Decision %02d", i), true)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	page := func(name string, in api.PersonQueueInput) time.Duration {
		t.Helper()
		in.Language = "fr"
		start := time.Now()
		def, err := views.PersonQueuePage(ctx, in)
		took := time.Since(start)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		data, _ := def.Data.(map[string]any)
		if n, _ := data["count"].(int); n < asked+decisions {
			t.Fatalf("%s: the queue holds %v items, want at least %d", name, data["count"], asked+decisions)
		}
		t.Logf("%s: %v", name, took.Round(time.Millisecond))
		return took
	}
	page("the open, cold", api.PersonQueueInput{})
	// The open, warm: a re-read of the same view computes the queue again.
	open := page("the open", api.PersonQueueInput{})
	if open > personQueuePageBudget {
		t.Errorf("the open took %v; want under %v", open.Round(time.Millisecond), personQueuePageBudget)
	}
	// Each click shows another part of the queue just read: it costs a
	// fraction of the open, never the queue computed again.
	clickBudget := max(open/2, 25*time.Millisecond)
	for _, v := range []struct {
		name string
		in   api.PersonQueueInput
	}{
		{"a click on a tile", api.PersonQueueInput{Action: api.ActionAnswer}},
		{"a click on « Tout »", api.PersonQueueInput{}},
		{"a click on another tile", api.PersonQueueInput{Action: api.ActionDecide}},
		{"a click on a kind chip", api.PersonQueueInput{Kind: "overdue"}},
	} {
		if took := page(v.name, v.in); took > clickBudget {
			t.Errorf("%s took %v; want under %v (half the open: the queue is not computed again)", v.name, took.Round(time.Millisecond), clickBudget)
		}
	}
}

// personQueuePageBudget is the page's server read, warm: well under the two
// seconds the person waits at most, the rest being the bridge and the drawing.
const personQueuePageBudget = 500 * time.Millisecond
