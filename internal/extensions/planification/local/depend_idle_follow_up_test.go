// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN ORDER TOWARD A REQUEST IDLE ON ITS FOLLOW-UPS NEVER DEADLOCKS
// : a request whose own steps are done stays idle until
// its follow-ups end, so ordering work after it waits on those follow-ups too.
// Whichever edge comes last closes the cycle and is refused, naming it: the
// follow-up already waits on the work, then the work is ordered after the
// idle request; or the work is ordered first, then the follow-up after it.
func TestAnOrderTowardARequestIdleOnAFollowUpThatWaitsBackIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	idle := file(t, ctx, req, api.FileInput{Title: "Connect the sources", Module: "billing", Kind: api.KindBuild, By: person})
	follow := file(t, ctx, req, api.FileInput{Title: "Build the connectors", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: follow.ID, To: idle.ID, Kind: cwmapi.EdgeType(api.EdgeFollowsUp)}); err != nil {
		t.Fatal(err)
	}
	work := file(t, ctx, req, api.FileInput{Title: "Read the sources", Module: "billing", Kind: api.KindBuild, By: person})
	// The follow-up waits on the work first; the work then ordered after the
	// request idle on that follow-up.
	if _, err := req.Depend(ctx, api.DependInput{ID: follow.ID, On: work.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := req.Depend(ctx, api.DependInput{ID: work.ID, On: idle.ID})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("work ordered after a request idle on a follow-up that waits on the work: %v; want refused, naming the cycle", err)
	}
	if got := dependsOnOf(t, ctx, cwm, work.ID); len(got) > 0 {
		t.Errorf("the refused order left %s depending on %v", work.ID, got)
	}
}

// dependsOnOf is what an object's depends-on edges point to.
func dependsOnOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range got.Object.Edges {
		if string(e.Kind) == api.EdgeDependsOn {
			out = append(out, e.To)
		}
	}
	return out
}
