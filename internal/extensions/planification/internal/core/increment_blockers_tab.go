// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE INCREMENT'S BLOCKERS TAB (its mock-up
// modules/research-planification-resilience/blockers-mockup.html): the
// pane's head, then a section loaded on its own - `increments blockers <id>`
// drawn as the mock-up draws it: a one-line verdict
// with its counts (yours, agents', outside, incidents), then one group per
// kind K1-K6 in reading order, each with its lead and its count, « Rien. »
// when it holds nothing; each line the root that holds the increment's work -
// its title and id, why, since when, the chain it heads, who acts - and the
// one action opening it. No second reader: the tab draws the verb's answer.

// The Blockers tab and its section (the panel's input and the region's).
const (
	paneBlockers    = "blockers"
	paneBlockerList = "blocker-list"
)

// blockersRoundKey is the page selection's key « Recompute » moves: the
// section reads it, so each press reads the blockers again.
const blockersRoundKey = "blockers_round"

// blockersTab is the Blockers tab: the pane's head, then its section, read
// again with each « Recompute ».
func (p paneData) blockersTab(ctx context.Context, cwm cwmapi.CWMService) node {
	args := node{"id": p.inc.ID, "tab": paneBlockerList, "round": "${selection." + blockersRoundKey + "}"}
	if p.lang != "" {
		args["language"] = p.lang
	}
	list := loadedSection("blockers", incrementPanePageURI, args, "", render.PlaceholderRows, 6)
	return node{"kind": "vbox", "gap": "md", "data-view": "increment-blockers", "children": []node{p.header(ctx, cwm), list}}
}

// blockersSelection is the selection the tab opens on: no « Recompute » yet.
func blockersSelection() map[string]any { return map[string]any{blockersRoundKey: 0} }
