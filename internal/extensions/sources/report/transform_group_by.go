// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"sort"
	"strings"
)

// groupByField is a named group-by field with a display label.
type groupByField struct {
	Name  string // Display label for direction columns (e.g., "Supplier")
	Field string // Row key prefix (e.g., "supplier")
}

// parseGroupByFields parses group_by param as [{name, field}] objects.
func parseGroupByFields(raw any) ([]groupByField, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", raw)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("'group_by' is empty")
	}
	result := make([]groupByField, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("element %d: expected {name, field} object, got %T", i, item)
		}
		name, _ := m["name"].(string)
		field, _ := m["field"].(string)
		if field == "" {
			return nil, fmt.Errorf("element %d: missing 'field'", i)
		}
		if name == "" {
			name = shortLabel(field)
		}
		result[i] = groupByField{Name: name, Field: field}
	}
	return result, nil
}

// groupByRelation aggregates rows by one or more fields, collecting related values.
//
// Params:
//   - group_by: []{name, field} — fields to group by, each with a display name.
//   - collect: []string — field IDs whose values are collected into sets per group.
//   - id_suffix: string — suffix for the ID sub-field (default: ".id")
//   - name_suffix: string — suffix for the name sub-field (default: ".displayName")
func groupByRelation(input *StepResult, params map[string]any) (*StepResult, error) {
	groupByRaw, ok := params["group_by"]
	if !ok {
		return nil, fmt.Errorf("group-by-relation: missing 'group_by' param")
	}
	groupByFields, err := parseGroupByFields(groupByRaw)
	if err != nil {
		return nil, fmt.Errorf("group-by-relation: 'group_by' %w", err)
	}

	collectRaw, ok := params["collect"]
	if !ok {
		return nil, fmt.Errorf("group-by-relation: missing 'collect' param")
	}
	collectFields, err := toStringSlice(collectRaw)
	if err != nil {
		return nil, fmt.Errorf("group-by-relation: 'collect' %w", err)
	}

	idSuffix := ".id"
	if v, ok := params["id_suffix"].(string); ok && v != "" {
		idSuffix = v
	}
	nameSuffix := ".displayName"
	if v, ok := params["name_suffix"].(string); ok && v != "" {
		nameSuffix = v
	}

	// Determine the direction label for each group_by field.
	// E.g., a "supplier" group_by field contributes the items where this entity
	// is the supplier, a "customer" one those where it is the customer.
	// We use a simplified model: each group_by field creates entries tagged with
	// which direction the relationship goes.
	type groupEntry struct {
		id          string
		displayName string
		collections map[string]map[string]bool // collectField → set of values
		directions  map[string]map[string]bool // direction → collectField → set of values
	}

	groups := make(map[string]*groupEntry) // keyed by entity ID (or name if no ID)

	for _, row := range input.Rows {
		for _, gbField := range groupByFields {
			// Get ID and name values (multiline)
			idVal := row[gbField.Field+idSuffix]
			nameVal := row[gbField.Field+nameSuffix]
			// Fallback: if no id/name suffix fields, use the field directly
			if idVal == "" && nameVal == "" {
				nameVal = row[gbField.Field]
			}

			ids := splitMultiline(idVal)
			names := splitMultiline(nameVal)

			// Align ids and names (use name as ID fallback)
			count := len(names)
			if len(ids) > count {
				count = len(ids)
			}

			for i := 0; i < count; i++ {
				var entID, entName string
				if i < len(ids) {
					entID = ids[i]
				}
				if i < len(names) {
					entName = names[i]
				}
				if entID == "" && entName == "" {
					continue
				}

				key := entID
				if key == "" {
					key = entName
				}

				g, exists := groups[key]
				if !exists {
					g = &groupEntry{
						id:          entID,
						displayName: entName,
						collections: make(map[string]map[string]bool),
						directions:  make(map[string]map[string]bool),
					}
					groups[key] = g
				}

				// Collect values for each collect field
				for _, cf := range collectFields {
					if g.collections[cf] == nil {
						g.collections[cf] = make(map[string]bool)
					}
					vals := splitMultiline(row[cf])
					for _, v := range vals {
						if v != "" {
							g.collections[cf][v] = true
						}
					}
				}

				// Track direction: which group_by field contributed this entry
				dirKey := gbField.Field
				if g.directions[dirKey] == nil {
					g.directions[dirKey] = make(map[string]bool)
				}
				// Use first collect field as the per-row reference
				if len(collectFields) > 0 {
					ref := row[collectFields[0]]
					if ref != "" {
						g.directions[dirKey][ref] = true
					}
				}
			}
		}
	}

	// Build output rows
	var rows []map[string]string
	for _, g := range groups {
		row := map[string]string{
			"id":          g.id,
			"displayName": g.displayName,
		}

		// Add per-direction columns if multiple group_by fields
		if len(groupByFields) > 1 {
			for _, gbField := range groupByFields {
				refs := g.directions[gbField.Field]
				row[gbField.Name+".count"] = fmt.Sprintf("%d", len(refs))
				row[gbField.Name+".values"] = sortedSetJoin(refs)
			}
		}

		// Add collected values
		for _, cf := range collectFields {
			vals := g.collections[cf]
			label := shortLabel(cf)
			row[label+".count"] = fmt.Sprintf("%d", len(vals))
			row[label+".values"] = sortedSetJoin(vals)
		}

		rows = append(rows, row)
	}

	// Sort by displayName
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["displayName"] < rows[j]["displayName"]
	})

	// Build columns
	cols := []Column{
		{ID: "id", Label: "ID"},
		{ID: "displayName", Label: "Display Name"},
	}
	if len(groupByFields) > 1 {
		for _, gbField := range groupByFields {
			cols = append(cols,
				Column{ID: gbField.Name + ".count", Label: gbField.Name + " Count"},
				Column{ID: gbField.Name + ".values", Label: gbField.Name + " Values"},
			)
		}
	}
	for _, cf := range collectFields {
		label := shortLabel(cf)
		cols = append(cols,
			Column{ID: label + ".count", Label: label + " Count"},
			Column{ID: label + ".values", Label: label + " Values"},
		)
	}

	return &StepResult{
		Rows:    rows,
		Columns: cols,
	}, nil
}

// splitMultiline splits a string by newlines and trims whitespace.
func splitMultiline(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// sortedSetJoin converts a set to a sorted, newline-joined string.
func sortedSetJoin(set map[string]bool) string {
	if len(set) == 0 {
		return ""
	}
	vals := make([]string, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	return strings.Join(vals, "\n")
}

// shortLabel extracts a short label from a field ID.
// "owner.name" → "owner"
// "flow.code" → "flow"
func shortLabel(fieldID string) string {
	if idx := strings.IndexByte(fieldID, '.'); idx > 0 {
		return fieldID[:idx]
	}
	return fieldID
}

// toStringSlice converts an any (expected []any or []string) to []string.
func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		result := make([]string, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d is not a string", i)
			}
			result[i] = s
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string array, got %T", v)
	}
}
