// SPDX-License-Identifier: Apache-2.0

// Package api defines the canonical AST and public types for the schema extension.
//
// The AST is the cross-language contract for the data-DSL framework: every
// downstream generator (JSON Schema, Go structs, TS types, Pydantic) consumes
// the canonical Schema. Two schemas — one written in YAML, one in JSON — that
// parse to byte-identical canonical AST are equivalent.
//
// Authoritative spec: modules/framework/schema/codegen/concepts.md and
// modules/framework/schema/dsl.md.
package schemaapi

import "encoding/json"

// Schema is the parsed and normalized result of a DSL file. It carries the
// dictionary of named user types plus optional metadata.
type Schema struct {
	DSLVersion  string           `json:"$schema,omitempty"`
	Name        string           `json:"name"`
	Version     string           `json:"version,omitempty"`
	Description string           `json:"description,omitempty"`
	Imports     []string         `json:"imports,omitempty"`
	Root        string           `json:"root,omitempty"`
	Types       map[string]*Type `json:"types"`
}

// TypeKind discriminates the variant of a Type node. Values match the DSL
// surface keyword (lower-case).
type TypeKind string

const (
	KindString  TypeKind = "string"
	KindNumber  TypeKind = "number"
	KindBoolean TypeKind = "boolean"
	KindObject  TypeKind = "object"
	KindArray   TypeKind = "array"
	KindChoice  TypeKind = "choice"
	KindLink    TypeKind = "link"
	KindTree    TypeKind = "tree"
	KindAny     TypeKind = "any"
	KindNull    TypeKind = "null"
)

// LinkMode is the cardinality of a link or tree relationship.
type LinkMode string

const (
	LinkOne2One   LinkMode = "one2one"
	LinkOne2Many  LinkMode = "one2many"
	LinkMany2One  LinkMode = "many2one"
	LinkMany2Many LinkMode = "many2many"
)

// Type is the canonical AST node. Fields not applicable to the active Kind are
// zero-valued and omitted from JSON output.
//
// The discriminator is Kind. The fields fall into bands:
//   - meta (label/placeholder/description/visibility/...): valid on every kind
//   - primitive (Content, ContentParams, Default): string/number/boolean
//   - object (Fields, IDField, TitleField, Catchall, Extends)
//   - array (Items, Min, Max)
//   - choice (Cases)
//   - link (Target, LinkMode, LinkType, ...): link and tree
//   - tree extras (RootValue, MaxDepth, AllowCircular, ShowPath, DragDrop)
//
// Extensions carries any UI extensions (ui-form / ui-table / ui-filter / ...)
// attached to this type. They are intentionally untyped at this layer; the
// renderer or generator interprets them.
type Type struct {
	Kind TypeKind `json:"type"`

	// --- shared metadata ---
	Label        string         `json:"label,omitempty"`
	Placeholder  string         `json:"placeholder,omitempty"`
	Description  string         `json:"description,omitempty"`
	Informations string         `json:"informations,omitempty"`
	Optional     bool           `json:"optional,omitempty"`
	Default      any            `json:"default,omitempty"`
	Visibility   []string       `json:"visibility,omitempty"`
	Detail       int            `json:"detail,omitempty"`
	Alias        []string       `json:"alias,omitempty"`
	Extensions   []Extension    `json:"extensions,omitempty"`
	Custom       map[string]any `json:"-"` // non-canonical pass-through (renderer hints)

	// --- primitives (string / number / boolean) ---
	Content       string         `json:"content,omitempty"`       // content-type name (e.g. "email", "markdown")
	ContentParams map[string]any `json:"contentParams,omitempty"` // inline params from shorthand or full block
	Enum          []any          `json:"enum,omitempty"`          // restricted value set for string/number (DSL: @enum(a,b,c))

	// --- object ---
	Fields     map[string]*Type `json:"fields,omitempty"`
	IDField    []string         `json:"idField,omitempty"`    // 1+ field names
	TitleField []string         `json:"titleField,omitempty"` // 1+ field names (composite)
	Catchall   *TypeRef         `json:"catchall,omitempty"`
	Extends    []string         `json:"extends,omitempty"`

	// --- array ---
	Items *TypeRef `json:"items,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`

	// --- choice ---
	Cases []*TypeRef `json:"cases,omitempty"`

	// --- link / tree ---
	Target      string   `json:"target,omitempty"`
	LinkMode    LinkMode `json:"mode,omitempty"`
	LinkType    string   `json:"linkType,omitempty"`    // "associated" (link default) or "part_of" (tree)
	TargetID    string   `json:"targetId,omitempty"`    // target's id field
	Reverse     string   `json:"reverse,omitempty"`     // reverse field on target
	Display     string   `json:"display,omitempty"`     // display field
	AllowCreate *bool    `json:"allowCreate,omitempty"` // pointer so default-true survives round-trip
	Searchable  *bool    `json:"searchable,omitempty"`

	// --- tree extras ---
	RootValue     any   `json:"rootValue,omitempty"`
	MaxDepth      int   `json:"maxDepth,omitempty"`
	AllowCircular bool  `json:"allowCircular,omitempty"`
	ShowPath      *bool `json:"showPath,omitempty"`
	DragDrop      *bool `json:"dragDrop,omitempty"`
}

// TypeRef is either a name reference to a top-level user type, or an inline
// Type definition. Exactly one of Name and Inline is non-zero.
//
// Canonical serialization: when Name is set, marshals as a bare string ("User");
// when Inline is set, marshals as the full Type object. This matches the DSL
// shorthand convention where field values can be either name references or
// inline types.
//
// The custom MarshalJSON / UnmarshalJSON below MUST stay in sync with the
// canonical encoder (`canonical.go::canonicalizeRef`) so the two paths
// produce identical output. Standard `json.Marshal` is what Wails /
// KTSP / NATS transports use under the hood — without this method,
// schemas with `items` / `catchall` / `cases` fields surface as
// PascalCase `{"Name":"","Inline":{...}}` on the wire and break every
// consumer (including ktw-display-lib's `SchemaQuery.getType`, which
// throws "Bad type" because the shape has no `type` field).
type TypeRef struct {
	Name   string
	Inline *Type
}

// MarshalJSON emits the canonical shape: a bare JSON string for a name
// reference, or the inline Type's own JSON for an inline definition.
// Nil refs serialise to `null`.
func (r *TypeRef) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	if r.Name != "" {
		return json.Marshal(r.Name)
	}
	if r.Inline != nil {
		return json.Marshal(r.Inline)
	}
	return []byte("null"), nil
}

// UnmarshalJSON accepts either a bare string (name reference) or a JSON
// object (inline Type definition). `null` is treated as the zero ref.
func (r *TypeRef) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*r = TypeRef{}
		return nil
	}
	// Strings → name reference.
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*r = TypeRef{Name: s}
		return nil
	}
	// Anything else → inline Type.
	t := &Type{}
	if err := json.Unmarshal(b, t); err != nil {
		return err
	}
	*r = TypeRef{Inline: t}
	return nil
}

// IsName reports whether this ref is a name (vs inline).
func (r *TypeRef) IsName() bool { return r != nil && r.Name != "" }

// IsInline reports whether this ref carries an inline type.
func (r *TypeRef) IsInline() bool { return r != nil && r.Inline != nil }

// Extension carries a UI extension attached to a type (ui-form, ui-table,
// ui-filter, or arbitrary custom extension). The Type field is the
// extension kind ("ui-form", etc.); the rest is opaque payload.
type Extension struct {
	Kind    string         `json:"type"`
	Payload map[string]any `json:"-"` // remaining keys (everything except "type")
}

// NewSchema constructs a minimal valid Schema with the given name.
func NewSchema(name string) *Schema {
	return &Schema{
		DSLVersion: "ktw-schema/v1",
		Name:       name,
		Types:      make(map[string]*Type),
	}
}

// NewType constructs a Type with the given kind. Helper for programmatic
// schema construction.
func NewType(k TypeKind) *Type { return &Type{Kind: k} }

// RefName builds a name reference TypeRef.
func RefName(name string) *TypeRef { return &TypeRef{Name: name} }

// RefInline builds an inline TypeRef.
func RefInline(t *Type) *TypeRef { return &TypeRef{Inline: t} }
