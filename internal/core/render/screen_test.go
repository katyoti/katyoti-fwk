// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A screen action opens one of the host's own screens by its name - todo,
// « À faire », what the rail's To do opens; any other
// name is refused.
func TestAScreenActionOpensAHostScreen(t *testing.T) {
	page := func(a render.Action) any {
		raw, _ := json.Marshal(render.Button("Open", a))
		var n any
		_ = json.Unmarshal(raw, &n)
		return n
	}
	if err := render.ValidatePage(page(render.OpenScreen("Ouvrir À faire", render.ScreenTodo))); err != nil {
		t.Errorf("a screen action opening « À faire » is refused: %v", err)
	}
	if err := render.ValidatePage(page(render.OpenScreen("Open", "person-queue"))); err == nil {
		t.Error("a screen action naming no host screen is taken")
	}
	if err := render.ValidatePage(page(render.Action{Screen: render.ScreenTodo, Panel: "dp-1"})); err == nil {
		t.Error("an action with two verbs is taken")
	}
}
