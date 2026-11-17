// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"strings"
	"testing"
)

func sampleJournal() []JournalEvent {
	return []JournalEvent{
		{File: "llm", Type: "session_start", Seq: 1, Data: map[string]any{"task": "demo"}},
		{File: "user", Type: "prompt_shown", Seq: 2, Turn: 1, Data: map[string]any{"prompt": "hello"}},
		{File: "llm", Type: "tool_call", Seq: 3, Turn: 1, Data: map[string]any{"tool": "file-read", "input": map[string]any{"path": "/repo/x"}}},
		{File: "llm", Type: "task_output", Seq: 4, Data: map[string]any{
			"run_id":    "run-1",
			"task_name": "worker-1",
			"status":    "completed",
			"output": map[string]any{
				"summary": "ok",
				"__spawn_dimensions": map[string]any{
					"permissions": map[string]any{
						"allowed_tools":   []any{"file-read", "file-write", "file-edit", "glob", "complete"},
						"permission_mode": "restricted",
					},
					"isolation":  map[string]any{"mode": "context_isolated"},
					"scheduling": map[string]any{"max_budget_usd": 5.0},
				},
			},
		}},
	}
}

func TestMatchEvent_ExactType(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	shape := map[string]any{"event": "task_output"}
	if !matchEvent(events[3], shape, vars) {
		t.Error("expected match for task_output event")
	}
	if matchEvent(events[0], shape, vars) {
		t.Error("expected no match for session_start event")
	}
}

func TestMatchEvent_FieldExact(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	shape := map[string]any{"event": "tool_call", "tool": "file-read"}
	if !matchEvent(events[2], shape, vars) {
		t.Error("expected match for tool_call with tool=file_read")
	}
	shape2 := map[string]any{"event": "tool_call", "tool": "bash"}
	if matchEvent(events[2], shape2, vars) {
		t.Error("expected no match when tool name differs")
	}
}

func TestMatchEvent_DottedPath(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	shape := map[string]any{
		"event":          "task_output",
		"output.summary": "ok",
	}
	if !matchEvent(events[3], shape, vars) {
		t.Error("expected match via dotted path output.summary=ok")
	}
}

func TestMatchEvent_Regex(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	shape := map[string]any{
		"event": "tool_call",
		"input": map[string]any{"path": map[string]any{"regex": `^/repo/`}},
	}
	if !matchEvent(events[2], shape, vars) {
		t.Error("expected regex match on input.path")
	}
}

func TestMatchEvent_VariableExpansion(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	// $SAFE_TOOL_SUBSET should expand to presets.SafeToolSubset; the snapshot
	// already has that exact set under permissions.allowed_tools.
	shape := map[string]any{
		"event": "task_output",
		"output": map[string]any{
			"__spawn_dimensions": map[string]any{
				"permissions": map[string]any{
					"allowed_tools": "$SAFE_TOOL_SUBSET",
				},
			},
		},
	}
	if !matchEvent(events[3], shape, vars) {
		t.Error("expected $SAFE_TOOL_SUBSET to expand and match permissions.allowed_tools")
	}
}

func TestMatchEvent_AnyWildcard(t *testing.T) {
	events := sampleJournal()
	vars := newMatcherVars()
	shape := map[string]any{
		"event":      "tool_call",
		"input.path": "any",
	}
	if !matchEvent(events[2], shape, vars) {
		t.Error("expected `any` to match arbitrary value")
	}
}

func TestFindSpawnDimensions_TopLevel(t *testing.T) {
	events := sampleJournal()
	dims, err := findSpawnDimensions(events, "")
	if err != nil {
		t.Fatalf("findSpawnDimensions: %v", err)
	}
	perms, _ := dims["permissions"].(map[string]any)
	if perms == nil || perms["permission_mode"] != "restricted" {
		t.Errorf("expected restricted permission_mode, got %+v", perms)
	}
}

func TestFindSpawnDimensions_ChildByName(t *testing.T) {
	events := sampleJournal()
	// task_name="worker-1" so child lookup should hit the same event.
	dims, err := findSpawnDimensions(events, "worker-1")
	if err != nil {
		t.Fatalf("findSpawnDimensions worker-1: %v", err)
	}
	if iso, _ := dims["isolation"].(map[string]any); iso == nil || iso["mode"] != "context_isolated" {
		t.Errorf("expected isolation.mode=context_isolated, got %+v", iso)
	}
}

func TestFindSpawnDimensions_MissingChild(t *testing.T) {
	events := sampleJournal()
	if _, err := findSpawnDimensions(events, "no-such-child"); err == nil {
		t.Error("expected error for unknown child")
	}
}

func TestMatchDimensions_HappyPath(t *testing.T) {
	events := sampleJournal()
	dims, _ := findSpawnDimensions(events, "")
	vars := newMatcherVars()
	shape := map[string]any{
		"permissions.allowed_tools":   "$SAFE_TOOL_SUBSET",
		"permissions.permission_mode": "restricted",
		"isolation.mode":              "context_isolated",
	}
	if msg := matchDimensions(dims, shape, vars); msg != "" {
		t.Errorf("expected pass, got: %s", msg)
	}
}

func TestMatchDimensions_Mismatch(t *testing.T) {
	events := sampleJournal()
	dims, _ := findSpawnDimensions(events, "")
	vars := newMatcherVars()
	shape := map[string]any{
		"permissions.permission_mode": "bubble",
	}
	if msg := matchDimensions(dims, shape, vars); msg == "" {
		t.Error("expected mismatch message")
	}
}

// {exists: false} is the UC-090 idiom: assert a dimension path is absent
// from the simulator snapshot. Required because serializeOverride strips
// default values (IsolationInProcess, history=="", etc.) before they reach
// the child config — we can't assert a value, we must assert absence.
func TestMatchDimensions_ExistsFalse_AbsentPathPasses(t *testing.T) {
	dims := map[string]any{
		"isolation": map[string]any{}, // mode key absent
	}
	vars := newMatcherVars()
	shape := map[string]any{
		"isolation.mode": map[string]any{"exists": false},
	}
	if msg := matchDimensions(dims, shape, vars); msg != "" {
		t.Errorf("expected absent path to pass {exists: false}, got: %s", msg)
	}
}

func TestMatchDimensions_ExistsFalse_PresentPathFails(t *testing.T) {
	dims := map[string]any{
		"isolation": map[string]any{"mode": "context_isolated"},
	}
	vars := newMatcherVars()
	shape := map[string]any{
		"isolation.mode": map[string]any{"exists": false},
	}
	msg := matchDimensions(dims, shape, vars)
	if msg == "" {
		t.Fatal("expected present path to fail {exists: false}")
	}
	if !strings.Contains(msg, "exists: false") {
		t.Errorf("failure message should mention exists:false; got %q", msg)
	}
}

func TestMatchDimensions_ExistsTrue_PresentPathPasses(t *testing.T) {
	dims := map[string]any{
		"isolation": map[string]any{"mode": "context_isolated"},
	}
	vars := newMatcherVars()
	shape := map[string]any{
		"isolation.mode": map[string]any{"exists": true},
	}
	if msg := matchDimensions(dims, shape, vars); msg != "" {
		t.Errorf("expected present path to pass {exists: true}, got: %s", msg)
	}
}
