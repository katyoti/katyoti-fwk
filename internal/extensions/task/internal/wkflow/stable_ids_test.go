// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"sort"
	"testing"
)

// A workflow's node ids are the same however many workflows the process has
// loaded before: a workflow saved in one process names, in its frames, nodes
// the machine rebuilt by another process has.
func TestAWorkflowsNodeIDsAreTheSameInEveryProcess(t *testing.T) {
	cfg := map[string]any{"steps": []any{
		map[string]any{"name": "a", "kind": "delegate", "task": map[string]any{"type": "simulator"}},
		map[string]any{"name": "pick", "kind": "switch", "on": "inputs.x", "cases": map[string]any{
			"one": map[string]any{"name": "o", "kind": "delegate", "task": map[string]any{"type": "simulator"}},
			"two": map[string]any{"name": "t", "kind": "delegate", "task": map[string]any{"type": "simulator"}},
		}},
	}}
	ids := func() []string {
		root, err := LoadNodes(cfg)
		if err != nil {
			t.Fatal(err)
		}
		tree := map[string]*Node{}
		indexTree(tree, root)
		var out []string
		for id := range tree {
			out = append(out, id)
		}
		sort.Strings(out)
		return out
	}
	first := ids()
	for i := 0; i < 3; i++ { // the process loads other workflows meanwhile
		if _, err := LoadNodes(map[string]any{"steps": []any{map[string]any{"name": "z", "kind": "delegate", "task": map[string]any{"type": "simulator"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	second := ids()
	if len(first) != len(second) {
		t.Fatalf("ids %v then %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ids differ between loads: %v then %v", first, second)
		}
	}
}
