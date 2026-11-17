// SPDX-License-Identifier: Apache-2.0

package el

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// WrapPrefix / WrapSuffix is the explicit expression-evaluation syntax.
// `${inputs.x}` evaluates `inputs.x`; bare `"inputs.x"` stays a literal.
// The wrap form is the ONLY way to opt a string into evaluation — there
// is no bare-expression fallback (closed per R11 / audit medium-finding-1).
const (
	WrapPrefix = "${"
	WrapSuffix = "}"
)

// UnwrapExpression returns (inner, true) if s is `${ ... }`, else ("", false).
// The check is conservative: the suffix must be the final character with no
// trailing whitespace, so a literal like `"${unmatched"` is correctly NOT
// detected as a wrapped expression and the caller treats it as literal.
func UnwrapExpression(s string) (string, bool) {
	if !strings.HasPrefix(s, WrapPrefix) || !strings.HasSuffix(s, WrapSuffix) {
		return "", false
	}
	inner := s[len(WrapPrefix) : len(s)-len(WrapSuffix)]
	if inner == "" {
		return "", false
	}
	return inner, true
}

// Compile compiles a standalone expression without caching. Use for
// one-shot validation; for repeated evaluation use a Cache. The compile
// uses expr.AllowUndefinedVariables so undefined names evaluate to nil
// rather than producing a parse-time error.
func Compile(expression string) (*vm.Program, error) {
	prog, err := expr.Compile(expression, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("el compile %q: %w", expression, err)
	}
	return prog, nil
}

// Names returns the top-level names an expression reads, variables and called
// functions — `verify` and `by` for `verify().green && by.kind == 'person'` —
// so an evaluator can tell an expression that calls a costly function.
func Names(expression string) (map[string]bool, error) {
	tree, err := parser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("el parse %q: %w", expression, err)
	}
	names := map[string]bool{}
	ast.Find(tree.Node, func(n ast.Node) bool {
		if id, ok := n.(*ast.IdentifierNode); ok {
			names[id.Value] = true
		}
		return false
	})
	return names, nil
}

// Cache caches compiled expr-lang programs keyed by expression string.
// Thread-safe. Create one per evaluation domain (e.g. one per workflow
// machine instance, or a package-level singleton for global predicates).
//
// Cache lifecycle: programs live for the cache's lifetime. There is no
// eviction — expression strings are bounded by source code / YAML, not
// user input, so the working set stays small.
type Cache struct {
	mu    sync.RWMutex
	progs map[string]*vm.Program
	opts  []expr.Option
}

// NewCache returns an empty Cache ready for use. Its expressions may name
// any variable: an undefined name evaluates to nil.
func NewCache() *Cache {
	return &Cache{progs: make(map[string]*vm.Program), opts: []expr.Option{expr.AllowUndefinedVariables()}}
}

// NewEnvCache returns a Cache that compiles against a DECLARED environment:
// env maps each variable an expression may name to a value of its type (the
// zero value is enough — `map[string]any{}`, `""`, `[]any{}`).
//
// A name the environment does not declare is a COMPILE error, not nil. That
// is the difference between "unknown name node" — which says the expression
// speaks another vocabulary — and a predicate that silently reads false
// forever because of a typo. Fields of a map-typed variable stay dynamic: a
// missing key is nil, which is what `'x' in object.fields` and `?.` are for.
//
// Use one per vocabulary; the environment is fixed for the cache's life.
func NewEnvCache(env map[string]any) *Cache {
	return &Cache{progs: make(map[string]*vm.Program), opts: []expr.Option{expr.Env(env)}}
}

// Compile returns the cached program for expression, compiling on first
// use under a write lock, with the cache's options (see NewCache and
// NewEnvCache for what an undefined name does).
func (c *Cache) Compile(expression string) (*vm.Program, error) {
	c.mu.RLock()
	prog, ok := c.progs[expression]
	c.mu.RUnlock()
	if ok {
		return prog, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring the write lock — another goroutine may
	// have compiled this expression while we were waiting.
	if prog, ok := c.progs[expression]; ok {
		return prog, nil
	}

	prog, err := expr.Compile(expression, c.opts...)
	if err != nil {
		return nil, fmt.Errorf("el compile %q: %w", expression, err)
	}
	c.progs[expression] = prog
	return prog, nil
}

// Eval compiles (if needed) and evaluates expression against vars.
// vars is the flat map of variables available to the expression
// (e.g. {"inputs": ..., "state": ..., "node": ...}). Caller decides
// the shape.
func (c *Cache) Eval(expression string, vars map[string]any) (any, error) {
	prog, err := c.Compile(expression)
	if err != nil {
		return nil, err
	}
	return expr.Run(prog, vars)
}

// EvalBool wraps Eval and applies Truthy to the result. Returns the
// truthy interpretation; the error is non-nil only on compile / eval
// failure (Truthy itself never fails).
func (c *Cache) EvalBool(expression string, vars map[string]any) (bool, error) {
	result, err := c.Eval(expression, vars)
	if err != nil {
		return false, err
	}
	return Truthy(result), nil
}

// ErrNotBool is returned by EvalPredicate for a result that is not a bool.
var ErrNotBool = errors.New("el: predicate did not return a bool")

// EvalPredicate evaluates a predicate that must return a bool. Unlike
// EvalBool it does NOT coerce: a guard returning a string or a number is a
// defect in the guard, and reading it as truthy would grant what it was
// written to withhold.
func (c *Cache) EvalPredicate(expression string, vars map[string]any) (bool, error) {
	result, err := c.Eval(expression, vars)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("%w (got %T)", ErrNotBool, result)
	}
	return b, nil
}
