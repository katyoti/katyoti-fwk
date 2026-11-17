// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// An action's result may carry the display panel its work runs in, and a page
// embeds a panel by its id with the `panel` node.
func TestAResultCarriesItsPanelAndAPageEmbedsOne(t *testing.T) {
	const id = "dp-01920000-0000-7000-8000-00000000a3f7"
	raw, err := json.Marshal(render.Result{OK: true, Message: "Installation lancée", Panel: id})
	if err != nil || !strings.Contains(string(raw), `"panel":"`+id+`"`) {
		t.Fatalf("result = %s (%v)", raw, err)
	}
	page := render.VBox(render.PanelNode(id, "Installer les programmes"))
	if err := render.ValidatePage(page); err != nil {
		t.Fatalf("a page embedding a panel: %v", err)
	}
	if err := render.ValidatePage(render.Node{"kind": "panel"}); err == nil || !strings.Contains(err.Error(), "panelId") {
		t.Fatalf("a panel node without its panel id passed: %v", err)
	}
}
