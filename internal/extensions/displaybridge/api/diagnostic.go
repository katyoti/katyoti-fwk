// SPDX-License-Identifier: Apache-2.0

// Package api defines the public surface of the displaybridge — the Go-side
// service that bridges Go consumers to the ktw-display TypeScript sidecar.
//
// Under refined Architecture C (locked 2026-05-01), TS owns the schema-semantic
// core (DSL parser, AST, codegen, rendering); Go owns the runtime executor.
// This bridge is the Phase-1 stub that lets Go consumers call ktw-display's
// API surface as if it were a native Go-side KTSP service. Internally each
// forward slot translates to an HTTP call to the bun sidecar.
//
// Phase 2 (ms07+) replaces the HTTP transport with native KTSP from the bun
// side; the Go-side interface here stays unchanged.
//
// Authoritative spec: modules/display/displaybridge/concepts.md and
// modules/framework/schema/architecture.md
package api

// Severity classifies a diagnostic finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Diagnostic carries a single parser, validator, or runtime finding from the
// bun sidecar. Path is a dot/bracket-separated locator into the schema
// (e.g. `types.User.fields.email`). Code is a stable identifier suitable for
// programmatic handling.
//
// Wire-shape mirrors the bun side's Diagnostic shape so JSON travels lossless.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code,omitempty"`
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
