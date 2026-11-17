// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A card of « À faire » names the project holding its request when several
// are served - a badge of element id project-mark - with no switch to set;
// with one project served no card names one; its actions carry the qualified
// request.
func TestACardNamesItsProjectWhenSeveralAreServed(t *testing.T) {
	ref := "walk-b:billing.RQ-001"
	of := api.PersonItem{Key: api.ActionTest + ":" + ref + "#it works", Action: api.ActionTest, Object: ref, Request: ref,
		Module: "billing", Project: "walk-b", Title: "walk-b's invoices", Deliverable: "it works", Asks: "Test it by hand."}
	one := of
	one.Key, one.Object, one.Request, one.Project = api.ActionTest+":billing.RQ-001#it works", "billing.RQ-001", "billing.RQ-001", ""

	for _, c := range []struct {
		it   api.PersonItem
		mark string
	}{
		{of, "walk-b"},
		{one, ""},
	} {
		card := queueCard(c.it, "fr", "")
		if got := projectMarkOf(card); got != c.mark {
			t.Errorf("project %q: the card's project badge says %q, want %q", c.it.Project, got, c.mark)
		}
		raw, _ := json.Marshal(card)
		if !strings.Contains(string(raw), `"id":"`+c.it.Request+`"`) {
			t.Errorf("project %q: the card's hand test must run on %s: %s", c.it.Project, c.it.Request, raw)
		}
		if err := render.ValidatePage(node{"kind": "vbox", "children": []node{card}}); err != nil {
			t.Errorf("project %q: the card must draw: %v", c.it.Project, err)
		}
	}
}

// projectMarkOf is the text of the card's badge of element id project-mark,
// "" when it has none.
func projectMarkOf(n node) string {
	if n["kind"] == "badge" && n["id"] == projectMarkID {
		s, _ := n["text"].(string)
		// What the walk reads: the renderer spreads a node's data-* keys.
		if n["data-element"] != "badge:"+projectMarkID || n["data-project"] != s {
			return "no data-element badge:project-mark naming " + s
		}
		return s
	}
	kids, _ := n["children"].([]node)
	for _, k := range kids {
		if s := projectMarkOf(k); s != "" {
			return s
		}
	}
	return ""
}
