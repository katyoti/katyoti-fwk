// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// TestTaskToolName_MultiSegmentURIs pins the OpenAI / Anthropic tool-name
// grammar (`^[a-zA-Z0-9_-]+$`) against hierarchical task URIs introduced
// by cwm-v2-providers US-15 (2026-06-08). Every disallowed char in the
// URI — `:`, `/`, ... — must collapse to `_` so the resulting tool name
// stays valid.
func TestTaskToolName_MultiSegmentURIs(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Pre-US-15 single-segment URIs (no regression).
		{"refine-dossier-scope", "refine_dossier_scope"},
		{"ktw-acme:refine-dossier-scope", "ktw_acme_refine_dossier_scope"},
		// US-15: hierarchical method paths.
		{"ktw-acme:tasks/dossier/refine-scope", "ktw_acme_tasks_dossier_refine_scope"},
		{"ktw-agents:tasks/inbox/seed-default-items", "ktw_agents_tasks_inbox_seed_default_items"},
		{"ktw-agents:tasks/a/b/c/d/e", "ktw_agents_tasks_a_b_c_d_e"},
		// Idempotency on `_`-only names: pass through unchanged.
		{"already_safe_name", "already_safe_name"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := taskToolName(tc.in)
			if got != tc.want {
				t.Errorf("taskToolName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// lateCatalog is a TaskService whose catalog learns a task late — the sidecar
// publishing it registers after the run began.
type lateCatalog struct {
	taskapi.TaskService
	known map[string]bool
	asks  int
}

// Get answers as the real catalog does: by the task URI `<provider>:<name>`
// (or a bare name) — an action URI's `tasks/` is not part of a task's name.
func (c *lateCatalog) Get(_ context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	c.asks++
	if !c.known[in.Name] {
		return taskapi.TaskInfo{}, errors.New("task not found")
	}
	return taskapi.TaskInfo{Name: in.Name, Description: "generate"}, nil
}

// A task its provider has not registered yet when the run starts is looked up
// again on a later turn and joins the tools — it used to be skipped for the
// whole run ("catalog lookup failed").
func TestANodeActionThatRegistersLateJoinsTheTools(t *testing.T) {
	const uri = "ktw-methods:tasks/generate-goal-features"
	cat := &lateCatalog{known: map[string]bool{}}
	clock := time.Unix(0, 0)
	p := newNodeActions(context.Background(), cat, []string{uri}, func() time.Time { return clock })
	if got := p.ToolDefs(); len(got) != 0 {
		t.Fatalf("tools before the provider registered = %v, want none", got)
	}
	if p.Stable() {
		t.Fatal("Stable with a task still pending")
	}

	cat.known["ktw-methods:generate-goal-features"] = true
	asks := cat.asks
	if got := p.ToolDefs(); len(got) != 0 || cat.asks != asks {
		t.Fatalf("looked up again before the retry interval (asks %d → %d)", asks, cat.asks)
	}
	clock = clock.Add(actionRetryEvery)
	got := p.ToolDefs()
	if len(got) != 1 || got[0].Name != taskToolName(uri) {
		t.Fatalf("tools after the provider registered = %v, want %s", got, taskToolName(uri))
	}
	if !p.Stable() {
		t.Fatal("not Stable once every task resolved")
	}
}

func TestTaskRunName_TheTaskURIOfAnActionURI(t *testing.T) {
	for in, want := range map[string]string{
		"ktw-methods:tasks/generate-goal-features": "ktw-methods:generate-goal-features",
		"ktw-acme:tasks/dossier/refine-scope":      "ktw-acme:dossier/refine-scope",
		"idea-curator-promote":                     "idea-curator-promote",
		"ktw-agents:company-info":                  "ktw-agents:company-info",
	} {
		if got := taskRunName(in); got != want {
			t.Errorf("taskRunName(%q) = %q, want %q", in, got, want)
		}
	}
}
