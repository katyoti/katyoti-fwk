// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A page text clamped to lines, mono and expandable
// validates and survives the wire; ValidatePage refuses a lineClamp that is
// not a whole number of 1 or more, a mono or expandable that is not a
// boolean, and expandable without lineClamp.
func TestATextClampMonoAndExpandableAreChecked(t *testing.T) {
	tree := render.VBox(
		render.Text("a long request title").Clamp(2),
		render.Text("m.RQ-433").Mono(),
		render.Text("a long block note").Clamp(3).Expandable().With("id", "note"),
	)
	def := render.Def{Kind: "page", DSLTree: tree}
	if err := def.Validate(); err != nil {
		t.Fatalf("a clamped, mono, expandable text must validate: %v", err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"lineClamp":2`, `"lineClamp":3`, `"mono":true`, `"expandable":true`} {
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

	for name, tc := range map[string]struct {
		tree render.Node
		want string
	}{
		"clamp zero":           {render.Text("x").Clamp(0), "a text's lineClamp is a whole number of 1 or more"},
		"clamp fraction":       {render.Text("x").With("lineClamp", 1.5), "a text's lineClamp is a whole number of 1 or more"},
		"clamp text":           {render.Text("x").With("lineClamp", "2"), "a text's lineClamp is a whole number of 1 or more"},
		"mono not bool":        {render.Text("x").With("mono", "yes"), "a text's mono is true or false"},
		"expandable not bool":  {render.Text("x").Clamp(2).With("expandable", 1), "a text's expandable is true or false"},
		"expandable, no clamp": {render.Text("x").Expandable(), "an expandable text needs its lineClamp"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := render.ValidatePage(tc.tree); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
}
