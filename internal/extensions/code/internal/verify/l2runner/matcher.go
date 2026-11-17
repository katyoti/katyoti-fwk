// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// matcherVars holds harness-side substitutions available to journal matchers
// at assertion time. Populated by the runner from the captured journal +
// known constants (`presets.SafeToolSubset`).
type matcherVars struct {
	values map[string]any
}

// newMatcherVars seeds the well-known variables. Add per-run lookups via
// .add() after the journal has been captured.
func newMatcherVars() *matcherVars {
	tools := make([]any, len(presets.SafeToolSubset))
	for i, t := range presets.SafeToolSubset {
		tools[i] = t
	}
	return &matcherVars{
		values: map[string]any{
			"SAFE_TOOL_SUBSET": tools,
		},
	}
}

func (mv *matcherVars) add(name string, v any) {
	if mv.values == nil {
		mv.values = map[string]any{}
	}
	mv.values[name] = v
}

func (mv *matcherVars) resolve(name string) (any, bool) {
	v, ok := mv.values[name]
	return v, ok
}

// matchEvent returns true when `event` satisfies the shape: the event's Type
// equals shape["event"] (or shape["type"]) and every other key in shape
// matches the corresponding path in event.Data.
func matchEvent(event JournalEvent, shape map[string]any, vars *matcherVars) bool {
	for key, want := range shape {
		switch key {
		case "event", "type":
			wantType, ok := want.(string)
			if !ok || event.Type != wantType {
				return false
			}
		case "file":
			wantFile, ok := want.(string)
			if !ok || event.File != wantFile {
				return false
			}
		case "turn":
			if !matchScalar(event.Turn, want, vars) {
				return false
			}
		case "seq":
			if !matchScalar(event.Seq, want, vars) {
				return false
			}
		default:
			// Path-match into event.Data — dotted paths supported.
			got, found := lookupPath(event.Data, key)
			if !found {
				if !isExistsAssertion(want, false) {
					return false
				}
				continue
			}
			if !matchValue(got, want, vars) {
				return false
			}
		}
	}
	return true
}

// matchValue is the central matcher. Handles literal, "any", "$VAR", and the
// map-form matchers ({regex, contains, exists}). Recurses through maps and
// lists so nested shapes like
// `output: {__spawn_dimensions: {permissions: {allowed_tools: $SAFE_TOOL_SUBSET}}}`
// just work.
func matchValue(got, want any, vars *matcherVars) bool {
	switch w := want.(type) {
	case string:
		// Special string sentinels.
		if w == "any" {
			return true
		}
		if strings.HasPrefix(w, "$") {
			if resolved, ok := vars.resolve(strings.TrimPrefix(w, "$")); ok {
				return matchValue(got, resolved, vars)
			}
			// Unresolved $VAR — treat as literal so authors get a clear miss.
			return reflect.DeepEqual(got, w)
		}
		// Plain string equality.
		gs, ok := got.(string)
		return ok && gs == w
	case map[string]any:
		// Map-form matchers
		if regexPat, ok := w["regex"].(string); ok {
			gs, ok := got.(string)
			if !ok {
				return false
			}
			re, err := regexp.Compile(regexPat)
			return err == nil && re.MatchString(gs)
		}
		if contains, ok := w["contains"]; ok {
			return containsMatch(got, contains, vars)
		}
		if want, ok := w["exists"]; ok {
			wantBool, _ := want.(bool)
			return (got != nil) == wantBool
		}
		// Otherwise: deep-match each key against the corresponding field of
		// got (which must itself be a map).
		gm, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, sub := range w {
			child, found := gm[k]
			if !found {
				return false
			}
			if !matchValue(child, sub, vars) {
				return false
			}
		}
		return true
	case []any:
		// List equality with $VAR expansion per element.
		gl, ok := got.([]any)
		if !ok {
			return false
		}
		if len(gl) != len(w) {
			return false
		}
		for i := range w {
			if !matchValue(gl[i], w[i], vars) {
				return false
			}
		}
		return true
	default:
		// Numeric coercion across YAML-int ↔ JSON-float — both wire formats
		// produce different Go types for the same literal, so a strict
		// reflect.DeepEqual would fail every cross-format compare.
		if gi, gok := toFloat64(got); gok {
			if wi, wok := toFloat64(want); wok {
				return gi == wi
			}
		}
		return reflect.DeepEqual(got, want)
	}
}

// toFloat64 widens any numeric Go type to float64 for cross-format equality.
func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// matchScalar is matchValue with the comfort of allowing int↔int64 coercion
// for the Turn/Seq numeric fields (YAML decodes to int but the struct holds int).
func matchScalar(got, want any, vars *matcherVars) bool {
	if s, ok := want.(string); ok && strings.HasPrefix(s, "$") {
		if resolved, ok := vars.resolve(strings.TrimPrefix(s, "$")); ok {
			return matchScalar(got, resolved, vars)
		}
	}
	if s, ok := want.(string); ok && s == "any" {
		return true
	}
	// Coerce both sides to int64 for numeric compare.
	gi, gok := toInt64(got)
	wi, wok := toInt64(want)
	if gok && wok {
		return gi == wi
	}
	return reflect.DeepEqual(got, want)
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}

func containsMatch(got, want any, vars *matcherVars) bool {
	// String contains.
	if wantStr, ok := want.(string); ok {
		if gotStr, ok := got.(string); ok {
			return strings.Contains(gotStr, wantStr)
		}
	}
	// List contains.
	if gotList, ok := got.([]any); ok {
		for _, item := range gotList {
			if matchValue(item, want, vars) {
				return true
			}
		}
	}
	return false
}

func isExistsAssertion(want any, missingOK bool) bool {
	wm, ok := want.(map[string]any)
	if !ok {
		return false
	}
	v, has := wm["exists"]
	if !has {
		return false
	}
	wantBool, _ := v.(bool)
	return wantBool == missingOK
}

// lookupPath walks a dotted path into a nested map.
// "permissions.allowed_tools" → m["permissions"].(map)["allowed_tools"].
// Returns (value, true) on success or (nil, false) if any segment missing.
func lookupPath(m map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, found := mm[p]
		if !found {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// findSpawnDimensions returns the `__spawn_dimensions` snapshot for a named
// child (or the top-level run when child is empty) by scanning the journal.
//
// Workflow children are nested in their parent's output map by step name.
// A direct child of the top-level run emits its own `task_output` event with
// `task_name=<name>` and the snapshot at `output.__spawn_dimensions`. A
// grandchild (or deeper) doesn't emit its own top-level event — its snapshot
// lives inside the parent workflow's output at `output.<step>.__spawn_dimensions`,
// and at greater depths under nested step keys.
//
// We do two passes: a direct top-level match first (covers UC-053-style direct
// spawns), then a recursive search through every task_output's output map for
// a key matching the child name with a __spawn_dimensions sub-map (covers
// UC-054-style sticky-policy chains through nested workflows).
func findSpawnDimensions(events []JournalEvent, child string) (map[string]any, error) {
	for _, e := range events {
		if e.Type != "task_output" {
			continue
		}
		if child != "" {
			if name, _ := e.Data["task_name"].(string); name != child {
				continue
			}
		}
		output, _ := e.Data["output"].(map[string]any)
		if output == nil {
			continue
		}
		dims, _ := output["__spawn_dimensions"].(map[string]any)
		if dims == nil {
			return nil, fmt.Errorf("task_output for %q has no __spawn_dimensions (simulator config needs `record_context: true`)", child)
		}
		return dims, nil
	}
	if child != "" {
		for _, e := range events {
			if e.Type != "task_output" {
				continue
			}
			output, _ := e.Data["output"].(map[string]any)
			if output == nil {
				continue
			}
			if dims, found := findNestedDimensions(output, child); found {
				return dims, nil
			}
		}
		return nil, fmt.Errorf("no task_output event found for child %q (searched top-level + nested workflow outputs)", child)
	}
	return nil, fmt.Errorf("no task_output event found in journal")
}

// findNestedDimensions walks a workflow output map looking for
// output[child].__spawn_dimensions at any depth. Workflow step outputs are
// keyed by step name, so a grandchild named `g` under mid-worker `m` shows
// up as `output["m"]["g"]["__spawn_dimensions"]` from the top-level
// workflow's task_output. Greater depths recurse the same way.
func findNestedDimensions(output map[string]any, child string) (map[string]any, bool) {
	if v, ok := output[child].(map[string]any); ok {
		if dims, ok := v["__spawn_dimensions"].(map[string]any); ok {
			return dims, true
		}
	}
	for _, v := range output {
		sub, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if dims, found := findNestedDimensions(sub, child); found {
			return dims, found
		}
	}
	return nil, false
}

// matchDimensions walks a dimension assertion shape against the snapshot.
// Returns "" on success, or a human-readable failure description.
//
// Path-absence assertions: use `{exists: false}` as the want value to assert
// a dimension path is NOT present in the snapshot. Used by UC-090 (default
// `IsolationInProcess` is intentionally stripped by serializeOverride, so
// `isolation.mode` must be absent from the child config). Symmetric
// `{exists: true}` is also accepted — declares presence without binding a
// value.
func matchDimensions(snapshot map[string]any, shape map[string]any, vars *matcherVars) string {
	for k, want := range shape {
		if k == "child" {
			continue // identification field, not a value to match
		}
		got, found := lookupPath(snapshot, k)
		if existsAssertion, ok := wantExistsAssertion(want); ok {
			if existsAssertion && !found {
				return fmt.Sprintf("dimension %q: {exists: true} but path absent (have: %v)", k, mapKeys(snapshot))
			}
			if !existsAssertion && found {
				return fmt.Sprintf("dimension %q: {exists: false} but path present with value %v", k, got)
			}
			continue
		}
		if !found {
			return fmt.Sprintf("dimension %q not present in snapshot (have: %v)", k, mapKeys(snapshot))
		}
		if !matchValue(got, want, vars) {
			return fmt.Sprintf("dimension %q: got %v, want %v", k, got, want)
		}
	}
	return ""
}

// wantExistsAssertion recognises `{exists: <bool>}` shape. Returns
// (boolValue, true) if the want is exactly that shape with a single
// boolean field; otherwise (_, false).
func wantExistsAssertion(want any) (bool, bool) {
	m, ok := want.(map[string]any)
	if !ok {
		return false, false
	}
	if len(m) != 1 {
		return false, false
	}
	v, has := m["exists"]
	if !has {
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
