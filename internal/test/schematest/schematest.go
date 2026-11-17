// SPDX-License-Identifier: Apache-2.0

// Package schematest compiles a test's schema fixture, written in the DSL, to
// the JSON Schema a validator takes. Tests only: production code resolves a
// data-type through the SchemaService and never parses for itself
// (tools/import-discipline, schema-parser-boundary, which exempts
// internal/test/ as the home of test-only readers).
package schematest

import (
	"context"
	"encoding/json"

	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// CompileJSONSchema compiles a DSL fixture to JSON Schema 2020-12.
func CompileJSONSchema(ctx context.Context, dsl []byte) (json.RawMessage, error) {
	return schemaparser.CompileJSONSchema(ctx, dsl)
}
