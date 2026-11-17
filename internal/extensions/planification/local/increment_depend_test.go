// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT DEPENDS AND IS DEPENDED ON: an increment
// that increments create made waits on another one, and an increment kept
// without the network-node capability (made before it was given) is ordered
// all the same - Depend gives the capability to what it orders.
func TestAnIncrementDependsOnAnotherIncrement(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	a := increment(t, ctx, "Invoices are drafted")
	b := increment(t, ctx, "Invoices reach their customers by mail")
	if !isNetworkNode(t, ctx, cwm, a.ID) {
		t.Errorf("increments create made %s without the network-node capability depends-on needs", a.ID)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: b.ID, On: a.ID}); err != nil {
		t.Fatalf("an increment made by increments create could not wait on another: %v", err)
	}

	// An increment stored without the capability: the depend gives it.
	c := increment(t, ctx, "Invoices are paid online")
	old, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := old.Object
	delete(o.Capabilities, api.CapNetworkNode)
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: b.ID}); err != nil {
		t.Fatalf("an increment kept without network-node could not wait on another: %v", err)
	}
	if !isNetworkNode(t, ctx, cwm, c.ID) {
		t.Errorf("%s depends on %s and is still no network node", c.ID, b.ID)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: a.ID, On: c.ID}); err == nil {
		t.Error("a depend closing a cycle of increments was accepted")
	}
}

// isNetworkNode says whether an object carries network-node, from its type or
// its own.
func isNetworkNode(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	t.Helper()
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(eff.Capabilities, func(c cwmapi.CapabilityRef) bool { return c.ID == api.CapNetworkNode })
}
