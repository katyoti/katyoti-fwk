// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// ,: a part of the planning pages fed by
// its own source is a section - its load its method with its args (their
// ${selection.*} read against the selection), with a
// placeholder - and stays on the page grammar; no region kind is left.
func TestAPlanningRegionIsASectionToo(t *testing.T) {
	r := loadedSection("release", "ktw-x:views/release", node{"id": "${selection.release}"}, "Pick one", render.PlaceholderRows, 4)
	load, _ := r["load"].(render.Load)
	if r["kind"] != "box" || load.Method != "ktw-x:views/release" || load.Args["id"] != "${selection.release}" {
		t.Errorf("the section's load is not its method and args: %v", r)
	}
	if ph, _ := r["placeholder"].(render.Placeholder); ph.Kind != "rows" || ph.Count != 4 {
		t.Errorf("placeholder: %v", r["placeholder"])
	}
	if err := render.ValidatePage(node{"kind": "vbox", "children": []node{r}}); err != nil {
		t.Errorf("off the page grammar: %v", err)
	}
}
