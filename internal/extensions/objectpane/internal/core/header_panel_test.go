// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// : a header panel is drawn above the tabs, so an aspect's tab
// list never sends it behind « More… ».
func TestHeaderPanelIsNeverMore(t *testing.T) {
	panels := []api.ObjectPanel{{ID: "tab:overview"}, {ID: "tab:header", Header: true}, {ID: "tab:files"}}
	out := applyTabList(panels, []string{"overview"}, nil)
	for _, p := range out {
		switch p.ID {
		case "tab:header":
			if p.More {
				t.Errorf("the header panel is flagged More")
			}
		case "tab:files":
			if !p.More {
				t.Errorf("an unlisted tab is not More")
			}
		}
	}
}

// : a More list whose names the strip never holds hides
// nothing - the unlisted tabs stay behind More; one that matches hides the rest.
func TestMoreListMatchingNoTabHidesNothing(t *testing.T) {
	panels := []api.ObjectPanel{{ID: "tab:overview"}, {ID: "tab:files"}, {ID: "tab:links"}}
	out := applyTabList(panels, []string{"overview"}, []string{"fiels"})
	if len(out) != 3 {
		t.Fatalf("a More list matching no tab hid tabs: %+v", out)
	}
	for _, p := range out {
		if p.ID != "tab:overview" && !p.More {
			t.Errorf("unlisted tab %s is not behind More", p.ID)
		}
	}
	out = applyTabList(panels, []string{"overview"}, []string{"fiels", "files"})
	if len(out) != 2 || out[1].ID != "tab:files" || !out[1].More {
		t.Fatalf("a More list with a matching name: want overview + files (More), got %+v", out)
	}
}
