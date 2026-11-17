// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ORDERING PASSES A FEATURE CARRYING NO REQUEST: A
// depends on B, B on a feature C carrying no request; ordering D after A is
// accepted - C is walked through its own edges, not refused - and a cycle
// closing through C's own edges is still refused. A gone node on the way is a
// leaf.
func TestOrderingPassesAFeatureCarryingNoRequest(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := world(t)
	c := want(t, ctx, cwm, "feature", "Mail the invoices") // never planned: no request
	b := file(t, ctx, svc, api.FileInput{Title: "b"})
	addDependsOn(t, ctx, cwm, b.ID, c)
	a := file(t, ctx, svc, api.FileInput{Title: "a", DependsOn: []string{b.ID}})

	d := file(t, ctx, svc, api.FileInput{Title: "d"})
	if _, err := svc.Depend(ctx, api.DependInput{ID: d.ID, On: a.ID}); err != nil {
		t.Fatalf("d on a (a → b → %s, a feature carrying no request): %v; want accepted", c, err)
	}

	// A cycle through the feature's own edge is still refused.
	e := file(t, ctx, svc, api.FileInput{Title: "e"})
	addDependsOn(t, ctx, cwm, c, e.ID)
	_, err := svc.Depend(ctx, api.DependInput{ID: e.ID, On: a.ID})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("e on a (a → b → %s → e): %v; want a cycle refused", c, err)
	}

	// The ends are still checked: a feature carrying no request is not ordered.
	if _, err := svc.Depend(ctx, api.DependInput{ID: d.ID, On: c}); err == nil {
		t.Fatalf("d on %s (a feature carrying no request) was accepted", c)
	}
}

// addDependsOn adds a depends-on edge from one object to another, as an
// earlier write left it.
func addDependsOn(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, from, to string) {
	t.Helper()
	module, err := cwmapi.ModuleOfObjectID(from)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: from})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: to})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		t.Fatal(err)
	}
}
