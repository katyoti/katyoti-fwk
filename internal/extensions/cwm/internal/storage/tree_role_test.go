// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// shippedObjectTypes returns every shipped object type by name.
func shippedObjectTypes(t *testing.T) map[string]api.ObjectType {
	t.Helper()
	up := filepath.Join("..", "..", "..", "..", "..")
	// Both places a pack lives: a sidecar's own, and an extension's.
	dirs, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm", "object-types"))
	extDirs, _ := filepath.Glob(filepath.Join(up, "internal", "extensions", "*", "contrib", "embed", "cwm", "object-types"))
	dirs = append(dirs, extDirs...)
	out := map[string]api.ObjectType{}
	for _, d := range dirs {
		types, err := LoadObjectTypesLayered(os.DirFS(filepath.Dir(d)))
		if err != nil {
			t.Fatalf("load %s: %v", d, err)
		}
		for _, ty := range types {
			out[ty.Name] = ty
		}
	}
	// A FLOOR: with the glob wrong this would compare nothing and pass.
	if len(out) < 5 {
		t.Fatalf("only %d object type(s) loaded — the scan is not working", len(out))
	}
	return out
}

func has(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// WHERE A TYPE SHOWS IS THE TYPE'S `tree:`, NOT A NAME AND NOT A CONTAINMENT RULE.
//
// "Is this drawn in the tree, and may it sit at the top?" is a display fact of
// the TYPE — `tree: root | node | none` — never a capability an object carries.
//
// It is deliberately NOT derived from `accept_parents` / `allowed_children`.
// Those are VALIDATION rules about what may be created; using them for DISPLAY
// would conflate two questions and make "declared nothing" mean the same as
// "deliberately a leaf".
func TestTreeIsDeclaredByTheType(t *testing.T) {
	types := shippedObjectTypes(t)

	for _, tc := range []struct {
		typ  string
		tree string
		why  string
	}{
		{"software-module", api.TreeRoot, "a module is a top-level thing in a project, whatever its type is named - that is D2"},
		{"documentation", api.TreeRoot, "same: a module of another type name"},
		{"software-project", api.TreeRoot, "a project holds everything with no parent"},
		{"group", api.TreeRoot, "a pure container exists only to be drawn and to hold"},
		// Goals and features are lists, not tree items: their parent's pane
		// shows them in a Goals and a Features tab.
		{"goal", "", "listed by its parent's pane, in its Goals tab"},
		{"feature", "", "listed by its parent's pane, in its Features tab"},
		// Use cases left the tree by the person's decision (tree
		// list): the panels of the module or feature that owns them show them.
		{"use-case", "", "listed by its owner's panels, not drawn in the tree"},
		{"composition", api.TreeNode, "drawn under the project that holds it, never at the top"},
		// The one that must NOT be drawn, and the reason is load-bearing:
		// 196 criteria under 100 use-cases would bury the tree in rows nobody
		// navigates by. It has a parent but is shown by the pane that owns it.
		{"acceptance-criterion", "", "196 of them would bury the tree"},
	} {
		ty, ok := types[tc.typ]
		if !ok {
			t.Errorf("object type %q not shipped", tc.typ)
			continue
		}
		if ty.Tree != tc.tree {
			t.Errorf("%s: tree = %q, want %q — %s", tc.typ, ty.Tree, tc.tree, tc.why)
		}
		for _, c := range []string{"tree-node", "tree-root", "method-core"} {
			if has(ty.Capabilities, c) {
				t.Errorf("%s still lists %q as a capability — a display fact is the type's", tc.typ, c)
			}
		}
	}
}

// A `group` IS A CONTAINER AND NOTHING ELSE.
//
// It carries no capability: an object core drawn in the tree. The moment it
// gains a `want` or a `checkable` it has stopped being a grouping node and
// become a thing with a verdict of its own.
//
// This exists because `www` was already this shape in practice and had to
// invent an "ideal state" to give the grader something to read. A grouping node
// should not have to fake a criterion to exist — with children, it grades by
// ROLLUP, which is the honest answer.
func TestGroupIsPurelyAContainer(t *testing.T) {
	g, ok := shippedObjectTypes(t)["group"]
	if !ok {
		t.Fatal("object type `group` not shipped")
	}
	if len(g.Capabilities) != 0 || g.Tree != api.TreeRoot {
		t.Fatalf("group carries %v (tree %q); a pure container carries no capability and is drawn — anything "+
			"with a want or a checkable belongs to a type that has one", g.Capabilities, g.Tree)
	}
}
