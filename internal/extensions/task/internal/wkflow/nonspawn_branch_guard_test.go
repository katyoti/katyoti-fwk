// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"strings"
	"testing"
)

// Non-delegate branches inside race/parallel are rejected at LOAD time.
// They used to be skipped SILENTLY — and in parallel the skipped
// branch still counted toward parallelPhase.Pending, so the node waited for
// a completion that could never arrive and the workflow hung forever with no
// error. The replacement contract: authors fail at PARSE with an error that
// names the branch and points at the explicit nested-workflow idiom.
// (The handlers keep their own runtime reject as defense-in-depth.)

func TestLoadNodes_ParallelNonDelegateBranch_RejectedAtParse(t *testing.T) {
	_, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{
			"name": "fanout",
			"kind": "parallel",
			"branches": []any{
				map[string]any{
					"name": "real-child",
					"task": map[string]any{"type": "simulator", "duration_ms": 10},
				},
				map[string]any{"kind": "wait", "duration_ms": 50},
			},
		},
	}})
	if err == nil {
		t.Fatal("parallel with a non-delegate branch must fail at parse (pre-fix it hung forever at runtime)")
	}
	if !strings.Contains(err.Error(), "not supported as a parallel branch") {
		t.Errorf("error = %q, want the branch-kind rejection", err)
	}
	if !strings.Contains(err.Error(), "type: workflow") {
		t.Errorf("error = %q, want it to point at the nested-workflow idiom", err)
	}
}

func TestLoadNodes_RaceNonDelegateBranch_RejectedAtParse(t *testing.T) {
	_, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{
			"name": "the-race",
			"kind": "race",
			"branches": []any{
				map[string]any{
					"name": "runner",
					"task": map[string]any{"type": "simulator", "duration_ms": 10},
				},
				map[string]any{"kind": "wait", "duration_ms": 50},
			},
		},
	}})
	if err == nil {
		t.Fatal("race with a non-delegate branch must fail at parse (pre-fix the wait branch was dropped silently)")
	}
	if !strings.Contains(err.Error(), "not supported as a race branch") {
		t.Errorf("error = %q, want the branch-kind rejection", err)
	}
}

func TestLoadNodes_RaceTimeout(t *testing.T) {
	// Parsed onto the node.
	root, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{
			"name":       "r",
			"kind":       "race",
			"timeout_ms": 5000,
			"branches": []any{
				map[string]any{"name": "a", "task": map[string]any{"type": "simulator"}},
			},
		},
	}})
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	if got := root.Sequence.Steps[0].Race.TimeoutMs; got != 5000 {
		t.Errorf("TimeoutMs = %d, want 5000", got)
	}

	// Negative is rejected.
	if _, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{
			"name": "r", "kind": "race", "timeout_ms": -1,
			"branches": []any{
				map[string]any{"name": "a", "task": map[string]any{"type": "simulator"}},
			},
		},
	}}); err == nil {
		t.Error("negative timeout_ms must be rejected")
	}
}
