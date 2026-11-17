// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// GenerateSchema generates a JSON Schema from a Go struct type. Goes via
// the canonical Type AST so display extensions (ui-form / ui-view /
// ui-table / ui-cli, dsl edge annotations) are preserved on the AST
// side even though they don't appear in the JSON Schema output.
func GenerateSchema[T any]() (json.RawMessage, error) {
	return corereflect.JSONSchema[T]()
}

// MustGenerateSchema generates a JSON Schema or panics. Init-time use.
func MustGenerateSchema[T any]() json.RawMessage {
	return corereflect.MustJSONSchema[T]()
}
