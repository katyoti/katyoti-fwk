// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// GOALS AND FEATURES ARE LISTS, NOT TREE ITEMS: with the SHIPPED goal and
// feature types (ktw-methods), a module's pane lists them in a "Goals" and a
// "Features" tab — each row with its lifecycle state and its key fields — and
// the tree draws neither.
func TestGoalsAndFeaturesAreTabsOfTheirParentNotTreeItems(t *testing.T) {
	shipped := func(rel string) string { return string(cwmapp.ShippedFile(t, rel)) }
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/goal.yaml":      shipped("app/ktw-methods/embed/cwm/object-types/goal.yaml"),
		"object-types/feature.yaml":   shipped("app/ktw-methods/embed/cwm/object-types/feature.yaml"),
		"workflows/goal.yaml":         shipped("app/ktw-methods/embed/cwm/workflows/goal.yaml"),
		"workflows/feature.yaml":      shipped("app/ktw-methods/embed/cwm/workflows/feature.yaml"),
		"capabilities/lifecycle.yaml": "name: lifecycle\nbadges: [{field: state, icon: progress, colors: {idea: gray, proposed: yellow}}]\n",
		"capabilities/want.yaml":      "name: want\nbadges: [{field: reachability, label: reach}]\n",
		"capabilities/feature.yaml":   "name: feature\n",
	})
	g := cwmapi.Object{ID: "m.G1", Type: "goal", Capabilities: map[string]map[string]any{"want": {"reachability": "asymptotic"}}}
	create(t, ctx, cwm, g)
	create(t, ctx, cwm, cwmapi.Object{ID: "m.F1", Type: "feature"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.F1", To: "proposed", By: cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}}); err != nil {
		t.Fatalf("propose F1: %v", err)
	}

	d := detail(t, ctx, svc, "m", false)
	by := panelByID(d.Panels)
	if p := by["tab:children/goal"]; p.Kind != api.PanelKindChildren || p.Name != "Goals" {
		t.Errorf("the goals tab: %+v (strip %v)", p, ids(d.Panels))
	}
	if p := by["tab:children/feature"]; p.Kind != api.PanelKindChildren || p.Name != "Features" {
		t.Errorf("the features tab: %+v (strip %v)", p, ids(d.Panels))
	}
	rows := map[string]api.ObjectChildRow{}
	for _, grp := range d.Children {
		if grp.Drawn {
			t.Errorf("%s is drawn in the tree", grp.Type)
		}
		for _, r := range grp.Rows {
			rows[r.ID] = r
		}
	}
	if r := rows["m.G1"]; r.State == nil || r.State.Label != "idea" || r.State.Color != "gray" ||
		len(r.Badges) != 1 || r.Badges[0].Label != "reach: asymptotic" {
		t.Errorf("the goal's row — never moved, so in its workflow's initial state, with its key field: %+v", r)
	}
	if r := rows["m.F1"]; r.State == nil || r.State.Label != "proposed" || r.State.Color != "yellow" || len(r.Badges) != 0 {
		t.Errorf("the feature's row — its state, never repeated as a key field: %+v", r)
	}

	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: "m", DrawnOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var walk func([]cwmapi.ObjectTreeNode)
	walk = func(ns []cwmapi.ObjectTreeNode) {
		for _, n := range ns {
			if n.Object.Type == "goal" || n.Object.Type == "feature" {
				t.Errorf("the tree draws %s (%s)", n.Object.ID, n.Object.Type)
			}
			walk(n.Children)
		}
	}
	walk(tree.Roots)
	if len(tree.Roots) != 1 || tree.Roots[0].Object.ID != "m" {
		t.Errorf("the tree: %+v", tree.Roots)
	}
}
