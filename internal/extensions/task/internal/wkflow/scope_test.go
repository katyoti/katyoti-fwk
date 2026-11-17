// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// The wkflow scope tests exercise the wkflow Scope projection
// (Inputs/State/Results/Loop/Now) through el.Cache. Basic Cache/Eval/Compile
// semantics live in internal/core/el/cache_test.go and are not duplicated
// here.

func TestScope_BasicEval(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{
		Inputs:  map[string]any{"topic": "AI safety"},
		State:   map[string]any{"count": 5},
		Results: map[string]any{"research": map[string]any{"summary": "findings here"}},
	}

	result, err := cache.Eval("inputs.topic", scope.AsMap())
	if err != nil {
		t.Fatalf("Eval inputs.topic: %v", err)
	}
	if result != "AI safety" {
		t.Errorf("inputs.topic = %v, want 'AI safety'", result)
	}

	result, err = cache.Eval("results.research.summary", scope.AsMap())
	if err != nil {
		t.Fatalf("Eval results.research.summary: %v", err)
	}
	if result != "findings here" {
		t.Errorf("results.research.summary = %v, want 'findings here'", result)
	}

	result, err = cache.Eval("state.count + 10", scope.AsMap())
	if err != nil {
		t.Fatalf("Eval state.count + 10: %v", err)
	}
	if result != 15 {
		t.Errorf("state.count + 10 = %v, want 15", result)
	}
}

func TestScope_LoopScope(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{
		Inputs:  map[string]any{},
		State:   map[string]any{},
		Results: map[string]any{},
		Loop: &LoopScope{
			Index: 2,
			Item:  "chapter-3",
		},
	}

	result, err := cache.Eval("loop.index", scope.AsMap())
	if err != nil {
		t.Fatalf("Eval loop.index: %v", err)
	}
	if result != 2 {
		t.Errorf("loop.index = %v, want 2", result)
	}

	result, err = cache.Eval("loop.item", scope.AsMap())
	if err != nil {
		t.Fatalf("Eval loop.item: %v", err)
	}
	if result != "chapter-3" {
		t.Errorf("loop.item = %v, want 'chapter-3'", result)
	}
}

func TestScope_NowIsSet(t *testing.T) {
	ws := &WorkflowState{
		Data:    map[string]any{},
		Results: map[string]any{},
	}
	scope := BuildScope(ws, nil, nil)

	if scope.Now.IsZero() {
		t.Error("now should not be zero")
	}

	m := scope.AsMap()
	if m["now"] == nil {
		t.Error("now missing from scope map")
	}
}

func TestScope_EvalBool(t *testing.T) {
	cache := el.NewCache()

	tests := []struct {
		expr  string
		state map[string]any
		want  bool
	}{
		{"state.ready == true", map[string]any{"ready": true}, true},
		{"state.ready == true", map[string]any{"ready": false}, false},
		{"state.count > 3", map[string]any{"count": 5}, true},
		{"state.count > 3", map[string]any{"count": 1}, false},
		{"results.check != nil", map[string]any{}, false},
	}

	for _, tt := range tests {
		scope := &Scope{
			Inputs:  map[string]any{},
			State:   tt.state,
			Results: map[string]any{},
		}
		got, err := cache.EvalBool(tt.expr, scope.AsMap())
		if err != nil {
			t.Errorf("EvalBool(%q): %v", tt.expr, err)
			continue
		}
		if got != tt.want {
			t.Errorf("EvalBool(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestResolveMap_OnScope(t *testing.T) {
	// Verify wkflow Scope binding resolves correctly. (Core ResolveMap
	// semantics are exhaustively tested in internal/core/el/cache_test.go.)
	cache := el.NewCache()
	scope := &Scope{
		Inputs:  map[string]any{"topic": "quantum computing"},
		State:   map[string]any{},
		Results: map[string]any{"step1": map[string]any{"data": "raw findings"}},
	}

	binds := map[string]any{
		"wrapped":     "${inputs.topic}",
		"data":        "${results.step1.data}",
		"bareExpr":    "inputs.topic", // bare → literal
		"plainString": "5+5",          // bare → literal (no silent arithmetic)
		"literal":     42,
	}

	resolved, err := el.ResolveMap(binds, scope.AsMap(), cache)
	if err != nil {
		t.Fatalf("ResolveMap: %v", err)
	}
	if resolved["wrapped"] != "quantum computing" {
		t.Errorf("wrapped = %v, want 'quantum computing'", resolved["wrapped"])
	}
	if resolved["data"] != "raw findings" {
		t.Errorf("data = %v, want 'raw findings'", resolved["data"])
	}
	if resolved["bareExpr"] != "inputs.topic" {
		t.Errorf("bareExpr = %v, want literal 'inputs.topic'", resolved["bareExpr"])
	}
	if resolved["plainString"] != "5+5" {
		t.Errorf("plainString = %v, want literal '5+5'", resolved["plainString"])
	}
	if resolved["literal"] != 42 {
		t.Errorf("literal = %v, want 42", resolved["literal"])
	}
}

func TestBuildScope_FromWorkflowState(t *testing.T) {
	ws := &WorkflowState{
		Active: []ActivationFrame{
			{NodeID: "root", Kind: NodeSequence},
		},
		Children: map[string]engine.ChildHandle{},
		Results: map[string]any{
			"researcher": map[string]any{"summary": "found 5 papers"},
		},
		Data: map[string]any{
			"iteration": 3,
		},
	}
	inputs := map[string]any{"goal": "survey AI safety"}

	scope := BuildScope(ws, inputs, &LoopScope{Index: 1, Item: "paper-2"})

	if scope.Inputs["goal"] != "survey AI safety" {
		t.Errorf("Inputs.goal = %v", scope.Inputs["goal"])
	}
	if scope.State["iteration"] != 3 {
		t.Errorf("State.iteration = %v", scope.State["iteration"])
	}
	res, ok := scope.Results["researcher"].(map[string]any)
	if !ok || res["summary"] != "found 5 papers" {
		t.Errorf("Results.researcher = %v", scope.Results["researcher"])
	}
	if scope.Loop == nil || scope.Loop.Index != 1 || scope.Loop.Item != "paper-2" {
		t.Errorf("Loop = %v", scope.Loop)
	}
	if scope.Now.IsZero() {
		t.Error("Now should not be zero")
	}
}
