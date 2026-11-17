// SPDX-License-Identifier: Apache-2.0

// Package core provides the concrete SchemaService implementation.
// Two cooperating internal roles:
//
//  1. Stateless parser methods (Parse, ParseFile, Validate) delegate to
//     the parser package; they're pure functions.
//
//  2. Stateful data-type store methods (RegisterDataType, GetDataType,
//     GetJSONSchema, ValidateAgainst) operate on an in-memory
//     dataTypeStore.
//
// Rendering is intentionally NOT a SchemaService responsibility — frontends
// consume GetDataType + the ui-* extensions on the AST and render in their
// own context (terminal width, ANSI capability, browser DOM, etc.). See
// modules/display/tui/ for the terminal renderer.
//
// The service is the single implementation of api.SchemaService; both
// hub-side bootstrap factories and standalone callers consume it.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// New returns a fresh SchemaService with an empty data-type store.
// Parser methods are immediately available; data-type methods require
// RegisterDataType to be called first (typically by the hub bootstrap
// factory).
func New(_ context.Context) (api.SchemaService, error) {
	return &service{store: newDataTypeStore()}, nil
}

// service implements api.SchemaService.
type service struct {
	store *dataTypeStore
}

// === Stateless parser methods ===

// Parse reads a YAML or JSON DSL schema from the input bytes.
func (s *service) Parse(ctx context.Context, in schemaapi.ParseInput) (schemaapi.ParseOutput, error) {
	return parser.Parse(ctx, in)
}

// ParseFile reads and parses a schema file from disk.
func (s *service) ParseFile(ctx context.Context, in schemaapi.ParseFileInput) (schemaapi.ParseOutput, error) {
	return parser.ParseFile(ctx, in)
}

// Validate runs the AST validator on a previously-parsed (or programmatically
// constructed) schema.
func (s *service) Validate(_ context.Context, in schemaapi.ValidateInput) (schemaapi.ValidateOutput, error) {
	if in.Schema == nil {
		return schemaapi.ValidateOutput{Valid: false}, nil
	}
	diags := parser.RunValidator(in.Schema)
	valid := true
	for _, d := range diags {
		if d.Severity == schemaapi.SeverityError {
			valid = false
			break
		}
	}
	return schemaapi.ValidateOutput{Valid: valid, Diagnostics: diags}, nil
}

// === Stateful data-type store methods ===

// RegisterDataType parses the body as a DSL schema and inserts it into
// the in-memory store keyed by URI. Hub-internal — called by the
// bootstrap factory + refresh path.
func (s *service) RegisterDataType(ctx context.Context, in api.RegisterDataTypeInput) error {
	if in.URI == "" {
		return fmt.Errorf("schema.RegisterDataType: URI required")
	}
	if len(in.Body) == 0 {
		return fmt.Errorf("schema.RegisterDataType: Body required for %q", in.URI)
	}
	out, err := parser.Parse(ctx, schemaapi.ParseInput{Body: in.Body})
	if err != nil {
		return fmt.Errorf("schema.RegisterDataType %q: parse: %w", in.URI, err)
	}
	if out.Schema == nil {
		return fmt.Errorf("schema.RegisterDataType %q: parser returned nil schema", in.URI)
	}
	s.store.register(in.URI, out.Schema)
	return nil
}

// GetDataType returns the canonical AST of a registered data-type.
func (s *service) GetDataType(_ context.Context, in api.GetDataTypeInput) (api.GetDataTypeOutput, error) {
	schema, err := s.store.get(in.URI)
	if err != nil {
		return api.GetDataTypeOutput{}, err
	}
	return api.GetDataTypeOutput{Schema: schema}, nil
}

// GetJSONSchema returns the compiled JSON Schema 2020-12 artifact for
// a registered data-type. Lazy-compiles on first request; cached.
func (s *service) GetJSONSchema(_ context.Context, in api.GetJSONSchemaInput) (api.GetJSONSchemaOutput, error) {
	js, err := s.store.getJSONSchema(in.URI)
	if err != nil {
		return api.GetJSONSchemaOutput{}, err
	}
	return api.GetJSONSchemaOutput{Schema: js}, nil
}

// GetDataTypeWithExtensions returns the AST with type-level extensions
// filtered to the requested (type, name) allowlist. Wildcards: any
// filter field set to "*" matches all values; an empty filter list
// strips every extension.
//
// The result is a SHALLOW CLONE of the registered schema so that
// filtering doesn't mutate the shared in-memory store. Only the parts
// we rewrite (the Types map and each Type's Extensions slice) are
// fresh; nested pointers (fields, content params, etc.) are shared
// with the registered original and remain immutable per the store's
// contract.
func (s *service) GetDataTypeWithExtensions(_ context.Context, in api.GetDataTypeWithExtensionsInput) (api.GetDataTypeOutput, error) {
	schema, err := s.store.get(in.URI)
	if err != nil {
		return api.GetDataTypeOutput{}, err
	}
	clone := *schema
	if schema.Types != nil {
		clone.Types = make(map[string]*schemaapi.Type, len(schema.Types))
		for name, t := range schema.Types {
			if t == nil {
				clone.Types[name] = nil
				continue
			}
			tCopy := *t
			tCopy.Extensions = filterExtensions(t.Extensions, in.Extensions)
			clone.Types[name] = &tCopy
		}
	}
	return api.GetDataTypeOutput{Schema: &clone}, nil
}

// filterExtensions returns the subset of `all` whose (Kind, Name) is
// matched by at least one entry in `keep`. An entry's Type=="*" matches
// any kind; Name=="*" matches any name.
//
// The Extension's logical "type" is the AST `Extension.Kind` (json
// tag `type`); the "name" lives inside `Extension.Payload["name"]`
// (since the parser folds every non-`type` key into Payload).
//
// A nil/empty `keep` returns nil (no extensions preserved).
func filterExtensions(all []schemaapi.Extension, keep []api.ExtensionRef) []schemaapi.Extension {
	if len(all) == 0 || len(keep) == 0 {
		return nil
	}
	out := make([]schemaapi.Extension, 0, len(all))
	for _, ext := range all {
		extName, _ := ext.Payload["name"].(string)
		for _, ref := range keep {
			if (ref.Type == "*" || ref.Type == ext.Kind) &&
				(ref.Name == "*" || ref.Name == extName) {
				out = append(out, ext)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ValidateAgainst validates a JSON body against a registered data-type.
//
// Depth is pinned by a probe test in internal/test/census: a missing required
// field and a wrong field type are both rejected, alongside the structural
// checks (body is JSON at all, registered schema is well-formed). Change the
// depth and that test fails, so this comment cannot drift from the behaviour
// again.
func (s *service) ValidateAgainst(_ context.Context, in api.ValidateAgainstInput) (schemaapi.ValidateOutput, error) {
	// The data-type's validation state — its AST's diagnostics and its
	// compiled JSON Schema — is built once per registration (getValidator),
	// never per body.
	v, err := s.store.getValidator(in.URI)
	if err != nil {
		return schemaapi.ValidateOutput{}, err
	}
	// First defense: AST is well-formed (the registered schema passed parser validation).
	// Second defense: body unmarshals as JSON at all.
	var probe any
	if err := json.Unmarshal(in.Body, &probe); err != nil {
		return schemaapi.ValidateOutput{
			Valid: false,
			Diagnostics: []schemaapi.Diagnostic{
				schemaapi.Error("INVALID_JSON", err.Error(), ""),
			},
		}, nil
	}
	// Third defense: the registered schema is itself well-formed.
	diags := append([]schemaapi.Diagnostic(nil), v.diags...)
	if !v.astValid {
		// A broken registered schema can't validate a body meaningfully.
		return schemaapi.ValidateOutput{Valid: false, Diagnostics: diags}, nil
	}

	// Fourth defense — the actual body-vs-schema check against the
	// data-type's compiled JSON Schema (2020-12), so missing required fields
	// and type mismatches are caught. Without this, ValidateAgainst only ever
	// confirmed the body was JSON and the schema was sound — never that the
	// body MATCHED.
	if v.compileErr != nil {
		return schemaapi.ValidateOutput{Valid: false, Diagnostics: append(diags,
			schemaapi.Error("SCHEMA_COMPILE", v.compileErr.Error(), ""))}, nil
	}
	if err := v.compiled.Validate(probe); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			diags = append(diags, flattenValidationError(ve)...)
		} else {
			diags = append(diags, schemaapi.Error("VALIDATION", err.Error(), ""))
		}
		return schemaapi.ValidateOutput{Valid: false, Diagnostics: diags}, nil
	}
	return schemaapi.ValidateOutput{Valid: true, Diagnostics: diags}, nil
}

// flattenValidationError walks a santhosh-tekuri ValidationError tree
// and emits one Diagnostic per LEAF cause, using the JSON-pointer
// instance location as the Path (e.g. "/project_code"). Leaf causes
// carry the actionable message ("missing properties: 'project_code'",
// "expected string, but got number"); parent nodes are structural.
func flattenValidationError(ve *jsonschema.ValidationError) []schemaapi.Diagnostic {
	var out []schemaapi.Diagnostic
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			out = append(out, schemaapi.Error("VALIDATION", e.Message, e.InstanceLocation))
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) == 0 {
		out = append(out, schemaapi.Error("VALIDATION", ve.Message, ve.InstanceLocation))
	}
	return out
}
