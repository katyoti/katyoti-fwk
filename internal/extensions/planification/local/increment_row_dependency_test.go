// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A ROW WAITS ON WHAT THE SCHEDULER WAITS ON: an
// increment's row says « waiting on <id> » by the scheduler's own rule
// (dependencyPassed): a request depending on a feature a person already moved
// done in its own lifecycle - its build's plan still open - waits on nothing; with no step left it says
// « nothing left », never « waiting on » that feature.
func TestAnIncrementRowWaitsOnlyOnWhatTheSchedulerWaitsOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers")
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"realized", api.StateDone} {
		if err := move(ctx, cwm, f, to, person); err != nil {
			t.Fatal(err)
		}
	}
	r := file(t, ctx, req, api.FileInput{Title: "The invoice list pages by fifty", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := req.Depend(ctx, api.DependInput{ID: r.ID, On: f}); err != nil {
		t.Fatal(err)
	}
	s := schedOf(t, ctx)
	todos, err := s.List(ctx, schedapi.ListInput{Request: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range todos.Todos {
		if _, err := s.Archive(ctx, schedapi.ArchiveInput{ID: td.ID, Note: "test: its steps gone"}); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sum.WaitingOn != 0 || sum.Idle != 1 {
		t.Errorf("the summary: waiting on %d, idle %d; want 0 and 1 - the feature it depends on is done: %+v", sum.WaitingOn, sum.Idle, sum)
	}
	def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "open-request-list"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	if raw := string(b); strings.Contains(raw, "waiting on "+f) || !strings.Contains(raw, `"data-state":"idle"`) {
		t.Errorf("its row waits on the done feature %s, or is not « nothing left »: %s", f, raw)
	}
}
