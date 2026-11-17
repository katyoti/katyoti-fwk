// SPDX-License-Identifier: Apache-2.0

package api

// Schema is the canonical AST returned from Parse. Same wire shape as
// app/ktw-display/src/schema/schema-types.ts (DSL-aligned 2026-05-01).
type Schema struct {
	DSLVersion  string           `json:"$schema,omitempty"`
	Name        string           `json:"name"`
	Version     int              `json:"version,omitempty"`
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

// Type is the canonical AST node. Field names follow the DSL grammar
// (snake_case for link/tree per the M5 alignment 2026-05-01). Content is the
// nested envelope; ContentRef carries either a string ref or an inline object.
type Type struct {
	Kind TypeKind `json:"type"`

	// shared metadata
	Label        string      `json:"label,omitempty"`
	Placeholder  string      `json:"placeholder,omitempty"`
	Description  string      `json:"description,omitempty"`
	Informations string      `json:"informations,omitempty"`
	Optional     bool        `json:"optional,omitempty"`
	Default      any         `json:"default,omitempty"`
	Visibility   []string    `json:"visibility,omitempty"`
	Detail       int         `json:"detail,omitempty"`
	Alias        []string    `json:"alias,omitempty"`
	Extensions   []Extension `json:"extensions,omitempty"`

	// primitives — nested content envelope (open shape; content body is a
	// {type, ...params} bag where `type` is the content-type name).
	Content *ContentRef `json:"content,omitempty"`

	// object
	Fields     map[string]*Type `json:"fields,omitempty"`
	IDField    []string         `json:"idField,omitempty"`
	TitleField []string         `json:"titleField,omitempty"`
	Catchall   string           `json:"catchall,omitempty"`
	Extends    []string         `json:"extends,omitempty"`

	// array
	Items *TypeRef `json:"items,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`

	// choice
	Cases []*TypeRef `json:"cases,omitempty"`

	// link / tree (snake_case per M5)
	Target      string   `json:"target,omitempty"`
	Mode        LinkMode `json:"mode,omitempty"`
	LinkType    string   `json:"link_type,omitempty"`
	TargetID    string   `json:"target_id,omitempty"`
	Reverse     string   `json:"reverse,omitempty"`
	Display     string   `json:"display,omitempty"`
	AllowCreate *bool    `json:"allow_create,omitempty"`
	Searchable  *bool    `json:"searchable,omitempty"`

	// tree extras
	RootValue     any   `json:"root_value,omitempty"`
	MaxDepth      int   `json:"max_depth,omitempty"`
	AllowCircular bool  `json:"allow_circular,omitempty"`
	ShowPath      *bool `json:"show_path,omitempty"`
	DragDrop      *bool `json:"drag_drop,omitempty"`
}

// TypeRef is either a name reference to a top-level user type, or an inline
// Type definition. Wire shape: bare string for name refs; full Type object
// for inline. Custom UnmarshalJSON handles both.
type TypeRef struct {
	Name   string
	Inline *Type
}

// IsName reports whether this ref is a name (vs inline).
func (r *TypeRef) IsName() bool { return r != nil && r.Name != "" }

// IsInline reports whether this ref carries an inline type.
func (r *TypeRef) IsInline() bool { return r != nil && r.Inline != nil }

// ContentRef carries either a string ref to a content type (e.g. "markdown"
// or "label(max:10)" shorthand) or an inline object with type+params.
type ContentRef struct {
	Ref    string         // string form: "markdown" or "label(max:10)"
	Inline map[string]any // object form: {type: "markdown", maxLength: 500}
}

// Extension carries a UI extension attached to a type (ui-form, ui-table,
// ui-filter, or arbitrary custom extension). Type discriminates; remaining
// keys are opaque payload.
type Extension struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"-"` // remaining keys (everything except "type")
}
