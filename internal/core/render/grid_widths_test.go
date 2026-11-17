// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A grid sized by widths validates and survives the wire: its
// widths, its dividers, an accordion drawn as rule lines whose folds keep
// their colour and data-* keys; ValidatePage refuses a negative width, a
// widths length other than cols, a divider outside the columns and an
// unknown accordion look.
func TestAGridSizedByWidthsValidates(t *testing.T) {
	row := render.Grid([]int{4, 150, 0, 210},
		render.Node{"kind": "box", "bg": "orange.6", "children": []render.Node{}},
		render.Text("m.RQ-047"), render.Text("title"), render.Text("4/6"),
	).Dividers(2, 3).With("gap", 0)
	tree := render.Accordion([]render.Fold{
		{ID: "running", Label: "Running", Badge: 2, Color: "orange", Data: map[string]string{"data-group": "running"}, Content: row},
		{ID: "done", Label: "59 done", Content: render.Text("…")},
	}, "running").RuleLook()
	def := render.Def{Kind: "page", DSLTree: tree}
	if err := def.Validate(); err != nil {
		t.Fatalf("a grid by widths must validate: %v", err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"widths":[4,150,0,210]`, `"cols":4`, `"dividers":[2,3]`, `"look":"rule"`, `"color":"orange"`, `"data-group":"running"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the wire form lacks %s: %s", want, s)
		}
	}
	var back render.Def
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("the decoded page must validate: %v", err)
	}
	var f render.Fold
	if err := json.Unmarshal([]byte(`{"label":"Running","color":"orange","data-group":"running","content":{"kind":"text","content":"x"}}`), &f); err != nil {
		t.Fatal(err)
	}
	if f.Color != "orange" || f.Data["data-group"] != "running" {
		t.Errorf("a fold reads back its colour and data keys: %+v", f)
	}

	grid := func(cols int, extra map[string]any) render.Node {
		n := render.Node{"kind": "grid", "cols": cols, "children": []render.Node{}}
		for k, v := range extra {
			n[k] = v
		}
		return n
	}
	for name, tc := range map[string]struct {
		tree render.Node
		want string
	}{
		"negative width":  {grid(2, map[string]any{"widths": []int{-4, 0}}), "width 0 is a number of pixels"},
		"short widths":    {grid(4, map[string]any{"widths": []int{4, 150, 0}}), "has 3 widths for 4 columns"},
		"long widths":     {grid(2, map[string]any{"widths": []int{4, 150, 0}}), "has 3 widths for 2 columns"},
		"widths not list": {grid(2, map[string]any{"widths": 4}), "a list of pixels"},
		"divider first":   {grid(4, map[string]any{"dividers": []int{0}}), "a divider is a column from 1 to 3"},
		"divider past":    {grid(4, map[string]any{"dividers": []int{4}}), "a divider is a column from 1 to 3"},
		"unknown look":    {render.Accordion(nil).With("look", "flat"), "an accordion's look is rule"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := render.ValidatePage(tc.tree); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
	if err := render.ValidatePage(grid(2, map[string]any{"widths": []int{0, 0}, "ratio": []int{2, 1}})); err != nil {
		t.Errorf("widths of 0 share by ratio: %v", err)
	}
}
