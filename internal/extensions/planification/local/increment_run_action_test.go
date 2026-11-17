// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT NOT YET RUN OFFERS « RUN » - its pane's
// head, its Open increments card and its Delivery short panel offer « Run »
// (increments/run as the person) exactly when the verb would run it: an open
// increment with delivery goals, not yet run, not on hold. Once run it offers
// Run in Claude Code instead; held, with no goals or dropped, neither Run.
func TestAnIncrementNotYetRunOffersRun(t *testing.T) {
	t.Parallel()
	w, ctx, _, _, inc := newDeliveryWorld(t)
	const run = `"data-control":"run-increment"`
	const verb = `"uri":"ktw-agents:increments/run"`
	pane := func(id string) string {
		t.Helper()
		return screenJSON(t, "the pane of "+id)(inc.PanePage(ctx, api.IncrementPaneInput{ID: id, Tab: "page"}))
	}
	panel := func(id string) string {
		t.Helper()
		return screenJSON(t, "the short panel of "+id)(inc.PanelPage(ctx, api.IncrementPanelInput{ID: id}))
	}
	card := func(id string) string {
		t.Helper()
		return screenJSON(t, "the open card of "+id)(inc.OpenPage(ctx, api.OpenIncrementsInput{Stream: "all", Held: "all", Shown: 25, Search: id}))
	}
	refused := func(id string) bool {
		t.Helper()
		_, err := inc.Run(ctx, api.IncrementRunInput{ID: id, By: person})
		return err != nil
	}

	// Defined, with its goals: Run on the pane and the panel, the person's
	// go; no Claude Code session yet - nothing is planned for it.
	if p := pane(w.planned); !strings.Contains(p, run) || !strings.Contains(p, verb) || !strings.Contains(p, `"by":"${person}"`) || !strings.Contains(p, `"label":"Run"`) {
		t.Errorf("a defined increment with goals offers no Run (increments/run as the person) on its pane: %s", p)
	} else if strings.Contains(p, "Run in Claude Code") {
		t.Errorf("a defined increment offers Run in Claude Code before it is run: %s", p)
	}
	if p := panel(w.planned); !strings.Contains(p, run) || !strings.Contains(p, verb) {
		t.Errorf("a defined increment's short panel offers no Run: %s", p)
	}
	if fr := screenJSON(t, "the French pane")(inc.PanePage(ctx, api.IncrementPaneInput{ID: w.planned, Tab: "page", Language: "fr"})); !strings.Contains(fr, `"label":"Lancer"`) {
		t.Errorf("in French Run is not « Lancer »: %s", fr)
	}

	// In no release, on the Open increments list: its card offers Run.
	free, err := inc.Create(ctx, api.CreateIncrementInput{Stream: w.stream, Title: "Export invoices", Goals: []string{"I export invoices"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if c := card(free.Increment.ID); !strings.Contains(c, free.Increment.ID) || !strings.Contains(c, run) || !strings.Contains(c, verb) {
		t.Errorf("the Open increments card of a defined increment offers no Run: %s", c)
	}

	// No delivery goals: the verb refuses, no Run anywhere.
	bare, err := inc.Create(ctx, api.CreateIncrementInput{Stream: w.stream, Title: "Bare invoices", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if p, c := pane(bare.Increment.ID), card(bare.Increment.ID); strings.Contains(p, run) || strings.Contains(c, run) || !refused(bare.Increment.ID) {
		t.Errorf("an increment with no delivery goals offers Run or is run: pane %v, card %v", strings.Contains(p, run), strings.Contains(c, run))
	}

	// On hold: the verb refuses, no Run.
	if p, pn := pane(w.held), panel(w.held); strings.Contains(p, run) || strings.Contains(pn, run) || !refused(w.held) {
		t.Errorf("a held increment offers Run or is run: pane %v, panel %v", strings.Contains(p, run), strings.Contains(pn, run))
	}

	// Once run: no Run, its Claude Code session offered.
	if refused(w.planned) {
		t.Fatalf("the defined increment %s is not run", w.planned)
	}
	if p := pane(w.planned); strings.Contains(p, run) || !strings.Contains(p, `"label":"Run in Claude Code"`) {
		t.Errorf("a run increment's pane: Run %v, Run in Claude Code %v; want Run gone and Run in Claude Code offered", strings.Contains(p, run), strings.Contains(p, "Run in Claude Code"))
	}
	if p := panel(w.planned); strings.Contains(p, run) {
		t.Errorf("a run increment's short panel still offers Run")
	}

	// Dropped: ended, no Run.
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: free.Increment.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if p := pane(free.Increment.ID); strings.Contains(p, run) || !refused(free.Increment.ID) {
		t.Errorf("a dropped increment offers Run or is run: %s", p)
	}
}
