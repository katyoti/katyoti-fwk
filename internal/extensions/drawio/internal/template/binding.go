// SPDX-License-Identifier: Apache-2.0

package template

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// exprCache compiles every expression once (bindings and ktw-when).
var exprCache = el.NewCache()

// Scope is what a binding, a placeholder and a ktw-when read for one
// element or relationship:
//
//	name        its label
//	id          its model id
//	kind        its kind
//	attributes  its attributes
//	constants   the library's constants
type Scope map[string]any

// NewScope builds the scope of one element or relationship.
func NewScope(id, name, kind string, attributes map[string]any, constants map[string]any) Scope {
	return Scope{"id": id, "name": name, "kind": kind, "attributes": attributes, "constants": constants}
}

// Evaluate returns the value of a binding against a scope. The primitives:
//
//	Scalar    → the value verbatim
//	Const     → constants.<path>
//	Lookup    → constants[table][value of path][field], else default
//	Match     → cases[value of path], else default
//	Coalesce  → first non-empty child
//	Expr      → an el expression over the scope
//
// A lookup or match miss is not an error: it returns the default.
func Evaluate(r *drawioapi.Resolver, scope Scope) (any, error) {
	switch r.Kind() {
	case drawioapi.ResolverScalar:
		return r.Scalar, nil
	case drawioapi.ResolverConst:
		return evaluatePath(map[string]any(scope), "constants."+r.Const), nil
	case drawioapi.ResolverLookup:
		key := evaluatePath(map[string]any(scope), r.Lookup.Path)
		if key == nil {
			return r.Lookup.Default, nil
		}
		tbl, ok := toMap(evaluatePath(map[string]any(scope), "constants."+r.Lookup.Table))
		if !ok {
			return r.Lookup.Default, nil
		}
		val, ok := tbl[fmt.Sprint(key)]
		if !ok {
			return r.Lookup.Default, nil
		}
		if r.Lookup.Field == "" {
			return val, nil
		}
		m, ok := toMap(val)
		if !ok || m[r.Lookup.Field] == nil {
			return r.Lookup.Default, nil
		}
		return m[r.Lookup.Field], nil
	case drawioapi.ResolverMatch:
		key := evaluatePath(map[string]any(scope), r.Match.Path)
		if v, ok := r.Match.Cases[fmt.Sprint(key)]; ok {
			return v, nil
		}
		return r.Match.Default, nil
	case drawioapi.ResolverCoalesce:
		for i := range r.Coalesce {
			v, err := Evaluate(&r.Coalesce[i], scope)
			if err != nil || isEmpty(v) {
				continue
			}
			return v, nil
		}
		return nil, nil
	case drawioapi.ResolverExpr:
		return exprCache.Eval(r.Expr, map[string]any(scope))
	}
	return nil, fmt.Errorf("unknown binding kind: %s", r.Kind())
}

// When reports whether a ktw-when expression holds for a scope. An
// expression that does not compile or run is false, with the error.
func When(expr string, scope Scope) (bool, error) {
	v, err := exprCache.Eval(expr, map[string]any(scope))
	if err != nil {
		return false, err
	}
	return el.Truthy(v), nil
}

// evaluatePath walks a dotted path; nil on any miss.
func evaluatePath(scope map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var cur any = scope
	for _, p := range strings.Split(path, ".") {
		m, ok := toMap(cur)
		if !ok {
			return nil
		}
		if cur, ok = m[p]; !ok {
			return nil
		}
	}
	return cur
}

// toMap accepts map[string]any and map[any]any.
func toMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	}
	return nil, false
}

// isEmpty mirrors el.Truthy's falsy set.
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// text renders a value for a label, an attribute or a style: nil is "",
// a whole float drops its decimals, a list joins with ", ".
func text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprint(int64(x))
		}
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, text(e))
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}
