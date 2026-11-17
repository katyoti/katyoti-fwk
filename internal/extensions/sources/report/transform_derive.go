// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"strings"
)

// deriveColumn computes a new column from existing fields.
//
// Params:
//   - column: string — new column ID
//   - label: string — new column label (defaults to column ID)
//   - op: string — operation: "concat", "count_lines", "copy", "literal"
//   - fields: []string — source field IDs (for concat)
//   - field: string — source field ID (for count_lines, copy)
//   - separator: string — separator for concat (default: " ")
//   - value: string — literal value (for literal op)
func deriveColumn(input *StepResult, params map[string]any) (*StepResult, error) {
	colID, ok := params["column"].(string)
	if !ok || colID == "" {
		return nil, fmt.Errorf("derive-column: missing 'column' param")
	}
	op, ok := params["op"].(string)
	if !ok || op == "" {
		return nil, fmt.Errorf("derive-column: missing 'op' param")
	}

	colLabel := colID
	if v, ok := params["label"].(string); ok && v != "" {
		colLabel = v
	}

	deriver, err := buildDeriver(op, params)
	if err != nil {
		return nil, fmt.Errorf("derive-column: %w", err)
	}

	rows := make([]map[string]string, len(input.Rows))
	for i, row := range input.Rows {
		// Copy existing row
		newRow := make(map[string]string, len(row)+1)
		for k, v := range row {
			newRow[k] = v
		}
		newRow[colID] = deriver(row)
		rows[i] = newRow
	}

	// Append new column to existing columns
	cols := make([]Column, len(input.Columns), len(input.Columns)+1)
	copy(cols, input.Columns)
	cols = append(cols, Column{ID: colID, Label: colLabel})

	return &StepResult{
		Rows:    rows,
		Columns: cols,
	}, nil
}

func buildDeriver(op string, params map[string]any) (func(map[string]string) string, error) {
	switch op {
	case "concat":
		srcFields, err := toStringSlice(params["fields"])
		if err != nil {
			return nil, fmt.Errorf("'fields' param: %w", err)
		}
		sep := " "
		if v, ok := params["separator"].(string); ok {
			sep = v
		}
		return func(row map[string]string) string {
			parts := make([]string, 0, len(srcFields))
			for _, f := range srcFields {
				if v := row[f]; v != "" {
					parts = append(parts, v)
				}
			}
			return strings.Join(parts, sep)
		}, nil

	case "count_lines":
		field, ok := params["field"].(string)
		if !ok || field == "" {
			return nil, fmt.Errorf("'field' param required for count_lines")
		}
		return func(row map[string]string) string {
			v := row[field]
			if v == "" {
				return "0"
			}
			return fmt.Sprintf("%d", len(splitMultiline(v)))
		}, nil

	case "copy":
		field, ok := params["field"].(string)
		if !ok || field == "" {
			return nil, fmt.Errorf("'field' param required for copy")
		}
		return func(row map[string]string) string {
			return row[field]
		}, nil

	case "literal":
		value, _ := params["value"].(string)
		return func(row map[string]string) string {
			return value
		}, nil

	default:
		return nil, fmt.Errorf("unknown op %q", op)
	}
}
