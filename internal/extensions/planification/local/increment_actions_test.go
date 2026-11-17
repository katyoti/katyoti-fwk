// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT IS ACTED ON FROM ITS PANE (feature): each
// act offered only where it applies - open, its release select (plan-for) and
// « Drop it » (dangerous); shipped in a planned release, « Take out »;
// everything it holds done, « Mark it done » once its delivery review passed,
// « Force it done » with a note before; dropped, nothing. The pane names
// no command to type.
func TestAnIncrementIsActedOnFromItsPage(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	in := increment(t, ctx, "Invoices by mail").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: in, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	page := func() string {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in, Tab: "page"})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the increment's pane must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	offers := func(stage, raw string, want, not []string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(raw, w) {
				t.Errorf("%s: the page does not offer %s", stage, w)
			}
		}
		for _, n := range not {
			if strings.Contains(raw, n) {
				t.Errorf("%s: the page offers %s", stage, n)
			}
		}
	}
	ship, unship := `"uri":"ktw-agents:increments/plan-for"`, `"uri":"ktw-agents:increments/unship"`
	done, drop := `"uri":"ktw-agents:increments/done"`, `"uri":"ktw-agents:increments/drop"`

	raw := page()
	offers("in no release, holding nothing", raw, []string{ship, drop, `"dangerous":true`}, []string{unship, done})
	if strings.Contains(raw, "(increments ") {
		t.Error("the page tells the person to type an increments command")
	}

	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in}); err != nil {
		t.Fatal(err)
	}
	offers("shipped in a planned release", page(), []string{unship, `"label":"Take out"`, drop}, []string{done})

	uc := usecase(t, ctx, cwm, "Mail the invoice")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in}); err != nil {
		t.Fatal(err)
	}
	offers("holding a use case not done", page(), []string{unship, drop}, []string{done})
	for _, to := range []string{"proposed", "accepted", "done"} {
		if err := move(ctx, cwm, uc, to, person); err != nil {
			t.Fatal(err)
		}
	}
	// Its delivery review has not run: usable-delivered refuses the done, so
	// the pane offers no plain « Mark it done » but « Force it done », naming
	// why, asking the note and passing the gate with it.
	forced := []string{done, `"id":"force-done"`, `"force":true`, `"note":"${value}"`, `"prompt":"Its delivery is not proven usable (its delivery review has not run`, unship, drop}
	offers("everything it holds done, its delivery unreviewed", page(), forced, []string{`"id":"mark-done"`})
	passReview(t, ctx, cwm, in)
	offers("everything it holds done, its delivery review passed", page(), []string{done, `"id":"mark-done"`, `"by":"${person}"`, unship, drop}, []string{`"id":"force-done"`, `"force":true`})

	if _, err := inc.Untarget(ctx, api.UntargetInput{ID: uc}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: next.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	offers("its release frozen", page(), []string{drop}, []string{unship})

	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: in, By: person}); err != nil {
		t.Fatal(err)
	}
	offers("dropped", page(), nil, []string{ship, unship, done, drop})
}

// passReview records a passed delivery review on an increment, as the
// delivery-review step leaves it.
func passReview(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	f, _, err := cwmapi.DecodeCapability[api.ReleaseFields](o, api.CapRelease)
	if err != nil {
		t.Fatal(err)
	}
	f.DeliveryReview = &api.DeliveryReview{Verdict: api.VerdictApproved, Demo: demoPath}
	if err := o.SetCapability(api.CapRelease, f); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
}
