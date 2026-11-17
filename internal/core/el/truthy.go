// SPDX-License-Identifier: Apache-2.0

package el

// Truthy returns the boolean interpretation of an arbitrary expr-lang
// result. False for nil / 0 / "" / false; true for everything else.
//
// Mirrors how lower-level languages coerce: any value that "exists and is
// non-empty" is true. Used by EvalBool and by call sites that need a
// consistent rule across Cache.Eval results of differing dynamic types.
func Truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		return false
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	}
	return true
}
