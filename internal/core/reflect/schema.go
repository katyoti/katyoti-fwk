// SPDX-License-Identifier: Apache-2.0

// One-line convenience helpers over the canonical TypeAST → JSON Schema
// path. The legacy implementation (which used `google/jsonschema-go`
// directly) was removed 2026-05-22 per
// these helpers replace it, going through the new TypeAST intermediate.
//
// Use:
//
//	js := reflect.MustJSONSchema[MyType]()                       // init-time
//	js, err := reflect.JSONSchema[MyType]()                      // runtime
//	js, err := reflect.JSONSchemaForType(reflect.TypeOf(t))      // runtime, type-only
//
// If you need the Type AST itself (display extensions, edge annotations,
// validation against another schema), call `reflect.TypeAST[T]()` directly
// — these one-liners discard the intermediate.

package reflect

import (
	"encoding/json"
	stdreflect "reflect"
)

// JSONSchema returns the JSON Schema 2020-12 bytes for T, via the
// canonical Type AST. Equivalent to TypeAST[T]() + ToJSONSchema() but
// one call.
func JSONSchema[T any]() (json.RawMessage, error) {
	ast, err := TypeAST[T]()
	if err != nil {
		return nil, err
	}
	return ast.ToJSONSchema()
}

// MustJSONSchema is the panic-on-error variant of JSONSchema. Use at
// init time for static definitions.
func MustJSONSchema[T any]() json.RawMessage {
	return MustTypeAST[T]().MustToJSONSchema()
}

// JSONSchemaForType is the runtime variant. Use when the type is only
// known as a reflect.Type (e.g. inside KTSP slot-method dispatchers).
func JSONSchemaForType(t stdreflect.Type) (json.RawMessage, error) {
	ast, err := TypeASTForType(t)
	if err != nil {
		return nil, err
	}
	return ast.ToJSONSchema()
}
