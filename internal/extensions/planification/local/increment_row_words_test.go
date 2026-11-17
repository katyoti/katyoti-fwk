// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT'S REQUEST LIST SAYS « TO DO » ONLY FOR WORK THAT CAN BE TAKEN
// (RQ-392): on its Open requests list
// a request with a step ready for an agent is « to do »
// with its step; one that waits on another open request says « waits on »
// and names it (a chip opening it); one with no step open and nothing awaited
// says « nothing left - it should end », never « ready »; in French too; the
// summary counts each state apart, and the increment is not listed among its
// own requests.
func TestAnIncrementsRequestRowsSayWhatEachOneWaitsOn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers")
	ready := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by date", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	waiting := file(t, ctx, req, api.FileInput{Title: "The invoice list sorts by customer", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := req.Depend(ctx, api.DependInput{ID: waiting.ID, On: ready.ID}); err != nil {
		t.Fatal(err)
	}
	// Its intake done, its next step waits on the request it depends on.
	s := schedOf(t, ctx)
	steps, err := s.List(ctx, schedapi.ListInput{Request: waiting.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range steps.Todos {
		if td.State == schedapi.StateReady {
			pullAndReport(t, ctx, td.ID, session, schedapi.Result{Summary: "assessed"})
			break
		}
	}
	idle := file(t, ctx, req, api.FileInput{Title: "The invoice list pages by fifty", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	// Its steps all gone, nothing ended it: nothing is left to do.
	todos, err := s.List(ctx, schedapi.ListInput{Request: idle.ID})
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
	if sum.ToDo != 1 || sum.WaitingOn != 1 || sum.Idle != 1 || sum.Requests != 3 {
		t.Errorf("the summary: to do %d, waiting on %d, idle %d of %d; want 1, 1, 1 of 3: %+v", sum.ToDo, sum.WaitingOn, sum.Idle, sum.Requests, sum)
	}

	def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "open-request-list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the Requests tab must display: %v", err)
	}
	b, _ := json.Marshal(def.DSLTree)
	raw := string(b)
	row := func(id string) string {
		i := strings.Index(raw, `"data-request":"`+id+`"`)
		if i < 0 {
			t.Fatalf("the Requests tab does not list %s: %s", id, raw)
		}
		j := strings.Index(raw[i+1:], `"data-request":"`)
		if j < 0 {
			return raw[i:]
		}
		return raw[i : i+1+j]
	}
	for _, c := range []struct {
		id, state string
		has       []string
	}{
		{ready.ID, "todo", []string{`"data-step-who":"intake"`}},
		{waiting.ID, "waiting-on", []string{`"data-step-who":"waits on ` + ready.ID + `"`, `"content":"waits on"`, `"data-open":"` + ready.ID + `"`}},
		{idle.ID, "idle", []string{`"data-step-who":"nothing left - it should end"`}},
	} {
		r := row(c.id)
		if !strings.Contains(r, `"data-state":"`+c.state+`"`) {
			t.Errorf("%s: its row's state is not %s: %s", c.id, c.state, r)
		}
		for _, h := range c.has {
			if !strings.Contains(r, h) {
				t.Errorf("%s: its row lacks %s: %s", c.id, h, r)
			}
		}
	}
	if strings.Contains(raw, `"data-request":"`+in.ID+`"`) {
		t.Error("the increment is listed among its own requests")
	}
	// Never « ready » for what cannot be taken.
	for _, id := range []string{waiting.ID, idle.ID} {
		if r := row(id); strings.Contains(r, `"data-step-who":"ready"`) || strings.Contains(r, `"content":"ready"`) {
			t.Errorf("%s reads ready though nothing of it can be taken: %s", id, r)
		}
	}
	// In French.
	fr, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "open-request-list", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := json.Marshal(fr.DSLTree)
	for _, want := range []string{`"data-step-who":"attend ` + ready.ID + `"`, `"content":"attend"`, "plus rien à faire - elle devrait se terminer"} {
		if !strings.Contains(string(fb), want) {
			t.Errorf("the French list lacks %s: %s", want, fb)
		}
	}
}
