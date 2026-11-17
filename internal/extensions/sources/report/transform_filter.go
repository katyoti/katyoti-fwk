// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"strings"
)

// filterRows keeps or excludes rows matching a condition.
//
// Params:
//   - field: string — field ID to check
//   - op: string — operator: "eq", "neq", "contains", "not_contains", "empty", "not_empty"
//   - value: string — value to compare against (not needed for empty/not_empty)
//   - exclude: bool — if true, removes matching rows instead of keeping them (default: false)
func filterRows(input *StepResult, params map[string]any) (*StepResult, error) {
	field, ok := params["field"].(string)
	if !ok || field == "" {
		return nil, fmt.Errorf("filter-rows: missing 'field' param")
	}
	op, ok := params["op"].(string)
	if !ok || op == "" {
		return nil, fmt.Errorf("filter-rows: missing 'op' param")
	}

	value, _ := params["value"].(string)
	exclude, _ := params["exclude"].(bool)

	matcher, err := buildMatcher(op, value)
	if err != nil {
		return nil, fmt.Errorf("filter-rows: %w", err)
	}

	var rows []map[string]string
	for _, row := range input.Rows {
		matches := matcher(row[field])
		if exclude {
			matches = !matches
		}
		if matches {
			rows = append(rows, row)
		}
	}

	// Preserve columns from input
	cols := input.Columns
	if cols == nil {
		cols = []Column{}
	}

	return &StepResult{
		Rows:    rows,
		Columns: cols,
	}, nil
}

func buildMatcher(op, value string) (func(string) bool, error) {
	switch op {
	case "eq":
		return func(v string) bool { return v == value }, nil
	case "neq":
		return func(v string) bool { return v != value }, nil
	case "contains":
		return func(v string) bool { return strings.Contains(v, value) }, nil
	case "not_contains":
		return func(v string) bool { return !strings.Contains(v, value) }, nil
	case "empty":
		return func(v string) bool { return v == "" }, nil
	case "not_empty":
		return func(v string) bool { return v != "" }, nil
	default:
		return nil, fmt.Errorf("unknown operator %q", op)
	}
}
