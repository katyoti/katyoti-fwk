// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"encoding/json"
	"fmt"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// CompileJSONSchema turns an authored DSL document into JSON Schema.
//
// # Why this exists as one function
//
// The DSL is the language everything is WRITTEN and STORED in. JSON Schema is
// what a few consumers SPEAK — an LLM given a tool definition, a form renderer
// that predates the AST. It is a projection, never a second source: derived at
// the edge that needs it and thrown away, exactly like `reflect.JSONSchema[T]`
// derives one from a Go type.
//
// One function rather than a call at each edge, because a derivation with three
// writers drifts: one of them ends up sniffing which language it was handed,
// and a sniffer is what a split language looks like from the inside.
//
// Cheap to reach for: this package depends on `schema/api` and nothing else, so
// compiling here costs a consumer two packages — not the ~56 of the schema
// SERVICE, which is service wiring, not the language.
func CompileJSONSchema(ctx context.Context, body []byte) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, nil
	}
	out, err := Parse(ctx, schemaapi.ParseInput{Body: body})
	if err != nil {
		return nil, fmt.Errorf("parse DSL: %w", err)
	}
	if out.Schema == nil {
		return nil, fmt.Errorf("parse DSL: no schema")
	}
	return out.Schema.ToJSONSchema()
}

// CompileJSONSchemaWithOptions is CompileJSONSchema for a form being served:
// fields declaring an `options_from` get their choices from sources first, so
// the JSON Schema carries them as `enum`. A field whose source fails stays
// free text; the failures come back beside the schema.
func CompileJSONSchemaWithOptions(ctx context.Context, body []byte, sources map[string]schemaapi.OptionSource) (json.RawMessage, []error, error) {
	if len(body) == 0 {
		return nil, nil, nil
	}
	out, err := Parse(ctx, schemaapi.ParseInput{Body: body})
	if err != nil {
		return nil, nil, fmt.Errorf("parse DSL: %w", err)
	}
	if out.Schema == nil {
		return nil, nil, fmt.Errorf("parse DSL: no schema")
	}
	warnings := schemaapi.ResolveOptions(ctx, out.Schema, sources)
	compiled, err := out.Schema.ToJSONSchema()
	return compiled, warnings, err
}

// JSONSchemaOrNil is CompileJSONSchema for the callers that have no way to
// report a failure — an LLM tool definition being assembled, a descriptor being
// filled in.
//
// Nil on failure rather than a partial document: a consumer that receives
// nothing falls back to its generic schema, while one that receives half a
// contract sends the model a shape it cannot satisfy.
func JSONSchemaOrNil(ctx context.Context, body []byte) json.RawMessage {
	compiled, err := CompileJSONSchema(ctx, body)
	if err != nil {
		return nil
	}
	return compiled
}
