// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// The goal card's generic pieces: a stacked gauge whose
// legend is drawn as swatches and a card whose border is dashed validate and
// survive the wire; ValidatePage refuses another legend and a dashed that is
// not a boolean.
func TestAGaugeSwatchLegendAndADashedCardAreChecked(t *testing.T) {
	gauge := render.Stacked("",
		render.Segment{Value: 17, Color: "green", Label: "done"},
		render.Segment{Value: 1, Color: "orange", Label: "running"},
	).Swatches()
	tree := render.Card(gauge).Dashed()
	def := render.Def{Kind: "page", DSLTree: tree}
	if err := def.Validate(); err != nil {
		t.Fatalf("a dashed card with a swatch legend must validate: %v", err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"legend":"swatch"`, `"dashed":true`, `"color":"green"`} {
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
	if err := render.ValidatePage(render.Stacked("", render.Segment{Value: 1})); err != nil {
		t.Errorf("legend true keeps the chips: %v", err)
	}

	for name, tc := range map[string]struct {
		tree render.Node
		want string
	}{
		"unknown legend":  {render.Stacked("", render.Segment{Value: 1}).With("legend", "dots"), "a gauge's legend is true, false or swatch"},
		"dashed not bool": {render.Card().With("dashed", "yes"), "a card's dashed is true or false"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := render.ValidatePage(tc.tree); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
}

// A card's stripe: a tone validates and reaches the wire; an
// empty or non-text stripe is refused.
func TestACardStripeIsATone(t *testing.T) {
	card := render.Card(render.Text("a blocker")).Stripe("yellow")
	if err := render.ValidatePage(card); err != nil {
		t.Fatalf("a striped card must validate: %v", err)
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"stripe":"yellow"`) {
		t.Errorf("the wire form lacks its stripe: %s", raw)
	}
	for _, bad := range []any{"", 3, true} {
		if err := render.ValidatePage(render.Card().With("stripe", bad)); err == nil || !strings.Contains(err.Error(), "a card's stripe is a tone") {
			t.Errorf("stripe %v: want refused, got %v", bad, err)
		}
	}
}

// A card's fill height: fill validates and reaches the wire
// any other height is refused.
func TestACardFillHeightIsFill(t *testing.T) {
	card := render.Card(render.Text("a long list")).FillHeight()
	if err := render.ValidatePage(card); err != nil {
		t.Fatalf("a fill-height card must validate: %v", err)
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"height":"fill"`) {
		t.Errorf("the wire form lacks its height: %s", raw)
	}
	for _, bad := range []any{"", 480, "auto", true} {
		if err := render.ValidatePage(render.Card().With("height", bad)); err == nil || !strings.Contains(err.Error(), "a card's height is fill") {
			t.Errorf("height %v: want refused, got %v", bad, err)
		}
	}
}
