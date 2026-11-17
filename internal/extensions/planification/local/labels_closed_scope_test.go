// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A REQUEST'S TO-DOS NEVER TAKE A CLOSED OR HELD SCOPE'S LABELS - a part
// split from an open increment's work, made to serve a feature a done
// increment took, keeps its origin's increment and release; made to serve a
// feature a held increment takes, it keeps them too; a request with no origin
// serving the held feature stays the held increment's work, held
func TestARequestKeepsItsOpenScopeWhenServingAClosedOrHeldOne(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	live := increment(t, ctx, "Invoices reach their customers by mail")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: live.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	origin := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing", Kind: api.KindFix, Increment: live.ID})

	// A done increment that took a feature.
	gone := increment(t, ctx, "Invoices are printed")
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: gone.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	printed := want(t, ctx, cwm, "feature", "Print the invoices")
	taken(t, ctx, cwm, gone.ID, printed)
	// A held increment that takes another.
	held := increment(t, ctx, "Reminders are mailed")
	reminded := want(t, ctx, cwm, "feature", "Remind late payers")
	taken(t, ctx, cwm, held.ID, reminded)
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: held.ID, Reason: "the mail provider is not chosen", By: person}); err != nil {
		t.Fatal(err)
	}

	part := func(title, serves string) string {
		p := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindFix, By: person})
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: p.ID, To: origin.ID, Kind: cwmapi.EdgeType(api.EdgeSplitFrom)}); err != nil {
			t.Fatal(err)
		}
		if _, err := req.Assess(ctx, api.AssessInput{ID: p.ID, Serves: serves, By: intake}); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	for _, c := range []struct{ id, scope string }{
		{part("Print through the mail house", printed), gone.ID},
		{part("Remind through the mail house", reminded), held.ID},
	} {
		l := stepOf(t, ctx, c.id, api.StepIntake).Labels
		if slices.Contains(l, "increment:"+c.scope) {
			t.Errorf("%s serving a feature of %s: labels %v; never that scope's", c.id, c.scope, l)
		}
		if !slices.Contains(l, "increment:"+live.ID) || !slices.Contains(l, "release:"+a.ID) {
			t.Errorf("%s split from %s's work: labels %v; want increment:%s and release:%s", c.id, live.ID, l, live.ID, a.ID)
		}
	}

	// No origin: serving the held feature, it is the held increment's work.
	lone := file(t, ctx, req, api.FileInput{Title: "Remind by text message", Module: "billing", Kind: api.KindFix, By: person})
	if _, err := req.Assess(ctx, api.AssessInput{ID: lone.ID, Serves: reminded, By: intake}); err != nil {
		t.Fatal(err)
	}
	if l := stepOf(t, ctx, lone.ID, api.StepIntake).Labels; !slices.Contains(l, "increment:"+held.ID) {
		t.Errorf("serving the held feature with no origin: labels %v; want increment:%s (held with it)", l, held.ID)
	}
}
