// SPDX-License-Identifier: Apache-2.0

package schemaapi

// Severity classifies a parser / validator finding.
//
// Lives in core/schema/api so any core-side consumer (the parser
// internals, the contribution layer, downstream generators) can produce
// diagnostics without taking a dependency on the SchemaService wire
// surface that lives in `internal/extensions/schema/api`.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Diagnostic carries a single parser or validator finding. Path is a
// dot/bracket-separated locator into the schema (e.g. `types.User.fields.email`).
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code,omitempty"` // stable error code (e.g. "ORPHAN_REF")
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
}

// Error builds an error diagnostic.
func Error(code, msg, path string) Diagnostic {
	return Diagnostic{Severity: SeverityError, Code: code, Message: msg, Path: path}
}

// Warning builds a warning diagnostic.
func Warning(code, msg, path string) Diagnostic {
	return Diagnostic{Severity: SeverityWarning, Code: code, Message: msg, Path: path}
}
