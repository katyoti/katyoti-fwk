// SPDX-License-Identifier: Apache-2.0

// Package reflect provides reflection-based service method discovery.
// Used by protocol adapters (KTSP, HTTP, MCP) to find publishable methods
// on Go service interfaces.
package reflect

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode"
)

var timeType = reflect.TypeOf(time.Time{})

var rawMessageType = reflect.TypeOf(json.RawMessage(nil))

// ServiceMethod describes a publishable method discovered via reflection.
type ServiceMethod struct {
	Name       string         // method name (e.g., "Render")
	InputType  reflect.Type   // the Input struct type (nil for no-input methods)
	OutputType reflect.Type   // the Output struct type
	Method     reflect.Method // the reflect.Method handle
}

// DiscoverMethods finds all publishable methods on a service type.
// Works with both interface types and concrete (pointer) types.
// A publishable method has signature: func(ctx context.Context, in T) (U, error)
// or func(ctx context.Context) (U, error) for no-input methods.
func DiscoverMethods(serviceType reflect.Type) []ServiceMethod {
	isInterface := serviceType.Kind() == reflect.Interface
	var methods []ServiceMethod
	for i := 0; i < serviceType.NumMethod(); i++ {
		m := serviceType.Method(i)
		if sm, ok := classifyMethod(m, isInterface); ok {
			methods = append(methods, sm)
		}
	}
	return methods
}

// classifyMethod checks if a method matches a publishable signature:
//
//	func(ctx context.Context, in T) (U, error)   — with input
//	func(ctx context.Context) (U, error)          — no input (e.g., List)
//
// isInterface: true for interface types (no receiver param), false for concrete types.
func classifyMethod(m reflect.Method, isInterface bool) (ServiceMethod, bool) {
	mt := m.Type

	if mt.NumOut() != 2 {
		return ServiceMethod{}, false
	}
	if !mt.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return ServiceMethod{}, false
	}

	// For concrete types, skip the receiver (param 0)
	offset := 0
	if !isInterface {
		offset = 1
	}
	paramCount := mt.NumIn() - offset

	outType := mt.Out(0)
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()

	switch paramCount {
	case 1:
		// func(ctx) (U, error) — no input struct
		if !mt.In(offset).Implements(ctxType) {
			return ServiceMethod{}, false
		}
		return ServiceMethod{
			Name: m.Name, InputType: nil, OutputType: outType, Method: m,
		}, true

	case 2:
		// func(ctx, T) (U, error) — with input struct
		if !mt.In(offset).Implements(ctxType) {
			return ServiceMethod{}, false
		}
		inType := mt.In(offset + 1)
		if inType.Kind() == reflect.Ptr {
			inType = inType.Elem()
		}
		if inType.Kind() != reflect.Struct {
			return ServiceMethod{}, false
		}
		return ServiceMethod{
			Name: m.Name, InputType: mt.In(offset + 1), OutputType: outType, Method: m,
		}, true

	default:
		return ServiceMethod{}, false
	}
}

// GoTypeToFlagType maps Go reflect.Type to CLI flag type string.
//
// Returned values:
//
//	"boolean"     — bool
//	"int"         — any int kind
//	"float"       — any float kind
//	"stringSlice" — []string (a JSON array, or comma-separated CLI input)
//	"stringMap"   — map[string]string (parsed from "k=v,k=v" CLI input)
//	"jsonObject"  — map[string]any / map[string]<non-string> (parsed via json.Unmarshal)
//	"jsonValue"   — a struct, a pointer to one, or a slice of anything but
//	                strings and bytes (raw JSON on the command line); and
//	                json.RawMessage: the JSON as written lands as is (an
//	                action's input), never quoted into a string
//	"bytes"       — []byte: the text as written, `@path` for a file's bytes,
//	                `base64:<data>` for binary
//	"string"      — everything else (time.Time included: it parses from text)
func GoTypeToFlagType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Pointer:
		if t.Elem().Kind() == reflect.Struct {
			return GoTypeToFlagType(t.Elem())
		}
		return "string"
	case reflect.Struct:
		if t == timeType {
			return "string"
		}
		return "jsonValue"
	case reflect.Slice:
		switch t.Elem().Kind() {
		case reflect.String:
			return "stringSlice"
		case reflect.Uint8:
			if t == rawMessageType {
				return "jsonValue"
			}
			return "bytes"
		}
		return "jsonValue"
	case reflect.Map:
		if t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.String {
			return "stringMap"
		}
		// map[string]any / map[string]interface{} / map with non-
		// string value: the CLI receives JSON text and the
		// dispatcher must json.Unmarshal it into the field type.
		// See cli/generate.go's "jsonObject" branch in buildParams.
		if t.Key().Kind() == reflect.String {
			return "jsonObject"
		}
		return "string"
	default:
		return "string"
	}
}

// ToKebab converts PascalCase to kebab-case, treating consecutive
// uppercase letters as a single acronym word.
//
//	"RunReport"          → "run-report"
//	"GetWidgetURL"       → "get-widget-url"   (URL = one acronym word)
//	"GenerateJSONSchema" → "generate-json-schema"
//	"GetTSXFile"         → "get-tsx-file"
//	"parseHTTPRequest"   → "parse-http-request"
//
// Rule: insert a hyphen at position i (i > 0) when:
//   - the previous rune is lowercase or a digit (boundary: a new word
//     begins after a non-uppercase character), OR
//   - the previous rune is uppercase AND the next rune exists and is
//     lowercase (boundary: end of an acronym → start of a new word).
//
// Used by KTSP/HTTP/MCP method names, auto-CLI command names, and
// agent-tool names so a single change here keeps all surfaces aligned.
func ToKebab(s string) string {
	runes := []rune(s)
	var result []rune
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			insertHyphen := false
			if !unicode.IsUpper(prev) {
				insertHyphen = true
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				insertHyphen = true
			}
			if insertHyphen {
				result = append(result, '-')
			}
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}

// FieldTagName extracts the primary name from a struct tag (before any comma).
// Returns "" if tag is empty or "-".
func FieldTagName(tag, key string) string {
	v := tag
	if v == "" || v == "-" {
		return ""
	}
	return strings.SplitN(v, ",", 2)[0]
}
