// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// lxKinds is a connector declaring how its applications are drawn: their
// label and their code as a slot at the search and summary sizes
const lxKinds = `name: lx
label: Applications LX
description: The applications of the map, with their code.
sync: ktw-apps:apps/source-sync
search: ktw-apps:apps/search
propose: ktw-apps:apps/propose
kinds:
  application:
    label: Application
    slots:
      - attr: code
        label: Code applicatif
        sizes: [search, summary]
`

// : a connector's declared item kinds reach every place an
// item card is drawn from - the picker list, the board's picker node, the
// proposal wizard's picker nodes and each answer of a multi-source search -
// as the same {<kind>: {label, slots: [{attr, label, sizes}]}} the display's
// item card reads. A connector declaring none hands none.
func TestAConnectorsItemKindsReachThePickerAndTheSearchAnswers(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"lx.yaml": lxKinds})
	want := `{"application":{"label":"Application","slots":[{"attr":"code","label":"Code applicatif","sizes":["search","summary"]}]}}`
	same := func(where string, got any) {
		t.Helper()
		raw, _ := json.Marshal(got)
		if string(raw) != want {
			t.Errorf("%s: kinds %s, want %s", where, raw, want)
		}
	}

	pk, err := svc.Pickers(ctx, api.PickersInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pk.Connectors {
		switch c.Name {
		case "lx":
			same("the picker list", c.Kinds)
		case "apps":
			if c.Kinds != nil {
				t.Errorf("a connector declaring no kinds hands none: %+v", c.Kinds)
			}
		}
	}

	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "lx"}); err != nil {
		t.Fatal(err)
	}
	panel, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "lx"})
	if err != nil {
		t.Fatal(err)
	}
	node := findPicker(panel.DSLTree, "source-lx")
	if node == nil {
		t.Fatalf("the board panel has no picker: %+v", panel.DSLTree)
	}
	same("the board's picker node", node["kinds"])
	apps, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	if n := findPicker(apps.DSLTree, "source-apps"); n != nil {
		if _, ok := n["kinds"]; ok {
			t.Errorf("a connector declaring no kinds puts none on its picker: %+v", n["kinds"])
		}
	}

	wiz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Connectors: []string{"lx"}, Terms: []string{"achats"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := findPicker(wiz.Page, "lx"); n == nil {
		t.Errorf("the proposal wizard has no picker for lx: %+v", wiz.Page)
	} else {
		same("the proposal wizard's picker node", n["kinds"])
	}

	out, err := svc.SearchSources(ctx, api.SearchSourcesInput{Terms: []string{"achats"}, Connectors: []string{"lx", "apps"}})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, s := range out.Sources {
		switch s.Connector {
		case "lx":
			seen++
			same("the multi-source search's answer", s.Kinds)
		case "apps":
			seen++
			if s.Kinds != nil {
				t.Errorf("a source declaring no kinds answers none: %+v", s.Kinds)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("the search answers both sources: %+v", out.Sources)
	}
}

// findPicker finds the picker node of an id anywhere in a page tree, after
// a JSON round trip so the kinds compare as they are sent.
func findPicker(tree any, id string) map[string]any {
	raw, _ := json.Marshal(tree)
	var v any
	_ = json.Unmarshal(raw, &v)
	var walk func(any) map[string]any
	walk = func(x any) map[string]any {
		switch n := x.(type) {
		case map[string]any:
			if n["kind"] == "picker" && n["id"] == id {
				return n
			}
			for _, c := range n {
				if got := walk(c); got != nil {
					return got
				}
			}
		case []any:
			for _, c := range n {
				if got := walk(c); got != nil {
					return got
				}
			}
		}
		return nil
	}
	return walk(v)
}
