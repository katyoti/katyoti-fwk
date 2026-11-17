// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A DROPPED RELEASE KEEPS NO INCREMENT — dropping it takes each increment
// shipping in it out, to wait in its stream's backlog for the next cut; one
// can also be taken out alone (unship). The project's streams live in
// sys-planification, beside its releases.
func TestDroppingAReleaseFreesItsIncrements(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	rel, inc, sm := releases(t, ctx), increments(t, ctx), streams(t, ctx)
	project := plan(t, ctx, rel, api.PlanInput{Version: "2027.05"}) // sys-planification
	katyoti, err := sm.Create(ctx, api.CreateStreamInput{Module: api.SystemModule, Name: "KaTyoti",
		StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d"}})
	if err != nil {
		t.Fatalf("a stream in %s: %v", api.SystemModule, err)
	}
	a := increment(t, ctx, "Invoices by mail")
	b := increment(t, ctx, "Reminders")
	for _, id := range []string{a.ID, b.ID} {
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: project.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: katyoti.Stream.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rel.Drop(ctx, api.MoveInput{ID: project.ID, By: person, Note: "no hand-planned releases"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, b.ID} {
		got, err := inc.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if got.Increment.ShipsIn != "" || got.Increment.Stream != katyoti.Stream.ID {
			t.Errorf("%s after its release was dropped: ships in %q, stream %q; want none, still %s", id, got.Increment.ShipsIn, got.Increment.Stream, katyoti.Stream.ID)
		}
	}
	// Unship alone.
	other := plan(t, ctx, rel, api.PlanInput{Version: "2027.06"})
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: a.ID, In: other.ID}); err != nil {
		t.Fatal(err)
	}
	if got, err := inc.Unship(ctx, api.GetInput{ID: a.ID}); err != nil || got.Increment.ShipsIn != "" {
		t.Errorf("unship: %+v %v; want it in no release", got.Increment, err)
	}
}
