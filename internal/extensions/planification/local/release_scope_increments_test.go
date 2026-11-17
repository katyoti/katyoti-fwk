// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE'S SCOPE NAMES ITS INCREMENTS APART — the increments shipping in a
// release are answered in their own field, each with its title, state and
// whether it is done; the module releases stay alone in theirs
func TestAReleaseScopeNamesItsIncrementsApart(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Stream: st.Stream.ID})
	in, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Send invoices by mail", ShipsIn: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := rel.Scope(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Modules) != 0 || len(sc.Increments) != 1 || sc.Increments[0].ID != in.Increment.ID ||
		sc.Increments[0].Title != "Send invoices by mail" || sc.Increments[0].State != api.IncrementOpen || sc.Increments[0].Done {
		t.Fatalf("the scope: modules %+v, increments %+v; want no module release and the open increment, by its title", sc.Modules, sc.Increments)
	}
}
