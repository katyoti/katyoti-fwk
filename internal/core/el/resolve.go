// SPDX-License-Identifier: Apache-2.0

package el

import "fmt"

// ResolveMap evaluates string values in binds as expressions against vars.
//
// Only strings wrapped in `${...}` are evaluated; bare strings are kept
// literally. A wrapped expression that fails to compile/run sets the
// per-bind result to the original wrapped string AND records the error,
// so the caller can surface a clear failure rather than silently degrading.
// The first error encountered is returned aggregated; remaining binds
// still process so callers can log + continue.
//
// Non-string values are kept as-is.
func ResolveMap(binds map[string]any, vars map[string]any, cache *Cache) (map[string]any, error) {
	if len(binds) == 0 {
		return nil, nil
	}
	resolved := make(map[string]any, len(binds))
	var firstErr error
	for k, v := range binds {
		strVal, isStr := v.(string)
		if !isStr {
			resolved[k] = v
			continue
		}
		inner, wrapped := UnwrapExpression(strVal)
		if !wrapped {
			resolved[k] = strVal
			continue
		}
		result, err := cache.Eval(inner, vars)
		if err != nil {
			resolved[k] = strVal
			if firstErr == nil {
				firstErr = fmt.Errorf("resolve %q: %w", k, err)
			}
			continue
		}
		resolved[k] = result
	}
	return resolved, firstErr
}
