// SPDX-License-Identifier: Apache-2.0

// Package reflect provides Go-struct → schema.Type AST conversion via
// reflection. The Type AST is the canonical intermediate (single source
// of truth) for runtime schemas across the universe; JSON Schema 2020-12
// is a derived artifact via Type.ToJSONSchema().
//
// Tag conventions honored (matches the contribution envelope
// — modules/contributions/concepts.md §2):
//
//	json:"name,omitempty"                          // field name + optionality
//	jsonschema:"description=...,required"          // descriptions + required
//	ui-form:"widget=...,label=..."                 // ktw-display auto-form hints
//	ui-view:"role=primary,..."                     // read-only display hints
//	ui-table:"columns=..."                         // table output hints
//	ui-cli:"mode=table|text|json,..."              // CLI rich-response hints
//	dsl:"link=many2many,relation=requires,target=Contribution"  // edge-bearing field
package reflect

import (
	"encoding/json"
	"fmt"
	stdreflect "reflect"
	"strings"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// TypeAST produces a canonical schema.Type AST from a Go type parameter.
// Replaces JSONSchema[T] as the source of truth for runtime schemas.
// Callers who need JSON Schema 2020-12 should chain with .ToJSONSchema().
func TypeAST[T any]() (*schemaapi.Type, error) {
	return TypeASTForType(stdreflect.TypeFor[T]())
}

// MustTypeAST is the panic-on-error variant of TypeAST, for init-time
// definitions where errors are not recoverable.
func MustTypeAST[T any]() *schemaapi.Type {
	t, err := TypeAST[T]()
	if err != nil {
		panic(err)
	}
	return t
}

// TypeASTForType produces a Type AST from a reflect.Type. Use when the
// type is only known at runtime.
func TypeASTForType(t stdreflect.Type) (*schemaapi.Type, error) {
	return walk(t, walkSeen{})
}

// OpenSchema is implemented by a type whose JSON form its Go fields do
// not describe — typically a union its own UnmarshalJSON reads. TypeAST
// renders it as any; the type's decoder is its check.
type OpenSchema interface {
	OpenSchema()
}

var openSchemaType = stdreflect.TypeFor[OpenSchema]()

// walkSeen tracks types currently on the recursion stack to break
// cycles. A re-entry on the same type emits a placeholder Any node.
type walkSeen map[stdreflect.Type]bool

func walk(t stdreflect.Type, seen walkSeen) (*schemaapi.Type, error) {
	// Unwrap pointers
	for t.Kind() == stdreflect.Ptr {
		t = t.Elem()
	}

	// json.RawMessage is an opaque, arbitrary-JSON payload — NOT a byte string.
	// It is []byte under the hood, so without this guard the slice branch below
	// would emit {"type":"string","format":"byte"} (base64), telling LLMs and
	// other clients to send a STRINGIFIED body. They then hand back quoted JSON
	// that downstream decoders silently drop. Treat it as KindAny so the schema
	// is open and callers send a real object/value. (Must be BEFORE the slice
	// branch — checking inside walkStruct never fires, RawMessage is slice-kind.)
	if t == stdreflect.TypeOf(json.RawMessage{}) {
		return &schemaapi.Type{Kind: schemaapi.KindAny}, nil
	}

	// A type that decodes its own JSON (a union read by its UnmarshalJSON)
	// and says so: its fields do not describe its wire form, so the schema
	// is open.
	if t.Implements(openSchemaType) || stdreflect.PointerTo(t).Implements(openSchemaType) {
		return &schemaapi.Type{Kind: schemaapi.KindAny}, nil
	}

	// time.Time is text on the wire (RFC 3339), not the struct it is in Go.
	if t == timeType {
		return &schemaapi.Type{Kind: schemaapi.KindString}, nil
	}

	if seen[t] {
		return &schemaapi.Type{Kind: schemaapi.KindAny, Description: fmt.Sprintf("cycle: %s", t.Name())}, nil
	}

	switch t.Kind() {
	case stdreflect.String:
		return &schemaapi.Type{Kind: schemaapi.KindString}, nil
	case stdreflect.Int, stdreflect.Int8, stdreflect.Int16, stdreflect.Int32, stdreflect.Int64,
		stdreflect.Uint, stdreflect.Uint8, stdreflect.Uint16, stdreflect.Uint32, stdreflect.Uint64,
		stdreflect.Float32, stdreflect.Float64:
		return &schemaapi.Type{Kind: schemaapi.KindNumber}, nil
	case stdreflect.Bool:
		return &schemaapi.Type{Kind: schemaapi.KindBoolean}, nil
	case stdreflect.Slice, stdreflect.Array:
		// []byte → string (matches encoding/json behaviour for base64).
		if t.Elem().Kind() == stdreflect.Uint8 {
			return &schemaapi.Type{Kind: schemaapi.KindString, Content: "byte"}, nil
		}
		elemAST, err := walk(t.Elem(), seen)
		if err != nil {
			return nil, fmt.Errorf("array element: %w", err)
		}
		return &schemaapi.Type{
			Kind:  schemaapi.KindArray,
			Items: schemaapi.RefInline(elemAST),
		}, nil
	case stdreflect.Map:
		// Maps emit object with Catchall.
		valAST, err := walk(t.Elem(), seen)
		if err != nil {
			return nil, fmt.Errorf("map value: %w", err)
		}
		return &schemaapi.Type{
			Kind:     schemaapi.KindObject,
			Catchall: schemaapi.RefInline(valAST),
		}, nil
	case stdreflect.Struct:
		return walkStruct(t, seen)
	case stdreflect.Interface:
		// Any interface → KindAny.
		return &schemaapi.Type{Kind: schemaapi.KindAny}, nil
	default:
		return nil, fmt.Errorf("unsupported reflect kind: %s", t.Kind())
	}
}

func walkStruct(t stdreflect.Type, seen walkSeen) (*schemaapi.Type, error) {
	seen[t] = true
	defer delete(seen, t)

	obj := &schemaapi.Type{
		Kind:   schemaapi.KindObject,
		Fields: make(map[string]*schemaapi.Type),
	}

	// Walk struct fields. Honor json tags for naming + omitempty.
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		// Embedded structs: hoist fields into the parent.
		if sf.Anonymous && sf.Type.Kind() == stdreflect.Struct {
			embed, err := walkStruct(sf.Type, seen)
			if err != nil {
				return nil, fmt.Errorf("embed %q: %w", sf.Name, err)
			}
			for k, v := range embed.Fields {
				obj.Fields[k] = v
			}
			continue
		}

		name, optional, skip := parseJSONTag(sf)
		if skip {
			continue
		}

		fieldAST, err := walk(sf.Type, seen)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", sf.Name, err)
		}
		fieldAST.Optional = optional

		// jsonschema:"description=..."
		if jsTag := sf.Tag.Get("jsonschema"); jsTag != "" {
			parseJSONSchemaTag(jsTag, fieldAST)
		}

		// ui-form / ui-view / ui-table / ui-cli — extension carriers
		for _, extName := range []string{"ui-form", "ui-view", "ui-table", "ui-cli"} {
			if tag := sf.Tag.Get(extName); tag != "" {
				fieldAST.Extensions = append(fieldAST.Extensions, parseUITag(extName, tag))
			}
		}

		// dsl:"link=many2many,relation=requires,target=Contribution"
		if dslTag := sf.Tag.Get("dsl"); dslTag != "" {
			parseDSLTag(dslTag, fieldAST)
		}

		obj.Fields[name] = fieldAST
	}

	return obj, nil
}

// parseJSONTag extracts the wire name + omitempty flag + skip rule
// from a struct field's `json` tag.
func parseJSONTag(sf stdreflect.StructField) (name string, optional, skip bool) {
	tag := sf.Tag.Get("json")
	if tag == "" {
		// Fall back to `yaml`. A schema names a field the way the DECODER
		// names it, and a config struct is decoded from YAML — so a struct
		// tagged only for YAML would otherwise be described under its Go field
		// names, publishing a shape that does not match the file it describes.
		tag = sf.Tag.Get("yaml")
	}
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return sf.Name, false, false
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = sf.Name
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			optional = true
		}
	}
	return name, optional, false
}

// parseJSONSchemaTag understands a small subset of the google/jsonschema-go
// tag grammar: `description=...`, `required`, `default=...`, `title=...`,
// `enum=a|b|c` (the restricted value set of a string field) and
// `content=markdown` (the field's content type, as the DSL's `string@markdown`).
// Comma-separated key[=value] pairs.
func parseJSONSchemaTag(tag string, dst *schemaapi.Type) {
	for _, pair := range splitTopLevelCommas(tag) {
		k, v, _ := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "description":
			dst.Description = v
		case "required":
			// required is the inverse of optional. If the json tag set
			// optional=true via omitempty, jsonschema:"required" overrides.
			dst.Optional = false
		case "default":
			dst.Default = v
		case "title":
			dst.Label = v
		case "content":
			dst.Content = v
		case "enum":
			dst.Enum = nil
			for _, e := range strings.Split(v, "|") {
				if e = strings.TrimSpace(e); e != "" {
					dst.Enum = append(dst.Enum, e)
				}
			}
		}
	}
}

// parseUITag wraps a ui-* tag into an Extension. The tag's content is
// stored verbatim in Extension.Payload under key "raw"; renderers parse
// it on demand. (Future evolution: structured payload per extension
// kind — see 3L `ui-cli-schema-extension`.)
func parseUITag(extName, tag string) schemaapi.Extension {
	payload := map[string]any{"raw": tag}
	for _, pair := range splitTopLevelCommas(tag) {
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok {
			payload[k] = true
		} else {
			payload[k] = v
		}
	}
	return schemaapi.Extension{Kind: extName, Payload: payload}
}

// parseDSLTag understands the edge-bearing-field annotation.
//
//	dsl:"link=many2many,relation=requires,target=Contribution"
//
// Populates LinkMode / Target on the destination AST node.
func parseDSLTag(tag string, dst *schemaapi.Type) {
	for _, pair := range splitTopLevelCommas(tag) {
		k, v, _ := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "link":
			switch schemaapi.LinkMode(v) {
			case schemaapi.LinkOne2One, schemaapi.LinkOne2Many,
				schemaapi.LinkMany2One, schemaapi.LinkMany2Many:
				dst.LinkMode = schemaapi.LinkMode(v)
			}
		case "target":
			dst.Target = v
		case "relation":
			// Stored in Extensions for the edge extractor to read.
			dst.Extensions = append(dst.Extensions, schemaapi.Extension{
				Kind:    "dsl-relation",
				Payload: map[string]any{"relation": v},
			})
		}
	}
}

// splitTopLevelCommas splits a tag by top-level commas, respecting
// parens and brackets so nested values aren't broken.
func splitTopLevelCommas(s string) []string {
	out := []string{}
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}
