// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

func treeNode(id, typ string, kids ...api.ObjectTreeNode) api.ObjectTreeNode {
	return api.ObjectTreeNode{Object: api.Object{ID: id, Type: typ}, Children: kids}
}

func flat(nodes []api.ObjectTreeNode) []string {
	var out []string
	var walk func([]api.ObjectTreeNode)
	walk = func(ns []api.ObjectTreeNode) {
		for _, n := range ns {
			out = append(out, n.Object.ID)
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

func drawnSvc() *Service {
	return &Service{objTypes: map[string]api.ObjectType{
		"module":               {Name: "module", Capabilities: []string{"container"}, Tree: api.TreeRoot},
		"use-case":             {Name: "use-case", Capabilities: []string{"content"}, Tree: api.TreeNode},
		"acceptance-criterion": {Name: "acceptance-criterion", Capabilities: []string{"content", "checkable"}},
	}}
}

// A TYPE WITHOUT `tree:` IS NOT DRAWN.
//
// `acceptance-criterion` is the case `tree:` exists for: 215 of them in
// this corpus, and a tree that lists every criterion is one nobody navigates.
// The filter asks the TYPE, never the name — a GUI branching on "is it called
// acceptance-criterion" is the type-name check the type's `tree:` replaces.
func TestPruneToDrawn_HidesTypesWithoutTreeNode(t *testing.T) {
	s := drawnSvc()
	in := []api.ObjectTreeNode{
		treeNode("cwm", "module",
			treeNode("cwm.UC1", "use-case",
				treeNode("cwm.UC1-AC1", "acceptance-criterion"),
				treeNode("cwm.UC1-AC2", "acceptance-criterion"),
			),
		),
	}
	got, total := s.pruneToDrawn(in)
	want := []string{"cwm", "cwm.UC1"}
	if ids := flat(got); len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("drawn = %v, want %v — criteria must not be drawn", ids, want)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 — the count must reflect what is drawn, not what was scanned", total)
	}
}

// A HIDDEN OBJECT DOES NOT HIDE ITS DESCENDANTS.
//
// "Do not draw this row" and "do not draw this subtree" are different
// instructions, and the capability only says the first. Dropping the subtree
// would let one undeclared type silently erase everything filed beneath it.
func TestPruneToDrawn_LiftsChildrenOfAHiddenObject(t *testing.T) {
	s := drawnSvc()
	in := []api.ObjectTreeNode{
		treeNode("cwm", "module",
			treeNode("cwm.AC-in-the-middle", "acceptance-criterion",
				treeNode("cwm.UC9", "use-case"),
			),
		),
	}
	got, _ := s.pruneToDrawn(in)
	ids := flat(got)
	found := false
	for _, id := range ids {
		if id == "cwm.UC9" {
			found = true
		}
	}
	if !found {
		t.Errorf("drawn = %v — a drawn object filed under a hidden one was dropped with it", ids)
	}
}

// AN UNDECLARED TYPE IS DRAWN.
//
// A type nobody declared is a problem to SEE. Hiding it by default would make
// a corpus with a typo in one `type:` render as a silently smaller tree.
func TestPruneToDrawn_UndeclaredTypeStaysVisible(t *testing.T) {
	s := drawnSvc()
	got, total := s.pruneToDrawn([]api.ObjectTreeNode{treeNode("x.mystery", "not-a-declared-type")})
	if total != 1 || len(got) != 1 {
		t.Errorf("an undeclared type was hidden (total=%d) — it should be visible so it can be noticed", total)
	}
}

// : every tree node says its level - project or
// module - read from its type's level capability, whatever the type's name;
// below them, none.
func TestEveryTreeNodeSaysItsLevel(t *testing.T) {
	s := &Service{objTypes: map[string]api.ObjectType{
		"software-project": {Name: "software-project", Capabilities: []string{api.CapProject}, Tree: api.TreeRoot},
		"software-module":  {Name: "software-module", Capabilities: []string{api.CapModule}, Tree: api.TreeNode},
		"use-case":         {Name: "use-case", Capabilities: []string{"content"}, Tree: api.TreeNode},
	}}
	nodes := []api.ObjectTreeNode{treeNode("p", "software-project", treeNode("sw", "software-module", treeNode("sw.UC1", "use-case")))}
	s.markTreeLevel(t.Context(), nodes)
	p, sw := nodes[0], nodes[0].Children[0]
	if p.Level != api.CapProject || sw.Level != api.CapModule || sw.Children[0].Level != "" {
		t.Fatalf("levels: %q %q %q; want project, module, none", p.Level, sw.Level, sw.Children[0].Level)
	}
}
