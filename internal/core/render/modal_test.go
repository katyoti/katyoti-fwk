// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// AN ACTION CAN OPEN A CENTRED MODAL HOLDING ANY PAGE NODE -
// the page grammar takes a modal action whose node is a valid page node, and
// refuses one with no node or an invalid one.
func TestAModalActionHoldsAPageNode(t *testing.T) {
	button := func(action map[string]any) map[string]any {
		return map[string]any{"kind": "button", "label": "Abandonner", "action": action}
	}
	ok := button(map[string]any{"label": "Abandonner", "modal": map[string]any{
		"title": "Abandonner l'étude ?", "size": "sm",
		"node": map[string]any{"kind": "vbox", "children": []any{
			map[string]any{"kind": "text", "content": "Rien n'est perdu : l'étude reste « À préparer »."},
			map[string]any{"kind": "button", "label": "Abandonner", "action": map[string]any{"uri": "ktw-acme:studies/abandon"}},
		}},
	}})
	if err := render.ValidatePage(ok); err != nil {
		t.Fatalf("a modal holding a page node is refused: %v", err)
	}
	if err := render.ValidatePage(button(map[string]any{"modal": map[string]any{"title": "x"}})); err == nil || !strings.Contains(err.Error(), "modal") {
		t.Errorf("a modal with no node is taken: %v", err)
	}
	if err := render.ValidatePage(button(map[string]any{"modal": map[string]any{"node": map[string]any{"kind": "no-such-kind"}}})); err == nil {
		t.Error("a modal holding an invalid node is taken")
	}
	found := false
	for _, v := range render.ActionVerbs {
		found = found || v == "modal"
	}
	if !found {
		t.Errorf("modal is not an action verb: %v", render.ActionVerbs)
	}
}
