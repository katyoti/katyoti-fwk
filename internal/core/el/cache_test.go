// SPDX-License-Identifier: Apache-2.0

package el

import (
	"errors"
	"strings"
	"testing"
)

func TestCache_EvalBasic(t *testing.T) {
	c := NewCache()
	vars := map[string]any{
		"inputs":  map[string]any{"topic": "AI safety"},
		"state":   map[string]any{"count": 5},
		"results": map[string]any{"research": map[string]any{"summary": "findings"}},
	}

	got, err := c.Eval("inputs.topic", vars)
	if err != nil {
		t.Fatalf("Eval inputs.topic: %v", err)
	}
	if got != "AI safety" {
		t.Errorf("inputs.topic = %v, want 'AI safety'", got)
	}

	got, err = c.Eval("results.research.summary", vars)
	if err != nil {
		t.Fatalf("Eval results.research.summary: %v", err)
	}
	if got != "findings" {
		t.Errorf("results.research.summary = %v, want 'findings'", got)
	}

	got, err = c.Eval("state.count + 10", vars)
	if err != nil {
		t.Fatalf("Eval arithmetic: %v", err)
	}
	if got != 15 {
		t.Errorf("state.count + 10 = %v, want 15", got)
	}
}

func TestCache_CachesPrograms(t *testing.T) {
	c := NewCache()
	vars := map[string]any{"x": 1}

	_, err := c.Eval("x + 1", vars)
	if err != nil {
		t.Fatalf("first eval: %v", err)
	}
	got, err := c.Eval("x + 1", vars)
	if err != nil {
		t.Fatalf("second eval: %v", err)
	}
	if got != 2 {
		t.Errorf("cached eval = %v, want 2", got)
	}

	c.mu.RLock()
	n := len(c.progs)
	c.mu.RUnlock()
	if n != 1 {
		t.Errorf("cache entries = %d, want 1", n)
	}
}

func TestCache_AllowsUndefinedVariables(t *testing.T) {
	// AllowUndefinedVariables: undefined names parse OK and evaluate to nil
	// at the top level, rather than producing a parse-time error.
	c := NewCache()
	got, err := c.Eval("missing", map[string]any{})
	if err != nil {
		t.Fatalf("Eval missing: %v", err)
	}
	if got != nil {
		t.Errorf("missing = %v, want nil", got)
	}
}

func TestCache_CompileErrorWrapped(t *testing.T) {
	c := NewCache()
	_, err := c.Compile("((")
	if err == nil {
		t.Fatal("expected compile error for '(('")
	}
	if !strings.Contains(err.Error(), "el compile") {
		t.Errorf("error %q should be wrapped with 'el compile' prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "((") {
		t.Errorf("error %q should reference the failing expression", err.Error())
	}
}

func TestCompile_StandaloneErrorWrapped(t *testing.T) {
	_, err := Compile("((")
	if err == nil {
		t.Fatal("expected standalone compile error for '(('")
	}
	if !strings.Contains(err.Error(), "el compile") {
		t.Errorf("error %q should be wrapped with 'el compile' prefix", err.Error())
	}
}

func TestCache_EvalBool(t *testing.T) {
	c := NewCache()
	tests := []struct {
		expr string
		vars map[string]any
		want bool
	}{
		{"state.ready == true", map[string]any{"state": map[string]any{"ready": true}}, true},
		{"state.ready == true", map[string]any{"state": map[string]any{"ready": false}}, false},
		{"state.count > 3", map[string]any{"state": map[string]any{"count": 5}}, true},
		{"state.count > 3", map[string]any{"state": map[string]any{"count": 1}}, false},
		{"missing != nil", map[string]any{}, false},
	}
	for _, tt := range tests {
		got, err := c.EvalBool(tt.expr, tt.vars)
		if err != nil {
			t.Errorf("EvalBool(%q): %v", tt.expr, err)
			continue
		}
		if got != tt.want {
			t.Errorf("EvalBool(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestUnwrapExpression(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wrapped bool
	}{
		{"${inputs.x}", "inputs.x", true},
		{"${5+5}", "5+5", true},
		{"${}", "", false},
		{"${inputs.x", "", false},
		{"$inputs.x}", "", false},
		{"plain", "", false},
		{"", "", false},
		{"${{nested}}", "{nested}", true},
	}
	for _, tc := range cases {
		got, ok := UnwrapExpression(tc.in)
		if ok != tc.wrapped {
			t.Errorf("UnwrapExpression(%q) wrapped = %v, want %v", tc.in, ok, tc.wrapped)
		}
		if ok && got != tc.want {
			t.Errorf("UnwrapExpression(%q) inner = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruthy(t *testing.T) {
	cases := []struct {
		v    any
		want bool
	}{
		{nil, false},
		{false, false},
		{true, true},
		{0, false},
		{1, true},
		{int64(0), false},
		{int64(-1), true},
		{0.0, false},
		{1.5, true},
		{"", false},
		{"hello", true},
		{[]int{}, true},               // non-zero slice → truthy (no explicit case)
		{map[string]int{}, true},      // map → truthy
		{struct{ X int }{X: 0}, true}, // arbitrary struct → truthy
	}
	for _, tc := range cases {
		if got := Truthy(tc.v); got != tc.want {
			t.Errorf("Truthy(%#v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestResolveMap_WrapsAndLiterals(t *testing.T) {
	c := NewCache()
	vars := map[string]any{
		"inputs":  map[string]any{"topic": "quantum"},
		"results": map[string]any{"step1": map[string]any{"data": "raw"}},
	}
	binds := map[string]any{
		"wrapped":     "${inputs.topic}",
		"data":        "${results.step1.data}",
		"bareExpr":    "inputs.topic", // bare → literal (no silent eval)
		"plainString": "5+5",          // bare → literal (no silent arithmetic)
		"literal":     42,
	}

	resolved, err := ResolveMap(binds, vars, c)
	if err != nil {
		t.Fatalf("ResolveMap: %v", err)
	}
	if resolved["wrapped"] != "quantum" {
		t.Errorf("wrapped = %v, want 'quantum'", resolved["wrapped"])
	}
	if resolved["data"] != "raw" {
		t.Errorf("data = %v, want 'raw'", resolved["data"])
	}
	if resolved["bareExpr"] != "inputs.topic" {
		t.Errorf("bareExpr = %v, want literal", resolved["bareExpr"])
	}
	if resolved["plainString"] != "5+5" {
		t.Errorf("plainString = %v, want literal (no silent arithmetic)", resolved["plainString"])
	}
	if resolved["literal"] != 42 {
		t.Errorf("literal = %v, want 42", resolved["literal"])
	}
}

func TestResolveMap_WrappedParseFailure(t *testing.T) {
	c := NewCache()
	vars := map[string]any{"inputs": map[string]any{"x": 1}}
	binds := map[string]any{
		"bad":  "${ unmatched paren (}",
		"good": "${inputs.x}",
	}

	resolved, err := ResolveMap(binds, vars, c)
	if err == nil {
		t.Fatal("ResolveMap should error on wrapped parse failure")
	}
	if resolved["good"] != 1 {
		t.Errorf("good = %v, want 1", resolved["good"])
	}
	if resolved["bad"] != "${ unmatched paren (}" {
		t.Errorf("bad = %v, want original wrapped string", resolved["bad"])
	}
}

func TestResolveMap_EmptyBinds(t *testing.T) {
	c := NewCache()
	resolved, err := ResolveMap(nil, map[string]any{}, c)
	if err != nil {
		t.Fatalf("ResolveMap nil: %v", err)
	}
	if resolved != nil {
		t.Errorf("ResolveMap nil = %v, want nil", resolved)
	}
}

// TestEnvCacheRefusesAnUndeclaredName — the guard that replaces CEL's type
// check: a name outside the vocabulary is a compile error naming it, never a
// nil that makes a predicate read false forever.
func TestEnvCacheRefusesAnUndeclaredName(t *testing.T) {
	c := NewEnvCache(map[string]any{"object": map[string]any{}, "type": ""})
	_, err := c.Compile(`node.state == 'active'`)
	if err == nil || !strings.Contains(err.Error(), "node") {
		t.Fatalf("an undeclared variable must fail to compile naming it, got %v", err)
	}
	// Map fields stay dynamic: a missing key is nil, not an error.
	ok, err := c.EvalPredicate(`object.status == nil && type == 'x'`, map[string]any{"object": map[string]any{}, "type": "x"})
	if err != nil || !ok {
		t.Errorf("declared names must evaluate, got %v %v", ok, err)
	}
	// The lenient cache is unchanged: its callers rely on nil for unknowns.
	if _, err := NewCache().Compile(`node.state == 'active'`); err != nil {
		t.Errorf("NewCache must still accept undeclared names: %v", err)
	}
}

// TestEvalPredicateDoesNotCoerce — a guard returning a non-bool is a defect,
// and truthiness would grant what the guard exists to withhold.
func TestEvalPredicateDoesNotCoerce(t *testing.T) {
	c := NewEnvCache(map[string]any{"s": ""})
	if _, err := c.EvalPredicate(`s`, map[string]any{"s": "yes"}); !errors.Is(err, ErrNotBool) {
		t.Errorf("a string result must be ErrNotBool, got %v", err)
	}
}
