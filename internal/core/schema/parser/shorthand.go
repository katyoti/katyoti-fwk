// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"strconv"
	"strings"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// expandShorthand walks the raw types map and produces a typed canonical AST.
// This is the S4 stage: it handles all shorthand forms documented in
// modules/framework/schema/dsl.md, "Shorthand syntax".
//
// Replaces the S2 stub.
func expandShorthand(raw *rawSchema) (*schemaapi.Schema, []schemaapi.Diagnostic, error) {
	s := &schemaapi.Schema{
		DSLVersion:  raw.DSLVersion,
		Name:        raw.Name,
		Version:     raw.Version,
		Description: raw.Description,
		Imports:     append([]string(nil), raw.Imports...),
		Root:        raw.Root,
		Types:       make(map[string]*schemaapi.Type, len(raw.Types)),
	}
	var diags []schemaapi.Diagnostic
	for name, def := range raw.Types {
		path := "types." + name
		t, ds := buildType(def, path)
		diags = append(diags, ds...)
		if t != nil {
			s.Types[name] = t
		}
	}
	return s, diags, nil
}

// buildType produces a Type from a raw value. The raw value may be:
//   - a shorthand string ("string@email?", "Item[]", "->User", "~Category", "A | B")
//   - a long-form block (map with `type:` plus other fields)
//   - any other shape → error diagnostic
func buildType(raw any, path string) (*schemaapi.Type, []schemaapi.Diagnostic) {
	switch v := raw.(type) {
	case string:
		t, diag := parseShorthand(v, path)
		if diag != nil {
			return t, []schemaapi.Diagnostic{*diag}
		}
		return t, nil
	case map[string]any:
		return buildBlock(v, path)
	case map[any]any:
		return buildBlock(toStringMap(v), path)
	default:
		return nil, []schemaapi.Diagnostic{schemaapi.Error("BAD_TYPE_DEF",
			fmt.Sprintf("expected shorthand string or block; got %T", raw), path)}
	}
}

// buildBlock handles the full-block (long-form) representation. The `type:`
// field MAY itself carry shorthand (e.g. `type: string@email`); we parse it
// first to get the base Type, then layer the block's other fields on top.
func buildBlock(m map[string]any, path string) (*schemaapi.Type, []schemaapi.Diagnostic) {
	var t *schemaapi.Type
	var diags []schemaapi.Diagnostic

	if rawType, ok := m["type"]; ok {
		switch v := rawType.(type) {
		case string:
			parsed, derr := parseShorthand(v, path+".type")
			if derr != nil {
				diags = append(diags, *derr)
			}
			if parsed == nil {
				parsed = schemaapi.NewType("")
			}
			t = parsed
		default:
			diags = append(diags, schemaapi.Error("BAD_TYPE_FIELD",
				fmt.Sprintf("type: must be a string; got %T", rawType), path+".type"))
			t = schemaapi.NewType("")
		}
	} else {
		t = schemaapi.NewType("")
	}

	// --- meta ---
	if v, ok := stringField(m, "label"); ok {
		t.Label = v
	}
	if v, ok := stringField(m, "placeholder"); ok {
		t.Placeholder = v
	}
	if v, ok := stringField(m, "description"); ok {
		t.Description = v
	}
	if v, ok := stringField(m, "informations"); ok {
		t.Informations = v
	}
	if v, ok := boolField(m, "optional"); ok {
		t.Optional = v
	}
	if v, ok := m["default"]; ok {
		t.Default = v
	}
	if v, ok := stringSliceField(m, "visibility"); ok {
		t.Visibility = v
	}
	if v, ok := intField(m, "detail"); ok {
		t.Detail = v
	}
	if v, ok := stringSliceField(m, "alias"); ok {
		t.Alias = v
	}

	// --- primitives content ---
	switch raw := m["content"].(type) {
	case nil:
		// no content block
	case string:
		t.Content = raw
	case map[string]any:
		applyContentBlock(t, raw)
	case map[any]any:
		applyContentBlock(t, toStringMap(raw))
	}

	// --- primitives enum (long-form YAML) ---
	// The shorthand form `string@enum(a, b, c)` populates t.Enum via
	// parseShorthand; the long-form block writes `enum: [a, b, c]` and
	// was silently dropped here until 2026-06-03. The validator's
	// checkEnum step expects t.Enum to ride on string/number, so the
	// "kind admits enum" check happens downstream — this stage just
	// captures the values as-is. YAML decodes scalar lists as `[]any`;
	// `[]interface{}` from JSON works identically.
	if v, ok := m["enum"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			t.Enum = arr
		}
	}

	// --- object fields ---
	if rawFields, ok := m["fields"]; ok {
		fm, fmDiags := buildFieldMap(rawFields, path+".fields")
		diags = append(diags, fmDiags...)
		t.Fields = fm
	}
	if v, ok := m["id"]; ok {
		t.IDField = toStringSlice(v)
	}
	if v, ok := m["idField"]; ok {
		t.IDField = toStringSlice(v)
	}
	if v, ok := m["title"]; ok {
		t.TitleField = toStringSlice(v)
	}
	if v, ok := m["titleField"]; ok {
		t.TitleField = toStringSlice(v)
	}
	if v, ok := m["extends"]; ok {
		t.Extends = toStringSlice(v)
	}
	if v, ok := m["catchall"]; ok {
		ref, refDiags := buildRef(v, path+".catchall")
		diags = append(diags, refDiags...)
		t.Catchall = ref
	}

	// --- array ---
	if v, ok := m["items"]; ok {
		ref, refDiags := buildRef(v, path+".items")
		diags = append(diags, refDiags...)
		t.Items = ref
	}
	if v, ok := numberField(m, "min"); ok {
		t.Min = ptrFloat(v)
	}
	if v, ok := numberField(m, "max"); ok {
		t.Max = ptrFloat(v)
	}

	// --- choice ---
	if v, ok := m["types"]; ok && t.Kind == schemaapi.KindChoice {
		cases, caseDiags := buildRefSlice(v, path+".types")
		diags = append(diags, caseDiags...)
		t.Cases = cases
	}

	// --- link / tree ---
	if v, ok := stringField(m, "target"); ok {
		t.Target = v
	}
	if v, ok := stringField(m, "mode"); ok {
		t.LinkMode = schemaapi.LinkMode(v)
	}
	if v, ok := stringField(m, "link_type"); ok {
		t.LinkType = v
	}
	if v, ok := stringField(m, "linkType"); ok {
		t.LinkType = v
	}
	if v, ok := stringField(m, "target_id"); ok {
		t.TargetID = v
	}
	if v, ok := stringField(m, "targetId"); ok {
		t.TargetID = v
	}
	if v, ok := stringField(m, "reverse"); ok {
		t.Reverse = v
	}
	if v, ok := stringField(m, "display"); ok {
		t.Display = v
	}
	if v, ok := boolField(m, "allow_create"); ok {
		t.AllowCreate = ptrBool(v)
	}
	if v, ok := boolField(m, "allowCreate"); ok {
		t.AllowCreate = ptrBool(v)
	}
	if v, ok := boolField(m, "searchable"); ok {
		t.Searchable = ptrBool(v)
	}

	// --- tree extras ---
	if v, ok := m["root_value"]; ok {
		t.RootValue = v
	}
	if v, ok := m["rootValue"]; ok {
		t.RootValue = v
	}
	if v, ok := intField(m, "max_depth"); ok {
		t.MaxDepth = v
	}
	if v, ok := intField(m, "maxDepth"); ok {
		t.MaxDepth = v
	}
	if v, ok := boolField(m, "allow_circular"); ok {
		t.AllowCircular = v
	}
	if v, ok := boolField(m, "allowCircular"); ok {
		t.AllowCircular = v
	}
	if v, ok := boolField(m, "show_path"); ok {
		t.ShowPath = ptrBool(v)
	}
	if v, ok := boolField(m, "showPath"); ok {
		t.ShowPath = ptrBool(v)
	}
	if v, ok := boolField(m, "drag_drop"); ok {
		t.DragDrop = ptrBool(v)
	}
	if v, ok := boolField(m, "dragDrop"); ok {
		t.DragDrop = ptrBool(v)
	}

	// --- extensions ---
	if v, ok := m["extensions"]; ok {
		t.Extensions = buildExtensions(v)
	}

	return t, diags
}

// applyContentBlock copies a `content: { type: ..., min: ..., ... }` block
// onto the Type's Content + ContentParams fields. The shorthand-derived
// Content (if any) is preserved unless the block sets `type:` explicitly.
func applyContentBlock(t *schemaapi.Type, content map[string]any) {
	if v, ok := stringField(content, "type"); ok {
		t.Content = v
	}
	params := make(map[string]any, len(content))
	for k, v := range content {
		if k == "type" {
			continue
		}
		params[k] = v
	}
	if len(params) > 0 {
		if t.ContentParams == nil {
			t.ContentParams = params
		} else {
			for k, v := range params {
				t.ContentParams[k] = v
			}
		}
	}
}

// buildFieldMap converts an object's `fields:` block into a typed map.
func buildFieldMap(raw any, path string) (map[string]*schemaapi.Type, []schemaapi.Diagnostic) {
	switch v := raw.(type) {
	case map[string]any:
		out := make(map[string]*schemaapi.Type, len(v))
		var diags []schemaapi.Diagnostic
		for k, fdef := range v {
			t, fd := buildType(fdef, path+"."+k)
			diags = append(diags, fd...)
			if t != nil {
				out[k] = t
			}
		}
		return out, diags
	case map[any]any:
		return buildFieldMap(toStringMap(v), path)
	default:
		return nil, []schemaapi.Diagnostic{schemaapi.Error("BAD_FIELDS",
			fmt.Sprintf("fields: must be a map; got %T", raw), path)}
	}
}

// buildRef converts a value at a typeref position. A bare identifier referring
// to a user type becomes a name ref; everything else is processed as inline.
func buildRef(raw any, path string) (*schemaapi.TypeRef, []schemaapi.Diagnostic) {
	switch v := raw.(type) {
	case string:
		// Bare-name fast path: if the string is a valid identifier and is NOT
		// a known primitive shorthand keyword, treat as a name ref.
		if isBareIdentifier(v) && !isPrimitiveName(v) {
			return schemaapi.RefName(v), nil
		}
		t, diag := parseShorthand(v, path)
		if diag != nil {
			return nil, []schemaapi.Diagnostic{*diag}
		}
		return schemaapi.RefInline(t), nil
	case map[string]any, map[any]any:
		t, ds := buildType(v, path)
		if t == nil {
			return nil, ds
		}
		return schemaapi.RefInline(t), ds
	default:
		return nil, []schemaapi.Diagnostic{schemaapi.Error("BAD_REF",
			fmt.Sprintf("expected typeref string or block; got %T", raw), path)}
	}
}

// buildRefSlice converts a list of typerefs (e.g. choice types).
func buildRefSlice(raw any, path string) ([]*schemaapi.TypeRef, []schemaapi.Diagnostic) {
	switch v := raw.(type) {
	case []any:
		out := make([]*schemaapi.TypeRef, 0, len(v))
		var diags []schemaapi.Diagnostic
		for i, item := range v {
			ref, ds := buildRef(item, fmt.Sprintf("%s[%d]", path, i))
			diags = append(diags, ds...)
			if ref != nil {
				out = append(out, ref)
			}
		}
		return out, diags
	default:
		return nil, []schemaapi.Diagnostic{schemaapi.Error("BAD_REF_LIST",
			fmt.Sprintf("expected list of typerefs; got %T", raw), path)}
	}
}

// buildExtensions converts a list of UI extensions to schemaapi.Extension entries.
// Each entry is a map with at least `type:` plus arbitrary payload.
func buildExtensions(raw any) []schemaapi.Extension {
	var src []any
	switch v := raw.(type) {
	case []any:
		src = v
	default:
		return nil
	}
	out := make([]schemaapi.Extension, 0, len(src))
	for _, item := range src {
		var m map[string]any
		switch v := item.(type) {
		case map[string]any:
			m = v
		case map[any]any:
			m = toStringMap(v)
		default:
			continue
		}
		kind, _ := m["type"].(string)
		payload := make(map[string]any, len(m))
		for k, v := range m {
			if k == "type" {
				continue
			}
			payload[k] = v
		}
		out = append(out, schemaapi.Extension{Kind: kind, Payload: payload})
	}
	return out
}

// parseShorthand parses a shorthand string into a Type. The grammar (per
// modules/framework/schema/dsl.md, "Shorthand syntax"):
//
//	expr        := choice | unary
//	choice      := unary ('|' unary)+        (top-level only; not inside parens)
//	unary       := link | tree | array | base
//	link        := '->' name [ '[]' ] [ '?' ]
//	tree        := '~' name [ '?' ]
//	array       := unary '[]' [ '?' ]
//	base        := name [ '@' content ] [ '(' params ')' ] [ '?' ]
//	params      := ident ':' value (',' ident ':' value)*
func parseShorthand(s, path string) (*schemaapi.Type, *schemaapi.Diagnostic) {
	src := strings.TrimSpace(s)
	if src == "" {
		d := schemaapi.Error("EMPTY_SHORTHAND", "empty shorthand", path)
		return nil, &d
	}

	// 1) top-level `|` → choice
	if cases := splitTopLevel(src, '|'); len(cases) > 1 {
		t := schemaapi.NewType(schemaapi.KindChoice)
		for i, c := range cases {
			ct, derr := parseShorthand(c, fmt.Sprintf("%s|%d", path, i))
			if derr != nil {
				return nil, derr
			}
			t.Cases = append(t.Cases, schemaapi.RefInline(ct))
		}
		return t, nil
	}

	// 2) trailing `?` → optional flag, strip and recurse into inner.
	optional := false
	if strings.HasSuffix(src, "?") {
		optional = true
		src = strings.TrimSuffix(src, "?")
		src = strings.TrimSpace(src)
	}

	// 3) trailing `[]` → array wrapper; recurse on inner.
	if strings.HasSuffix(src, "[]") {
		inner := strings.TrimSpace(strings.TrimSuffix(src, "[]"))
		if inner == "" {
			d := schemaapi.Error("EMPTY_ARRAY_INNER", "array shorthand has no element type", path)
			return nil, &d
		}
		t := schemaapi.NewType(schemaapi.KindArray)
		t.Optional = optional
		// Special: link `->User[]` is many2many, not array of link. Detect here.
		if strings.HasPrefix(inner, "->") {
			name := strings.TrimSpace(inner[2:])
			if !isBareIdentifier(name) {
				d := schemaapi.Error("BAD_LINK_TARGET", "expected identifier after `->`", path)
				return nil, &d
			}
			lt := schemaapi.NewType(schemaapi.KindLink)
			lt.Target = name
			lt.LinkMode = schemaapi.LinkMany2Many
			lt.Optional = optional
			return lt, nil
		}
		// otherwise: array of typeref
		ct, derr := parseShorthand(inner, path+".items")
		if derr != nil {
			return nil, derr
		}
		// If the inner is a bare user-type name, store as name ref instead of inline primitive.
		if isBareIdentifier(inner) && !isPrimitiveName(inner) {
			t.Items = schemaapi.RefName(inner)
		} else {
			t.Items = schemaapi.RefInline(ct)
		}
		return t, nil
	}

	// 4) `->Name` → link many2one (no [] after)
	if strings.HasPrefix(src, "->") {
		name := strings.TrimSpace(src[2:])
		if name == "" {
			d := schemaapi.Error("BAD_LINK_TARGET", "expected identifier after `->`", path)
			return nil, &d
		}
		// allow inline params like ->User(mode:many2one, reverse:reports)
		var params map[string]any
		if i := strings.Index(name, "("); i >= 0 {
			if !strings.HasSuffix(name, ")") {
				d := schemaapi.Error("BAD_LINK_PARAMS", "missing `)` in link shorthand", path)
				return nil, &d
			}
			rawParams := name[i+1 : len(name)-1]
			name = strings.TrimSpace(name[:i])
			p, derr := parseInlineParams(rawParams, path)
			if derr != nil {
				return nil, derr
			}
			params = p
		}
		if !isBareIdentifier(name) {
			d := schemaapi.Error("BAD_LINK_TARGET", "expected identifier after `->`", path)
			return nil, &d
		}
		t := schemaapi.NewType(schemaapi.KindLink)
		t.Target = name
		t.LinkMode = schemaapi.LinkMany2One
		t.Optional = optional
		applyLinkParams(t, params)
		return t, nil
	}

	// 5) `~Name` → tree
	if strings.HasPrefix(src, "~") {
		name := strings.TrimSpace(src[1:])
		var params map[string]any
		if i := strings.Index(name, "("); i >= 0 {
			if !strings.HasSuffix(name, ")") {
				d := schemaapi.Error("BAD_TREE_PARAMS", "missing `)` in tree shorthand", path)
				return nil, &d
			}
			rawParams := name[i+1 : len(name)-1]
			name = strings.TrimSpace(name[:i])
			p, derr := parseInlineParams(rawParams, path)
			if derr != nil {
				return nil, derr
			}
			params = p
		}
		if !isBareIdentifier(name) {
			d := schemaapi.Error("BAD_TREE_TARGET", "expected identifier after `~`", path)
			return nil, &d
		}
		t := schemaapi.NewType(schemaapi.KindTree)
		t.Target = name
		t.LinkMode = schemaapi.LinkMany2One
		t.LinkType = "part_of"
		t.Optional = optional
		applyTreeParams(t, params)
		return t, nil
	}

	// 6) base form: `name[@content][(params)]`
	return parseBase(src, optional, path)
}

func parseBase(src string, optional bool, path string) (*schemaapi.Type, *schemaapi.Diagnostic) {
	// Split off inline params: `name@content(p:v, p:v)`.
	var rawParams string
	if i := strings.Index(src, "("); i >= 0 {
		if !strings.HasSuffix(src, ")") {
			d := schemaapi.Error("BAD_PARAMS", "missing `)` in shorthand params", path)
			return nil, &d
		}
		rawParams = src[i+1 : len(src)-1]
		src = src[:i]
	}

	// Split base type vs content type.
	typeName := src
	contentName := ""
	if i := strings.Index(src, "@"); i >= 0 {
		typeName = strings.TrimSpace(src[:i])
		contentName = strings.TrimSpace(src[i+1:])
	}

	if typeName == "" {
		d := schemaapi.Error("EMPTY_TYPE", "shorthand has no type name", path)
		return nil, &d
	}

	t := schemaapi.NewType(schemaapi.TypeKind(typeName))
	if !isPrimitiveName(typeName) {
		// User-type reference: keep as a Type whose Kind is the type name and
		// has no fields. The validator (S5) is responsible for reporting unknown
		// types. Downstream consumers treat this as a name reference equivalent.
		// Note: for typeref positions, buildRef short-circuits this case to
		// produce a clean Name ref instead.
		t.Kind = schemaapi.TypeKind(typeName)
	}
	t.Optional = optional

	// `@enum(a,b,c)` is structural sugar — populate t.Enum directly with the
	// positional values; do NOT carry "enum" as a content-type. Mutually
	// exclusive with other content-types (enforced by the parser: parseBase
	// only sees one `@<name>` token).
	if contentName == "enum" {
		values, derr := parseEnumValues(rawParams, path)
		if derr != nil {
			return nil, derr
		}
		t.Enum = values
		return t, nil
	}

	if contentName != "" {
		t.Content = contentName
	}

	if rawParams != "" {
		params, derr := parseInlineParams(rawParams, path)
		if derr != nil {
			return nil, derr
		}
		t.ContentParams = params
	}

	return t, nil
}

// parseEnumValues parses the positional value list inside `@enum(...)`.
// Each comma-separated token is coerced via parseScalar (int, float, bool,
// string). An empty list is a parse error — `@enum()` is meaningless.
func parseEnumValues(raw, path string) ([]any, *schemaapi.Diagnostic) {
	if strings.TrimSpace(raw) == "" {
		d := schemaapi.Error("EMPTY_ENUM", "`@enum(...)` requires at least one value", path)
		return nil, &d
	}
	parts := splitTopLevel(raw, ',')
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			d := schemaapi.Error("EMPTY_ENUM_VALUE", "empty `@enum` value (stray comma?)", path)
			return nil, &d
		}
		out = append(out, parseScalar(p))
	}
	return out, nil
}

// parseInlineParams parses `key:value, key:value` into a map. Values are
// interpreted as int, float, bool, or string (in that order).
func parseInlineParams(raw, path string) (map[string]any, *schemaapi.Diagnostic) {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	parts := splitTopLevel(raw, ',')
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idx := strings.Index(p, ":")
		if idx < 0 {
			// A bare name is a flag: `gate-dots(labels)` = labels: true.
			if isParamName(p) {
				out[p] = true
				continue
			}
			d := schemaapi.Error("BAD_PARAM",
				fmt.Sprintf("inline param missing ':' — %q", p), path)
			return nil, &d
		}
		key := strings.TrimSpace(p[:idx])
		val := strings.TrimSpace(p[idx+1:])
		out[key] = parseScalar(val)
	}
	return out, nil
}

// parseScalar converts an inline-params value to int / float / bool / string.
func parseScalar(s string) any {
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if s == "null" {
		return nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	// strip surrounding quotes if present
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// applyLinkParams maps inline link shorthand params to LinkType fields.
func applyLinkParams(t *schemaapi.Type, p map[string]any) {
	if v, ok := p["mode"].(string); ok {
		t.LinkMode = schemaapi.LinkMode(v)
	}
	if v, ok := p["target_id"].(string); ok {
		t.TargetID = v
	}
	if v, ok := p["targetId"].(string); ok {
		t.TargetID = v
	}
	if v, ok := p["link_type"].(string); ok {
		t.LinkType = v
	}
	if v, ok := p["linkType"].(string); ok {
		t.LinkType = v
	}
	if v, ok := p["reverse"].(string); ok {
		t.Reverse = v
	}
	if v, ok := p["display"].(string); ok {
		t.Display = v
	}
	if v, ok := p["allow_create"].(bool); ok {
		t.AllowCreate = ptrBool(v)
	}
	if v, ok := p["allowCreate"].(bool); ok {
		t.AllowCreate = ptrBool(v)
	}
	if v, ok := p["searchable"].(bool); ok {
		t.Searchable = ptrBool(v)
	}
}

// applyTreeParams maps inline tree shorthand params to TreeType fields.
// Tree mode and link_type are fixed (many2one, part_of); other fields apply.
func applyTreeParams(t *schemaapi.Type, p map[string]any) {
	applyLinkParams(t, p)
	t.LinkMode = schemaapi.LinkMany2One
	t.LinkType = "part_of"
	if v, ok := p["max_depth"].(int64); ok {
		t.MaxDepth = int(v)
	}
	if v, ok := p["maxDepth"].(int64); ok {
		t.MaxDepth = int(v)
	}
	if v, ok := p["allow_circular"].(bool); ok {
		t.AllowCircular = v
	}
	if v, ok := p["show_path"].(bool); ok {
		t.ShowPath = ptrBool(v)
	}
	if v, ok := p["showPath"].(bool); ok {
		t.ShowPath = ptrBool(v)
	}
	if v, ok := p["drag_drop"].(bool); ok {
		t.DragDrop = ptrBool(v)
	}
}

// splitTopLevel splits by sep at the top level (not inside parens or brackets).
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case sep:
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[last:i]))
				last = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[last:]))
	return out
}

// isBareIdentifier reports whether s is a simple identifier (letters, digits,
// underscore; no shorthand decorations).
func isBareIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// isPrimitiveName reports whether the name is a built-in type keyword.
func isPrimitiveName(s string) bool {
	switch s {
	case "string", "number", "boolean", "any", "null", "object", "array", "choice", "link", "tree":
		return true
	}
	return false
}

// --- value coercion helpers ---

func stringField(m map[string]any, k string) (string, bool) {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

func boolField(m map[string]any, k string) (bool, bool) {
	if v, ok := m[k]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

func intField(m map[string]any, k string) (int, bool) {
	if v, ok := m[k]; ok {
		switch n := v.(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
	}
	return 0, false
}

func numberField(m map[string]any, k string) (float64, bool) {
	if v, ok := m[k]; ok {
		switch n := v.(type) {
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		case float64:
			return n, true
		}
	}
	return 0, false
}

func stringSliceField(m map[string]any, k string) ([]string, bool) {
	v, ok := m[k]
	if !ok {
		return nil, false
	}
	return toStringSlice(v), true
}

// toStringSlice coerces YAML / JSON list-or-scalar values to []string.
func toStringSlice(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), x...)
	}
	return nil
}

// toStringMap converts a YAML-decoded map[any]any (yaml.v2 style) to map[string]any.
// yaml.v3 already produces map[string]any for string-keyed mappings, so this is
// mostly a defensive helper.
func toStringMap(m map[any]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if ks, ok := k.(string); ok {
			out[ks] = convertNestedMaps(v)
		}
	}
	return out
}

func convertNestedMaps(v any) any {
	switch x := v.(type) {
	case map[any]any:
		return toStringMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convertNestedMaps(e)
		}
		return out
	}
	return v
}

func ptrBool(b bool) *bool        { return &b }
func ptrFloat(f float64) *float64 { return &f }

// isParamName reports whether s is a bare param name (a flag): a letter, then
// letters, digits, '_' or '-'.
func isParamName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_' || r == '-'):
		default:
			return false
		}
	}
	return true
}
