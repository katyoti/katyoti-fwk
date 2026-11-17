// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
)

// T11 — property-based seal for the `${expr}` contract. Uses testing/quick
// stdlib (no external generator framework) and 1000 random cases per
// property.
//
// Properties:
//   P1. Literal preservation: any non-string non-${} value passes through
//   P2. ${expr} evaluation: well-formed wrapped expressions return their
//       evaluated value
//   P3. Parse-failure detection: malformed ${...} bodies return an error
//       and leave the bind set to the original wrapped string
//   P4. Bare strings stay literal (the silent eval-or-fall-back footgun
//       is closed — only `${...}` opts a string into evaluation)

// genQuickConfig keeps every quick.Check the same shape — 1000 iterations,
// fresh seed per run so consecutive `go test -count=N` runs explore
// different random subsets.
func genQuickConfig() *quick.Config {
	return &quick.Config{
		MaxCount: 1000,
		Rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// TestProperty_LiteralPreservation: any non-string, non-${} value in the
// binds map round-trips unchanged through ResolveMap.
func TestProperty_LiteralPreservation(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{Inputs: map[string]any{"x": 1}}

	f := func(intVal int, floatVal float64, boolVal bool) bool {
		binds := map[string]any{
			"i": intVal,
			"f": floatVal,
			"b": boolVal,
		}
		resolved, err := el.ResolveMap(binds, scope.AsMap(), cache)
		if err != nil {
			return false
		}
		return resolved["i"] == intVal &&
			resolved["f"] == floatVal &&
			resolved["b"] == boolVal
	}
	if err := quick.Check(f, genQuickConfig()); err != nil {
		t.Errorf("P1 literal preservation failed: %v", err)
	}
}

// TestProperty_WrappedExpressionEvaluates: well-formed ${expr} bodies
// always return the evaluated value, never the wrapped string. Generator
// produces arithmetic on inputs.x — the result is the deterministic
// integer answer regardless of which operator was picked.
func TestProperty_WrappedExpressionEvaluates(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{Inputs: map[string]any{"x": int64(7)}}

	ops := []string{"+", "-", "*"}
	f := func(opIdx uint8, rhs int8) bool {
		op := ops[int(opIdx)%len(ops)]
		wrapped := "${inputs.x " + op + " " + itoa(int64(rhs)) + "}"
		binds := map[string]any{"y": wrapped}
		resolved, err := el.ResolveMap(binds, scope.AsMap(), cache)
		if err != nil {
			return false
		}
		got, ok := toInt64(resolved["y"])
		if !ok {
			return false
		}
		var want int64
		switch op {
		case "+":
			want = 7 + int64(rhs)
		case "-":
			want = 7 - int64(rhs)
		case "*":
			want = 7 * int64(rhs)
		}
		return got == want
	}
	if err := quick.Check(f, genQuickConfig()); err != nil {
		t.Errorf("P2 wrapped expression eval failed: %v", err)
	}
}

// TestProperty_WrappedParseFailureReturnsError: an arbitrary malformed
// ${...} body causes ResolveMap to return a non-nil error AND leave the
// bind set to the original wrapped string.
func TestProperty_WrappedParseFailureReturnsError(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{Inputs: map[string]any{}}

	// Hand-picked malformed bodies. expr-lang's parser tolerates a
	// surprising amount of garbage, so the safest invalid shapes are
	// syntax that can't possibly be balanced or completed.
	bodies := []string{
		"((",
		")(",
		"))",
		"&&",
		"||",
		"a..b",
		"a==",
		"==a",
		".",
	}
	f := func(idx uint8) bool {
		body := bodies[int(idx)%len(bodies)]
		wrapped := "${" + body + "}"
		binds := map[string]any{"bad": wrapped}
		resolved, err := el.ResolveMap(binds, scope.AsMap(), cache)
		if err == nil {
			return false
		}
		return resolved["bad"] == wrapped
	}
	if err := quick.Check(f, genQuickConfig()); err != nil {
		t.Errorf("P3 wrapped parse-failure detection failed: %v", err)
	}
}

// TestProperty_BareStringsStayLiteral: any string that doesn't start
// with ${ stays a literal. The pre-R11 silent-eval-or-fallback footgun
// (where `"5+5"` evaluated to 10) is closed.
func TestProperty_BareStringsStayLiteral(t *testing.T) {
	cache := el.NewCache()
	scope := &Scope{Inputs: map[string]any{"x": int64(99)}}

	// A representative mix: bare arithmetic (would have eval'd legacy),
	// bare identifiers (would have eval'd legacy), bare data strings.
	bares := []string{
		"5+5",
		"inputs.x",
		"inputs.x * 2",
		"hello world",
		"  whitespace-padded  ",
		"true",
		"null",
		"42",
	}
	f := func(idx uint8) bool {
		s := bares[int(idx)%len(bares)]
		binds := map[string]any{"k": s}
		resolved, err := el.ResolveMap(binds, scope.AsMap(), cache)
		if err != nil {
			return false
		}
		return resolved["k"] == s
	}
	if err := quick.Check(f, genQuickConfig()); err != nil {
		t.Errorf("P4 bare-strings-literal failed: %v", err)
	}
}

// TestProperty_UnwrapExpression_RoundTrip: an arbitrary string s is
// detected as wrapped iff `s == "${" + inner + "}"` with inner non-empty.
// Sanity guard for the wrap detector across random inputs including
// boundary cases.
func TestProperty_UnwrapExpression_RoundTrip(t *testing.T) {
	f := func(inner string) bool {
		if strings.ContainsAny(inner, "\x00") {
			return true // skip control bytes
		}
		wrapped := "${" + inner + "}"
		got, ok := el.UnwrapExpression(wrapped)
		if inner == "" {
			return !ok
		}
		return ok && got == inner
	}
	if err := quick.Check(f, genQuickConfig()); err != nil {
		t.Errorf("P5 UnwrapExpression round-trip failed: %v", err)
	}
}

// itoa is a tiny base-10 helper — strconv would pull in a dep we don't
// otherwise need at the top of this file.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
