// SPDX-License-Identifier: Apache-2.0

package formdef

import "fmt"

// ConvertResult holds the JSON Schema and UI Schema produced from a FormDefinition.
type ConvertResult struct {
	JSONSchema map[string]any
	UISchema   map[string]any
}

// Convert transforms a FormDefinition into JSON Schema + UI Schema
// suitable for JSON Forms rendering.
func Convert(def *FormDefinition) (*ConvertResult, error) {
	js, err := toJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("json schema: %w", err)
	}
	ui, err := toUISchema(def)
	if err != nil {
		return nil, fmt.Errorf("ui schema: %w", err)
	}
	return &ConvertResult{JSONSchema: js, UISchema: ui}, nil
}

// --- JSON Schema conversion ---

func toJSONSchema(def *FormDefinition) (map[string]any, error) {
	fields := def.Fields
	if len(def.Groups) > 0 {
		for _, g := range def.Groups {
			fields = append(fields, g.Fields...)
		}
	}
	return fieldsToSchema(fields)
}

func fieldsToSchema(fields []Field) (map[string]any, error) {
	props := map[string]any{}
	var required []any

	for _, f := range fields {
		prop, err := fieldToSchemaProperty(f)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Key, err)
		}
		props[f.Key] = prop
		if f.Required {
			required = append(required, f.Key)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, nil
}

func fieldToSchemaProperty(f Field) (map[string]any, error) {
	prop := map[string]any{}

	switch f.Type {
	case "string":
		prop["type"] = "string"
	case "text":
		prop["type"] = "string"
	case "markdown":
		prop["type"] = "string"
	case "integer":
		prop["type"] = "integer"
	case "number":
		prop["type"] = "number"
	case "boolean":
		prop["type"] = "boolean"
	case "enum":
		prop["type"] = "string"
		// Use oneOf with const+title so JSON Forms displays labels, not raw values
		oneOf := make([]any, len(f.Options))
		for i, o := range f.Options {
			entry := map[string]any{
				"const": o.Value,
				"title": o.Label,
			}
			oneOf[i] = entry
		}
		prop["oneOf"] = oneOf
	case "date":
		prop["type"] = "string"
		prop["format"] = "date"
	case "time":
		prop["type"] = "string"
		prop["format"] = "time"
	case "datetime":
		prop["type"] = "string"
		prop["format"] = "date-time"
	case "object":
		sub, err := fieldsToSchema(f.Fields)
		if err != nil {
			return nil, err
		}
		return sub, nil
	case "array":
		itemProp, err := fieldToSchemaProperty(*f.Items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		prop["type"] = "array"
		prop["items"] = itemProp
	case "table":
		// Table is an array of objects, where columns define the object properties
		colSchema, err := fieldsToSchema(f.Columns)
		if err != nil {
			return nil, fmt.Errorf("columns: %w", err)
		}
		prop["type"] = "array"
		prop["items"] = colSchema
	default:
		return nil, fmt.Errorf("unsupported type %q", f.Type)
	}

	// Apply validation constraints
	if f.Validation != nil {
		applyValidation(prop, f.Validation)
	}
	if f.Default != nil {
		prop["default"] = f.Default
	}
	if f.Description != "" {
		prop["description"] = f.Description
	}
	if f.ReadOnly {
		prop["readOnly"] = true
	}

	return prop, nil
}

func applyValidation(prop map[string]any, v *Validation) {
	if v.MinLength != nil {
		prop["minLength"] = *v.MinLength
	}
	if v.MaxLength != nil {
		prop["maxLength"] = *v.MaxLength
	}
	if v.Minimum != nil {
		prop["minimum"] = *v.Minimum
	}
	if v.Maximum != nil {
		prop["maximum"] = *v.Maximum
	}
	if v.Pattern != "" {
		prop["pattern"] = v.Pattern
	}
	if v.Format != "" {
		prop["format"] = v.Format
	}
}

// --- UI Schema conversion ---

func toUISchema(def *FormDefinition) (map[string]any, error) {
	if len(def.Groups) > 0 {
		return groupsToUISchema(def.Groups), nil
	}
	return fieldsToUILayout(def.Fields, "#/properties"), nil
}

func groupsToUISchema(groups []Group) map[string]any {
	categories := make([]any, len(groups))
	for i, g := range groups {
		categories[i] = map[string]any{
			"type":     "Category",
			"label":    g.Label,
			"elements": fieldUIElements(g.Fields, "#/properties"),
		}
	}
	return map[string]any{
		"type":     "Categorization",
		"elements": categories,
	}
}

func fieldsToUILayout(fields []Field, scopePrefix string) map[string]any {
	return map[string]any{
		"type":     "VerticalLayout",
		"elements": fieldUIElements(fields, scopePrefix),
	}
}

func fieldUIElements(fields []Field, scopePrefix string) []any {
	elements := make([]any, 0, len(fields))
	for _, f := range fields {
		elements = append(elements, fieldToUIElement(f, scopePrefix))
	}
	return elements
}

func fieldToUIElement(f Field, scopePrefix string) map[string]any {
	scope := fmt.Sprintf("%s/%s", scopePrefix, f.Key)

	elem := map[string]any{
		"type":  "Control",
		"scope": scope,
	}
	if f.Label != "" {
		elem["label"] = f.Label
	}

	// Type-specific UI options
	opts := map[string]any{}
	switch f.Type {
	case "text":
		opts["multi"] = true
	case "markdown":
		opts["multi"] = true
		opts["format"] = "markdown"
	case "enum":
		if f.Display == "radio" {
			opts["format"] = "radio"
			// Pass option descriptions for custom radio renderer
			hasDesc := false
			for _, o := range f.Options {
				if o.Description != "" {
					hasDesc = true
					break
				}
			}
			if hasDesc {
				descs := make([]any, len(f.Options))
				for i, o := range f.Options {
					descs[i] = map[string]any{
						"value":       o.Value,
						"label":       o.Label,
						"description": o.Description,
					}
				}
				opts["optionDetails"] = descs
			}
		}
	case "date":
		opts["format"] = "date"
	case "time":
		opts["format"] = "time"
	case "datetime":
		opts["format"] = "date-time"
	case "table":
		opts["format"] = "table"
	case "object":
		// For object fields, create a Group layout with nested controls
		subElements := fieldUIElements(f.Fields, scope+"/properties")
		elem = map[string]any{
			"type":     "Group",
			"label":    f.Label,
			"elements": subElements,
		}
		return elem
	}

	if f.Placeholder != "" {
		opts["placeholder"] = f.Placeholder
	}
	if f.Description != "" {
		opts["description"] = f.Description
	}
	if len(opts) > 0 {
		elem["options"] = opts
	}

	return elem
}
