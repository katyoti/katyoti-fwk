// SPDX-License-Identifier: Apache-2.0

package engine

import "encoding/json"

// SchemaProvider is an optional interface that MachineFactory implementations
// MAY implement to expose default schemas and body handling.
//
// Task-domain only — not on generic TypeDef.
//
// Hybrid schema model:
//   - Strategy provides defaults via this interface (auto-generated from Go types or hardcoded)
//   - a task's contract files (<name>.input.dsl.yaml / <name>.output.dsl.yaml) override strategy defaults
//   - Resolution: YAML > strategy default > none
//
// Both sides are the ktw-schema DSL — the language every authored schema in the
// universe is written in. A strategy default and a TASK.md block override each
// other, so they have to be the same language; JSON Schema is derived from
// either at the edge that speaks it (schemaparser.CompileJSONSchema).
type SchemaProvider interface {
	// DefaultInputSchema returns the strategy's default DSL schema for input.
	// Returns nil if the strategy has no default input schema.
	// Overridden by the task's input contract file when it has one.
	DefaultInputSchema() json.RawMessage

	// DefaultOutputSchema returns the strategy's default DSL schema for output.
	// Returns nil if the strategy has no default output schema.
	// Overridden by the task's output contract file when it has one.
	DefaultOutputSchema() json.RawMessage

	// BodyFieldName returns the config field name that the TASK.md body maps to.
	// For example: agent returns "prompt", template returns "template",
	// human-gate returns "message".
	// Returns "" if the strategy does not use body content.
	BodyFieldName() string
}

// ResolveInputSchema returns the effective input schema for a task.
// YAML-declared schema (from task) takes precedence over strategy default.
func ResolveInputSchema(factory MachineFactory, taskSchema json.RawMessage) json.RawMessage {
	if len(taskSchema) > 0 {
		return taskSchema
	}
	if sp, ok := factory.(SchemaProvider); ok {
		return sp.DefaultInputSchema()
	}
	return nil
}

// ResolveOutputSchema returns the effective output schema for a task.
// YAML-declared schema (from task) takes precedence over strategy default.
func ResolveOutputSchema(factory MachineFactory, taskSchema json.RawMessage) json.RawMessage {
	if len(taskSchema) > 0 {
		return taskSchema
	}
	if sp, ok := factory.(SchemaProvider); ok {
		return sp.DefaultOutputSchema()
	}
	return nil
}

// GetBodyFieldName returns the body field name from a factory, or "" if
// the factory does not implement SchemaProvider or does not use body.
func GetBodyFieldName(factory MachineFactory) string {
	if sp, ok := factory.(SchemaProvider); ok {
		return sp.BodyFieldName()
	}
	return ""
}
