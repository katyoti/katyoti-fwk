// SPDX-License-Identifier: Apache-2.0

package api

// ObjectBrief is the briefing-shape view of an object, built by object-context
// from the object and its type's `context` section (`briefing_fields`).
//
// Rendered as a one-liner wherever objects are listed. NEVER dump prose here —
// StructuredFields carries only short, typed values (status, severity, …). A
// consumer that needs more reads the object (ObjectGet).
type ObjectBrief struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Title            string            `json:"title,omitempty"`
	Summary          string            `json:"summary,omitempty"      jsonschema:"≤ 300 chars per kind"`
	StructuredFields map[string]string `json:"structured_fields,omitempty" jsonschema:"Structured field values picked by the type's context.briefing_fields"`
	Source           string            `json:"source,omitempty"       jsonschema:"Id of the object that contributed it (local / ancestor / type-injected)"`
}
