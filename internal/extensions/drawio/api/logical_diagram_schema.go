// SPDX-License-Identifier: Apache-2.0

package api

import _ "embed"

//go:embed logical_diagram.schema.json
var logicalDiagramSchemaJSON []byte

// LogicalDiagramSchema returns the JSON Schema (Draft 2020-12) for the
// LogicalDiagram wire format. Use it for runtime validation in CLI
// tools, IDE pluggers, or any consumer that wants to check an LLM's
// emitted JSON against the contract before passing it to the renderer.
//
// The bytes are an embedded copy of `logical_diagram.schema.json` —
// stable across reads, no I/O.
func LogicalDiagramSchema() []byte {
	return logicalDiagramSchemaJSON
}
