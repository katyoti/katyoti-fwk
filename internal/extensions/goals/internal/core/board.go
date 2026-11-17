// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Board is every goal under an object — a module's whole subtree, not only
// its direct children; a project's every module; with several projects
// served, those of the project holding the object only — as the page the Goals tab
// shows: the goals by state as a bar and as chips filtering the list, then
// each goal (a goal's sub-goals right after it) with its state, the object it
// sits under, its reachability and what serves it, a click opening it; Add
// creates one under the object. Read from CWM's index, plus the incoming
// `serves` edges of each goal.
func (s *Service) Board(ctx context.Context, in api.BoardInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: goals.Board: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	return board(ctx, cwm, in)
}

func board(ctx context.Context, cwm cwmapi.CWMService, in api.BoardInput) (render.Def, error) {
	// Holding keeps to the project holding the object (a qualified id or a
	// bare homonym in a universe serving several projects); the index keeps
	// the goals only - the whole subtree read over the bus is megabytes.
	rows, err := paneapi.HoldingSearch(ctx, cwm, in.ID, cwmapi.ObjectSearchInput{Types: []string{api.GoalType}})
	if err != nil {
		return render.Def{}, err
	}
	rows = treeOrder(rows)
	served := map[string]int{}
	for _, r := range rows {
		ref := r.ID
		if r.Project != "" {
			ref = cwmapi.QualifiedID(r.Project, r.ID)
		}
		servers, err := serversOf(ctx, cwm, ref)
		if err != nil {
			return render.Def{}, err
		}
		served[r.ID] = len(servers)
	}
	return render.Def{Kind: "page", DSLTree: boardTree(in, rows, served)}, nil
}

// treeOrder puts each goal right after the goal it sits under, siblings by
// position then name — the tree read top to bottom.
func treeOrder(rows []paneapi.ObjectRow) []paneapi.ObjectRow {
	in := map[string]bool{}
	kids := map[string][]paneapi.ObjectRow{}
	for _, r := range rows {
		in[r.ID] = true
	}
	var roots []paneapi.ObjectRow
	for _, r := range rows {
		if in[r.Parent] {
			kids[r.Parent] = append(kids[r.Parent], r)
		} else {
			roots = append(roots, r)
		}
	}
	less := func(list []paneapi.ObjectRow) {
		sort.SliceStable(list, func(i, j int) bool {
			a, b := list[i].Object.Order, list[j].Object.Order
			if a != b && a != 0 && b != 0 {
				return a < b
			}
			if (a == 0) != (b == 0) {
				return b == 0
			}
			return list[i].Name < list[j].Name
		})
	}
	var out []paneapi.ObjectRow
	var walk func([]paneapi.ObjectRow)
	walk = func(list []paneapi.ObjectRow) {
		less(list)
		for _, r := range list {
			out = append(out, r)
			walk(kids[r.ID])
		}
	}
	walk(roots)
	return out
}

func boardTree(in api.BoardInput, rows []paneapi.ObjectRow, served map[string]int) render.Node {
	lang := in.Language
	q := map[string]any{"state": in.State}
	shown := rows
	if in.State != "" {
		shown = nil
		for _, r := range rows {
			if r.State == in.State {
				shown = append(shown, r)
			}
		}
	}
	uncovered := 0
	for _, r := range rows {
		if served[r.ID] == 0 {
			uncovered++
		}
	}
	var blocks []render.Node
	if bar := paneapi.StateBar(tr(lang, "byState"), rows); bar != nil {
		blocks = append(blocks, bar)
	}
	if chips := paneapi.StateChips(tr(lang, "states"), tr(lang, "all"), rows, in.State, q); chips != nil {
		blocks = append(blocks, chips)
	}
	lines := make([]render.Node, 0, len(shown))
	for _, r := range shown {
		lines = append(lines, paneapi.RowNode(r, reachability(lang, r), servedBadge(lang, served[r.ID])))
	}
	aside := []render.Node{}
	if uncovered > 0 {
		aside = append(aside, render.Badge(fmt.Sprintf(tr(lang, "uncovered"), uncovered), "orange").With("size", "xs").With("keepCase", true))
	}
	aside = append(aside, render.Button(tr(lang, "add"), render.CreateUnder(tr(lang, "addGoal"), api.GoalType, "")).With("icon", "plus"))
	sec := render.Section(tr(lang, "goals"), "target").With("size", "md").With("color", "teal").With("badge", len(shown)).Aside(aside...)
	if len(shown) == 0 {
		empty := tr(lang, "noGoals")
		if len(rows) > 0 {
			empty = tr(lang, "noneInState")
		}
		sec = sec.Empty(empty, "")
	} else {
		sec["children"] = []render.Node{render.VBox(lines...).With("gap", "xs")}
	}
	return render.VBox(append(blocks, sec)...).With("gap", "md")
}

// reachability is a goal's want reachability as a badge: asymptotic (pursued,
// never done) or closable.
func reachability(lang string, r paneapi.ObjectRow) render.Node {
	v, _ := r.Object.Capabilities[cwmapi.CapWant]["reachability"].(string)
	if v == "" {
		return nil
	}
	tone := "violet"
	if v == "closable" {
		tone = "cyan"
	}
	return render.Badge(tr(lang, v), tone).With("size", "xs").With("variant", "outline").With("tooltip", tr(lang, "reachability"))
}

// servedBadge says how many objects serve a goal — orange when none does.
func servedBadge(lang string, n int) render.Node {
	if n == 0 {
		return render.Badge(tr(lang, "nothingServes"), "orange").With("size", "xs").With("icon", "alert-triangle").With("keepCase", true)
	}
	return render.Badge(fmt.Sprintf(tr(lang, "servedBy"), n), "green").With("size", "xs").With("icon", "link").With("keepCase", true)
}
