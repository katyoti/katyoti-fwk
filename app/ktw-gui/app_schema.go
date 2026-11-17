// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"

	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// SchemaASTResult is the Wails-facing return of GetSchemaAST. On
// success `AST` carries the canonical Schema AST as raw JSON bytes
// (the shape `@ktw/display-lib`'s `<UIFormView>` consumes directly);
// on failure `Error` carries the substrate's actual message so the
// frontend surfaces it inline rather than guessing.
//
// First slice of US-S1 Phase 1 (2026-06-03).
type SchemaASTResult struct {
	AST   json.RawMessage `json:"ast,omitempty"`
	Error string          `json:"error,omitempty"`
}

// GetSchemaAST resolves a data-type URI (e.g.
// `ktw-demo:/data-type/task/hello-world/input`) into the canonical
// Schema AST with the requested extensions already filtered.
//
// Routing: one round trip to `ktw-hub:schema/get-data-type-with-extensions`.
// The hub's SchemaService is the universe-wide schema authority — it
// pre-registered every catalog data-type at Phase 2, so this is a
// pure in-memory lookup, no per-sidecar spawn, no client-side
// extension walk.
//
// extensionType / extensionName filter the type-level extensions on
// the returned AST. The frontend's WidgetPanel passes
// (`"ui-form"`, `"default"`) so it gets exactly the variant it will
// render; passing (`"*"`, `"*"`) returns all extensions.
//
// Wails-bound: window.go.main.App.GetSchemaAST(universeName,
//
//	schemaURI, extensionType, extensionName).
func (a *App) GetSchemaAST(universeName, schemaURI, extensionType, extensionName string) SchemaASTResult {
	if schemaURI == "" {
		return SchemaASTResult{Error: "schema URI is required"}
	}
	if extensionType == "" {
		extensionType = "*"
	}
	if extensionName == "" {
		extensionName = "*"
	}

	sender, err := a.universeSender(universeName)
	if err != nil {
		return SchemaASTResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}

	in := schemaapi.GetDataTypeWithExtensionsInput{
		URI: schemaURI,
		Extensions: []schemaapi.ExtensionRef{{
			Type: extensionType,
			Name: extensionName,
		}},
	}
	respRaw, err := sender.Call(a.ctx, "ktw-hub:schema/get-data-type-with-extensions", in)
	if err != nil {
		return SchemaASTResult{Error: err.Error()}
	}
	var resp schemaapi.GetDataTypeOutput
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return SchemaASTResult{Error: fmt.Sprintf("Decode GetDataTypeWithExtensions response: %v", err)}
	}
	if resp.Schema == nil {
		return SchemaASTResult{Error: fmt.Sprintf(
			"SchemaService returned nil schema for %q — registration may have failed at hub Phase 2 (check hub logs for the URI)", schemaURI)}
	}

	// Choices computed when the form is served (`options_from`): a field
	// declaring a source gets its options from the universe — the themes
	// in reach, not a list written in the schema.
	if cat, err := a.universeCatalog(universeName); err == nil {
		resolveFormOptions(a.formOptionsCtx(universeName), resp.Schema, cat)
	}

	// Re-marshal the AST to canonical JSON so the frontend gets the
	// exact byte shape the renderers consume.
	astBytes, err := json.Marshal(resp.Schema)
	if err != nil {
		return SchemaASTResult{Error: fmt.Sprintf("Re-marshal schema AST: %v", err)}
	}
	return SchemaASTResult{AST: astBytes}
}

// SchemaViolation is one server-side validation finding, flattened
// from the substrate's Diagnostic shape so the frontend can map it
// to a form field via Path without knowing the Go types.
type SchemaViolation struct {
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// ValidateSchemaResult is the Wails-facing return of
// ValidateAgainstSchema. `Error` is transport/lookup failure only —
// validation findings (including failures) come back as Violations
// with Valid=false, so the frontend can distinguish "couldn't
// validate" from "validated and rejected".
type ValidateSchemaResult struct {
	Valid      bool              `json:"valid"`
	Violations []SchemaViolation `json:"violations,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// ValidateAgainstSchema validates a JSON value against a registered
// data-type URI via the hub's SchemaService — the server-side gate
// the schema-driven task launcher runs BEFORE dispatching the task
// (client-side react-hook-form validation is necessary but not
// sufficient: the substrate's validator is the contract authority).
//
// Routing: one round trip to `ktw-hub:schema/validate-against`.
//
// Wails-bound: window.go.main.App.ValidateAgainstSchema(universeName,
//
//	schemaURI, valueJSON).
func (a *App) ValidateAgainstSchema(universeName, schemaURI, valueJSON string) ValidateSchemaResult {
	if schemaURI == "" {
		return ValidateSchemaResult{Error: "schema URI is required"}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return ValidateSchemaResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	respRaw, err := sender.Call(a.ctx, "ktw-hub:schema/validate-against",
		schemaapi.ValidateAgainstInput{URI: schemaURI, Body: json.RawMessage(valueJSON)})
	if err != nil {
		return ValidateSchemaResult{Error: err.Error()}
	}
	var resp coreschemaapi.ValidateOutput
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return ValidateSchemaResult{Error: fmt.Sprintf("Decode ValidateAgainst response: %v", err)}
	}
	out := ValidateSchemaResult{Valid: resp.Valid}
	for _, d := range resp.Diagnostics {
		out.Violations = append(out.Violations, SchemaViolation{
			Severity: string(d.Severity),
			Code:     d.Code,
			Message:  d.Message,
			Path:     d.Path,
		})
	}
	return out
}

// JSONSchemaResult is the Wails-facing return of GetJSONSchema. On
// success `Schema` carries the compiled JSON Schema 2020-12 bytes
// (the shape the local SchemaForm + SchemaView consume directly);
// on failure `Error` carries the substrate's actual message so the
// frontend surfaces it inline rather than guessing.
//
// cwm-v3 US-4 (2026-06-07).
type JSONSchemaResult struct {
	Schema json.RawMessage `json:"schema,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// GetJSONSchema resolves a data-type URI into compiled JSON Schema
// 2020-12 bytes. Pair this with GetSchemaAST: use GetSchemaAST when
// you need the canonical AST shape with extensions; use this when
// you just need to feed a JSON-Schema-consuming widget (the local
// SchemaForm / SchemaView in NodeEditorTab + NodeTab).
//
// Routing: one round trip to `ktw-hub:schema/get-json-schema`. The
// hub's SchemaService is the universe-wide schema authority — every
// catalog data-type was pre-registered at Phase 2, so this is a
// pure in-memory lookup + AST → JSON Schema compile, no per-sidecar
// spawn.
//
// Wails-bound: window.go.main.App.GetJSONSchema(universeName, schemaURI).
func (a *App) GetJSONSchema(universeName, schemaURI string) JSONSchemaResult {
	if schemaURI == "" {
		return JSONSchemaResult{Error: "schema URI is required"}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return JSONSchemaResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	respRaw, err := sender.Call(a.ctx, "ktw-hub:schema/get-json-schema",
		schemaapi.GetJSONSchemaInput{URI: schemaURI})
	if err != nil {
		return JSONSchemaResult{Error: err.Error()}
	}
	var resp schemaapi.GetJSONSchemaOutput
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return JSONSchemaResult{Error: fmt.Sprintf("Decode GetJSONSchema response: %v", err)}
	}
	if len(resp.Schema) == 0 {
		return JSONSchemaResult{Error: fmt.Sprintf(
			"SchemaService returned empty schema for %q — registration may have failed at hub Phase 2 (check hub logs)", schemaURI)}
	}
	return JSONSchemaResult{Schema: resp.Schema}
}
