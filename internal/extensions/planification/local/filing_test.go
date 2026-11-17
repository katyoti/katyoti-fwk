// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT A PERSON ASKS IS A DEMAND — filed with no kind by a person, a request is
// a demand; a module's ask keeps intake's kind; an explicit kind (direct work:
// a fix) is kept. The views separate the demands from the modules' work.
func TestAPersonsAskIsADemandAndTheViewsSeparateIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	asked := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", By: person})
	if asked.Kind != api.KindDemand {
		t.Errorf("a person's ask with no kind: %q; want demand", asked.Kind)
	}
	fix := file(t, ctx, req, api.FileInput{Title: "Fix the label", Module: "billing", Kind: api.KindFix, By: person})
	if fix.Kind != api.KindFix {
		t.Errorf("a person's fix: %q; want fix kept (direct work)", fix.Kind)
	}
	work := file(t, ctx, req, api.FileInput{Title: "Expose the totals", Module: "billing", By: cwmapi.Actor{ID: "billing", Kind: "module"}})
	if work.Kind == api.KindDemand {
		t.Errorf("a module's ask with no kind: %q; want it left to intake, never a demand", work.Kind)
	}

	ids := func(split string) []string {
		t.Helper()
		out, err := req.Demands(ctx, api.DemandsInput{Split: split})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, d := range out.Demands {
			got = append(got, d.ID)
		}
		return got
	}
	if got := ids("demands"); !slices.Equal(got, []string{asked.ID}) {
		t.Errorf("demands: %v; want only %s", got, asked.ID)
	}
	if got := ids("work"); !slices.Contains(got, fix.ID) || !slices.Contains(got, work.ID) || slices.Contains(got, asked.ID) {
		t.Errorf("work: %v; want %s and %s, not the demand", got, fix.ID, work.ID)
	}
	if _, err := req.Demands(ctx, api.DemandsInput{Split: "everything"}); err == nil {
		t.Error("Demands of an unknown split answered; want it refused")
	}

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	d, w := strings.Index(page, "Demands — 1"), strings.Index(page, "Module work — 2")
	if d < 0 || w < 0 {
		t.Fatalf("the deck's Requests tab has no « Demands — 1 » and « Module work — 2 »")
	}
	if w < d {
		t.Fatal("« Module work » comes before « Demands »")
	}
	demands := page[d:w]
	if !strings.Contains(demands, `"title":"`+asked.Title) || strings.Contains(demands, `"title":"`+fix.Title) || strings.Contains(demands, `"title":"`+work.Title) {
		t.Errorf("« Demands » lists %s only: %s", asked.ID, demands)
	}
	if rest := page[w:]; !strings.Contains(rest, `"title":"`+fix.Title) || !strings.Contains(rest, `"title":"`+work.Title) {
		t.Error("« Module work » does not list the fix and the module's ask")
	}
}
