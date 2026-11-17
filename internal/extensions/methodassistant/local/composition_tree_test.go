// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	macore "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// PLANNING SHOWS GOALS AND THE TREE OF FEATURES — Composition's tree puts each
// feature under the goal it serves, a sub-feature under the feature it breaks
// down, a feature serving nothing at the top; a row is its title and state,
// and only Advanced adds the planning fields (cost of delay, reachability).
func TestCompositionIsAGoalsAndFeaturesTreeWithPlanningFieldsBehindAdvanced(t *testing.T) {
	ctx, cwm := cwmapp.New(t, methodPack(nil))
	cwmapp.Module(t, ctx, cwm, "m")
	g := put(t, ctx, cwm, "g1", "goal", "Customers pay on time", nil)
	f := put(t, ctx, cwm, "f1", "feature", "Invoices go out", map[string]any{"business_value": 8, "time_criticality": 2},
		cwmapi.ObjectEdge{Kind: "serves", To: g})
	sub := putUnder(t, ctx, cwm, f, "f1-1", "feature", "By mail", nil)
	loose := put(t, ctx, cwm, "f9", "feature", "Someday", nil)
	svc := macore.New(nil)

	simple, err := svc.Composition(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	parent := map[string]string{}
	for _, r := range simple.Tree {
		parent[r.ID] = r.Parent
		if strings.Contains(r.Meta, "CoD") {
			t.Errorf("simple row %s carries %q; want no planning field until Advanced", r.ID, r.Meta)
		}
		if r.State == "" {
			t.Errorf("row %s has no state; want its state (idea when it has none)", r.ID)
		}
	}
	for id, want := range map[string]string{g: "", f: g, sub: f, loose: ""} {
		if got, ok := parent[id]; !ok || got != want {
			t.Errorf("row %s under %q (in tree %v); want under %q", id, got, ok, want)
		}
	}
	adv, err := svc.Composition(ctx, api.ObjectInput{ID: "m", Advanced: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range adv.Tree {
		if r.ID == f && !strings.Contains(r.Meta, "CoD 10") {
			t.Errorf("advanced row %s carries %q; want its cost of delay (CoD 10)", r.ID, r.Meta)
		}
	}
	if !adv.Advanced {
		t.Error("the advanced answer does not say it is advanced")
	}
}
