// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : a source says who kept each of its items and when - the
// one record of it, read by any consumer through SourcesOf. The keeper is
// the one who held the item first: holding it again, by another, keeps it;
// applying carries it into the selection.
func TestAHeldItemSaysWhoKeptItAndWhen(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	alice := &api.Keeper{ID: "jdoe", Kind: "person"}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", By: alice,
		Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}}}); err != nil {
		t.Fatal(err)
	}
	kept := func() map[string]api.SelectedItem {
		t.Helper()
		of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
		if err != nil || len(of.Sources) != 1 {
			t.Fatalf("sources of: %v %+v", err, of)
		}
		out := map[string]api.SelectedItem{}
		for _, it := range of.Sources[0].Kept {
			out[it.Key] = it
		}
		return out
	}
	first := kept()["A1"]
	if first.KeptBy == nil || *first.KeptBy != *alice || first.KeptAt == "" {
		t.Fatalf("the held item says who kept it and when: %+v", first)
	}
	// Held again with a new item, by an agent: A1 keeps its keeper, A2 gets the agent.
	bot := &api.Keeper{ID: "claude-x", Kind: "agent"}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", By: bot,
		Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}}}); err != nil {
		t.Fatal(err)
	}
	k := kept()
	if k["A1"].KeptBy == nil || *k["A1"].KeptBy != *alice || k["A1"].KeptAt != first.KeptAt {
		t.Errorf("held again, the item changed keeper: %+v", k["A1"])
	}
	if k["A2"].KeptBy == nil || *k["A2"].KeptBy != *bot {
		t.Errorf("the new item is kept by who held it: %+v", k["A2"])
	}
	// Applied: the selection carries the keepers.
	if _, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	k = kept()
	if len(k) != 2 || k["A1"].KeptBy == nil || *k["A1"].KeptBy != *alice || k["A1"].KeptAt != first.KeptAt || k["A2"].KeptBy == nil || *k["A2"].KeptBy != *bot {
		t.Errorf("applied, the selection lost who kept its items: %+v", k)
	}
	// Picked again on the source's own pick tree (source-select): a kept item
	// keeps its keeper, a new one is kept by who picks it.
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil || len(of.Sources) != 1 {
		t.Fatalf("sources of: %v %+v", err, of)
	}
	if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: of.Sources[0].ID, By: bot,
		Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A4", Title: "Acme CRM"}}}); err != nil {
		t.Fatal(err)
	}
	k = kept()
	if k["A1"].KeptBy == nil || *k["A1"].KeptBy != *alice || k["A1"].KeptAt != first.KeptAt {
		t.Errorf("picked again, the item lost its keeper: %+v", k["A1"])
	}
	if k["A4"].KeptBy == nil || *k["A4"].KeptBy != *bot || k["A2"].Key != "" {
		t.Errorf("the pick tree's choice: %+v", k)
	}
	// An item held with no one named reads with no keeper.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A3", Title: "Acme Paie"}}}); err != nil {
		t.Fatal(err)
	}
	if a3 := kept()["A3"]; a3.Key != "A3" || a3.KeptBy != nil {
		t.Errorf("an item held by no one named: %+v", a3)
	}
}
