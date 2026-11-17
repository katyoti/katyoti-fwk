// SPDX-License-Identifier: Apache-2.0

// Package api declares the SchemaService — the wire-side surface that
// publishes the core schema parser + data-type store on KTSP / HTTP /
// Cobra.
//
// The AST types (Schema, Type, …), the parser, the canonical JSON
// serialiser, the JSON Schema 2020-12 compiler, and the
// parse/validate I/O types all live in `internal/core/schema/api`
// (package `schemaapi`). Core consumers (reflect, kts, contribution)
// depend on those directly without crossing into extension territory.
//
// This package keeps only the SchemaService wire surface:
//   - The SchemaService interface (Parse / Validate / data-type store)
//   - The SchemaServiceDef descriptor used by the publish framework
//   - The data-type store input/output structs (Register / Get /
//     Validate / GetWithExtensions)
//   - ErrDataTypeNotFound
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// SchemaService is the contract for the schema extension. It serves two
// roles:
//
//  1. STATELESS PARSER — Parse / ParseFile / Validate convert DSL bytes
//     into canonical AST. These methods are pure functions; the impl
//     delegates to `internal/core/schema/parser`.
//
//  2. STATEFUL DATA-TYPE STORE — RegisterDataType / GetDataType /
//     GetJSONSchema / ValidateAgainst hold parsed data-types keyed by
//     URI and serve them to consumers across the universe. Hub-side
//     ktw-hub uses these at bootstrap to publish the universe-wide
//     data-type authority; see (modules/contributions/decisions/OD-006-in-what-order-does-the-hub-build-its-catalog-and.cwm.md).
//
// Rendering is intentionally OUT OF SCOPE. Frontends call GetDataType,
// read the ui-* extensions off the AST, and render in their own context
// (terminal width / ANSI capability for the TUI, browser DOM for
// ktw-display, etc.). See modules/display/tui/ for the terminal
// renderer.
//
// Consumers pick the methods relevant to their use case; the two roles
// share state internally so register-then-fetch round-trips.
type SchemaService interface {
	// === Stateless parser ===

	// Parse reads a YAML or JSON DSL schema from the input bytes and returns
	// the canonical AST. Format is auto-detected unless ParseInput.Format is
	// set explicitly. Imports are resolved relative to ParseInput.BasePath.
	Parse(ctx context.Context, in schemaapi.ParseInput) (schemaapi.ParseOutput, error)

	// ParseFile is a convenience wrapper that reads a file from disk and
	// parses it. The file's directory becomes the import base path.
	ParseFile(ctx context.Context, in schemaapi.ParseFileInput) (schemaapi.ParseOutput, error)

	// Validate runs the AST validator on a schema produced via Parse or built
	// programmatically. Useful after manual AST edits.
	Validate(ctx context.Context, in schemaapi.ValidateInput) (schemaapi.ValidateOutput, error)

	// === Stateful data-type store ===

	// RegisterDataType ingests a data-type definition into the in-memory
	// store, keyed by its URI. The body is parsed as a DSL Schema; the
	// service holds the canonical AST until the next register-or-clear.
	//
	// Called by ktw-hub's bootstrap factory during Phase 1 of the
	// hub-boot-order protocol.
	// HUB-INTERNAL — not exposed as a KTSP slot method.
	RegisterDataType(ctx context.Context, in RegisterDataTypeInput) error

	// GetDataType returns the canonical AST of a previously-registered
	// data-type by URI. Returns ErrDataTypeNotFound if absent.
	GetDataType(ctx context.Context, in GetDataTypeInput) (GetDataTypeOutput, error)

	// GetJSONSchema returns the compiled JSON Schema 2020-12 artifact for
	// a registered data-type. Compiled on first request, cached.
	GetJSONSchema(ctx context.Context, in GetJSONSchemaInput) (GetJSONSchemaOutput, error)

	// ValidateAgainst validates a JSON body against a registered
	// data-type by URI. Returns the same Diagnostic shape as Validate.
	ValidateAgainst(ctx context.Context, in ValidateAgainstInput) (schemaapi.ValidateOutput, error)

	// GetDataTypeWithExtensions returns the canonical AST of a
	// registered data-type FILTERED to keep only the (type, name)
	// extension pairs the caller asked for. Use this from frontends
	// that already know which `ui-*` variants they will render — one
	// round trip, no client-side extension walk, no ambient extension
	// payload.
	//
	// Wildcards: an Extensions entry with Type=="*" matches any type;
	// Name=="*" matches any name. Use [{Type:"ui-form", Name:"*"}] to
	// keep every ui-form variant; [{Type:"*", Name:"*"}] is equivalent
	// to GetDataType (everything).
	//
	// An empty Extensions list returns the schema with NO extensions
	// (just the type structure) — useful for raw-shape rendering
	// where the caller will choose a default widget themselves.
	//
	// Returns ErrDataTypeNotFound when the URI has no registration.
	GetDataTypeWithExtensions(ctx context.Context, in GetDataTypeWithExtensionsInput) (GetDataTypeOutput, error)
}

// === Data-type store inputs/outputs (the data-type kind: modules/contributions/contribution-types.md §7;
// the schema service as a projection of the catalog: modules/contributions/projections.md)

// RegisterDataTypeInput carries one data-type Body keyed by its URI.
type RegisterDataTypeInput struct {
	URI  string `json:"uri"  desc:"Canonical data-type URI: <provider>:/data-type/<name>" required:"true"`
	Body []byte `json:"body" desc:"DSL Schema body (YAML or canonical JSON)"               required:"true"`
}

// GetDataTypeInput fetches a registered data-type by URI.
type GetDataTypeInput struct {
	URI string `json:"uri" cli:"arg" desc:"Canonical data-type URI to fetch" required:"true"`
}

// GetDataTypeOutput returns the canonical AST.
type GetDataTypeOutput struct {
	Schema *schemaapi.Schema `json:"schema"`
}

// GetJSONSchemaInput fetches the compiled JSON Schema for a registered data-type.
type GetJSONSchemaInput struct {
	URI string `json:"uri" cli:"arg" desc:"Canonical data-type URI to compile to JSON Schema 2020-12" required:"true"`
}

// GetJSONSchemaOutput returns the compiled JSON Schema artifact.
type GetJSONSchemaOutput struct {
	Schema json.RawMessage `json:"schema"`
}

// ValidateAgainstInput validates a JSON body against a registered data-type.
type ValidateAgainstInput struct {
	URI  string          `json:"uri"  desc:"Canonical data-type URI to validate against" required:"true"`
	Body json.RawMessage `json:"body" desc:"JSON body to validate"                       required:"true"`
}

// ExtensionRef identifies an extension (type, name) pair to keep on a
// schema's types. Either field can be `"*"` to match any value.
type ExtensionRef struct {
	Type string `json:"type" desc:"Extension type (e.g. \"ui-form\"); \"*\" matches any" required:"true"`
	Name string `json:"name" desc:"Extension name (e.g. \"default\");   \"*\" matches any" required:"true"`
}

// GetDataTypeWithExtensionsInput fetches a registered data-type and
// returns its AST with the type-level extensions filtered down to the
// listed (type, name) pairs.
type GetDataTypeWithExtensionsInput struct {
	URI        string         `json:"uri"                   cli:"arg" desc:"Canonical data-type URI" required:"true"`
	Extensions []ExtensionRef `json:"extensions,omitempty"           desc:"Extension filter (empty = drop all extensions; use [{type:*,name:*}] for everything)"`
}

// ErrDataTypeNotFound is returned by GetDataType / GetJSONSchema /
// ValidateAgainst when the URI has no registered data-type.
var ErrDataTypeNotFound = errDataTypeNotFound{}

type errDataTypeNotFound struct{}

func (errDataTypeNotFound) Error() string { return "schema: data-type not found" }

// SchemaServiceDef describes the SchemaService for auto-publication
// (KTSP / HTTP / Cobra CLI generation).
//
// NOTE: RegisterDataType is intentionally absent from the Methods map.
// It is the hub-internal ingest mechanism called by ktw-hub's bootstrap
// factory; external callers must not be able to mutate the data-type
// store over the bus.
var SchemaServiceDef = meta.ServiceDef{
	Name:    "schema",
	Short:   "Data-DSL schema parser, normalizer, and universe-wide data-type store",
	Long:    `Two roles: (1) stateless parser+normalizer that converts YAML/JSON DSL schemas into canonical AST; (2) stateful data-type store that holds parsed schemas keyed by URI, compiles them to JSON Schema 2020-12, and validates bodies. The AST is the cross-language contract every downstream generator (JSON Schema, Go structs, TS types, Pydantic) consumes. Hub-side ktw-hub publishes this service so the universe shares ONE schema authority. Rendering is out of scope — frontends consume the AST + ui-* extensions and render in their own context (see modules/display/tui for the terminal renderer). Authoritative spec: modules/framework/schema/concepts.md.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Parse": meta.MethodLong(
			"Parse a YAML or JSON DSL schema from bytes",
			`Reads a YAML or JSON schema string and returns the canonical AST. Auto-detects format unless Format is set. Imports are resolved relative to BasePath. Validation runs implicitly; use Validate for re-validation after manual AST edits.`,
		),
		"ParseFile": meta.MethodLong(
			"Parse a YAML or JSON DSL schema from a file",
			`Reads a schema file from disk. The file's parent directory becomes the import base path. Format is inferred from the file extension (.yaml, .yml, .json) unless Format is set explicitly.`,
		),
		"Validate": meta.MethodLong(
			"Validate an existing AST",
			`Walks an existing AST and reports validation errors (orphan refs, missing required fields, type-content incompatibilities, tree invariant violations). Always runs as part of Parse; this method is for re-validation after manual AST edits.`,
		),
		"GetDataType": meta.MethodLong(
			"Fetch a registered data-type by URI",
			`Returns the canonical Schema AST of a data-type previously registered (via RegisterDataType at hub bootstrap, or via runtime data-type Contributions). Returns ErrDataTypeNotFound when the URI has no registration.`,
		),
		"GetJSONSchema": meta.MethodLong(
			"Fetch the compiled JSON Schema 2020-12 for a registered data-type",
			`Returns the JSON Schema 2020-12 artifact compiled from a registered data-type's AST. Compiled on first request and cached. Polyglot consumers without DSL parser access use this to validate locally.`,
		),
		"ValidateAgainst": meta.MethodLong(
			"Validate a JSON body against a registered data-type",
			`Validates a JSON body against a registered data-type by URI. Returns the same Diagnostic shape as Validate. Used by the catalog at Phase 2 ingest and by consumers that want defensive decode.`,
		),
		"GetDataTypeWithExtensions": meta.MethodLong(
			"Fetch a registered data-type with its extensions filtered to a (type,name) allowlist",
			`Returns the canonical Schema AST of a registered data-type with its type-level extensions narrowed to the listed (type, name) pairs. Use this from frontends that already know which ui-* variants they will render — one round trip, no client-side extension walk. Type or Name can be "*" as a wildcard; an empty list returns the schema with no extensions. Returns ErrDataTypeNotFound when the URI has no registration.`,
		),
	},
}
