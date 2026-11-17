// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"fmt"
)

// A Def — or a page holding schema-bound nodes — names the schemas it shows
// in three ways: a data-type URI (`schema: ktw-demo:/data-type/book`), the
// input of an action (`schema: {action: <uri>}`, a form whose submit runs
// it), or the AST inline. A `ui` block lays a ui-<kind> extension over it.
// None of that is something a renderer can resolve: ResolveComponents makes
// the whole tree self-contained before it is shown — every schema an AST,
// every ui block laid over it — with the host's fetcher. The object pane's
// ResolvePanel verb is the one host that calls it (a GUI, a TUI and a
// document read through it alike).

// SchemaBoundKinds are the component types whose `schema` names the data they
// show; each reads the `ui-<kind>` extension named by `variant`.
var SchemaBoundKinds = map[string]bool{"form": true, "view": true, "table": true, "tree-table": true, "wizard": true, "timeline-lanes": true}

// ExtensionReferences are, per extension type, the fields of an extension
// that name a variant of another extension type — a composition drawing its
// parts with other variants (a lane timeline's header and cards are ui-view
// variants: `laneHeader` on the lane type, `itemView` on the item type). A
// component resolved by reference keeps those variants too, on whichever
// type of the document declares them.
var ExtensionReferences = map[string]map[string]string{
	"ui-timeline-lanes": {"laneHeader": "ui-view", "itemView": "ui-view", "backlogView": "ui-view"},
}

// SchemaRef is a schema named by reference: a data-type URI, or the input of
// an action.
type SchemaRef struct {
	URI    string
	Action string
}

// String names the reference in an error.
func (r SchemaRef) String() string {
	if r.Action != "" {
		return "the input of " + r.Action
	}
	return r.URI
}

// ComponentResolver is what ResolveComponents asks the host.
type ComponentResolver struct {
	// Fetch answers a referenced schema as its canonical AST (a JSON tree),
	// its type-level extensions filtered to one `ui-<kind>` variant ("*" =
	// every extension of that type). Called once per distinct (ref,
	// extension, variant).
	Fetch func(ref SchemaRef, extensionType, extensionName string) (map[string]any, error)
	// Finish, when set, is applied to every component's schema once it is
	// inline — fetched or authored — before a ui block is laid over it: the
	// host computes the fields' choices (`options_from`) there.
	Finish func(schema map[string]any) (map[string]any, error)
}

// ItemTypeKind is the component a list's `itemType` is drawn by: each
// element is drawn as the `itemVariant` (default DefaultItemVariant) of that
// data type's `ui-view` extensions, so an itemType resolves like a view's
// schema.
const ItemTypeKind = "view"

// DefaultItemVariant is the variant a list draws its typed elements as when
// it names none.
const DefaultItemVariant = "card"

// ResolveComponents makes every schema-bound component of content
// self-contained: a schema named by URI or by action is replaced by its AST,
// an inline one finished, and a `ui` block laid on the root type as the
// variant it names. A list's `itemType` named by URI is resolved the same
// way, as the view its elements are drawn by (its `itemVariant`, default
// card). It walks the whole content — a page's nested nodes, its tabs, list
// templates and the node each element of a list holds in its `nodeField`
// included — and never enters a component's own payload (`schema`,
// `itemType`, `ui`, `data` beyond a list's node field).
func ResolveComponents(content any, r ComponentResolver) (any, error) {
	cache := map[string]map[string]any{}
	var walk func(v any) (any, error)
	walk = func(v any) (any, error) {
		switch n := v.(type) {
		case []any:
			for i := range n {
				out, err := walk(n[i])
				if err != nil {
					return nil, err
				}
				n[i] = out
			}
			return n, nil
		case map[string]any:
			for k, child := range n {
				if isPayload(k) {
					continue // a component's own payload, not nested components
				}
				out, err := walk(child)
				if err != nil {
					return nil, err
				}
				n[k] = out
			}
			for _, el := range listNodeElements(n) {
				out, err := walk(el.m[el.field])
				if err != nil {
					return nil, err
				}
				el.m[el.field] = out
			}
			if err := resolveItemType(n, r, cache); err != nil {
				return nil, err
			}
			kind, _ := n["kind"].(string)
			if !SchemaBoundKinds[kind] {
				return n, nil
			}
			return n, resolveComponent(n, kind, r, cache)
		default:
			return v, nil
		}
	}
	return walk(content)
}

// HasComponents reports whether content holds a schema-bound component —
// something ResolveComponents may change; a host skips the round trip when
// it holds none.
func HasComponents(content any) bool {
	switch n := content.(type) {
	case []any:
		for _, c := range n {
			if HasComponents(c) {
				return true
			}
		}
	case map[string]any:
		if kind, _ := n["kind"].(string); SchemaBoundKinds[kind] {
			return true
		}
		if hasItemType(n) {
			return true
		}
		for _, el := range listNodeElements(n) {
			if HasComponents(el.m[el.field]) {
				return true
			}
		}
		for k, c := range n {
			if !isPayload(k) && HasComponents(c) {
				return true
			}
		}
	}
	return false
}

// isPayload says whether a node's field is a component's own payload, which
// the walkers never enter.
func isPayload(k string) bool {
	return k == "schema" || k == "ui" || k == "data" || k == "itemType"
}

// hasItemType says whether a node names the data type its elements are drawn
// by (a list's `itemType`), by URI or inline.
func hasItemType(n map[string]any) bool {
	switch t := n["itemType"].(type) {
	case string:
		return t != ""
	case map[string]any:
		return true
	}
	return false
}

// resolveItemType resolves a list's `itemType` as the schema of the view each
// element is drawn by: a URI becomes the data type's AST with its
// `itemVariant` `ui-view` extension, an inline one is finished.
func resolveItemType(n map[string]any, r ComponentResolver, cache map[string]map[string]any) error {
	if !hasItemType(n) {
		return nil
	}
	variant, _ := n["itemVariant"].(string)
	if variant == "" {
		variant = DefaultItemVariant
	}
	item := map[string]any{"kind": ItemTypeKind, "schema": n["itemType"], "variant": variant}
	if err := resolveComponent(item, ItemTypeKind, r, cache); err != nil {
		return fmt.Errorf("list itemType: %w", err)
	}
	n["itemType"] = item["schema"]
	return nil
}

// listElement is one element of a list that holds its own node in field.
type listElement struct {
	m     map[string]any
	field string
}

// listNodeElements is, for a list declaring `nodeField`, each element of its
// `data` that holds a node in that field: nodes the page draws, so nodes the
// walkers enter. Any other `data` stays a component's own payload.
func listNodeElements(n map[string]any) []listElement {
	field, _ := n["nodeField"].(string)
	data, isList := n["data"].([]any)
	if field == "" || !isList {
		return nil
	}
	var out []listElement
	for _, d := range data {
		if m, ok := d.(map[string]any); ok && m[field] != nil {
			out = append(out, listElement{m, field})
		}
	}
	return out
}

// SchemaRefOf reads a component's `schema` as a reference: a string is a
// data-type URI, `{action: <uri>}` the input of an action. ok is false for an
// inline AST or no schema.
func SchemaRefOf(schema any) (SchemaRef, bool) {
	switch s := schema.(type) {
	case string:
		return SchemaRef{URI: s}, s != ""
	case map[string]any:
		if _, inline := s["root"]; inline {
			return SchemaRef{}, false
		}
		if a, _ := s["action"].(string); a != "" {
			return SchemaRef{Action: a}, true
		}
	}
	return SchemaRef{}, false
}

func resolveComponent(n map[string]any, kind string, r ComponentResolver, cache map[string]map[string]any) error {
	extType := "ui-" + kind
	variant, _ := n["variant"].(string)
	if variant == "" {
		variant = "default"
	}
	if ref, ok := SchemaRefOf(n["schema"]); ok {
		key := ref.URI + "|" + ref.Action + "|" + extType + "|" + variant
		ast, hit := cache[key]
		if !hit {
			if r.Fetch == nil {
				return fmt.Errorf("%s component: schema %s: no fetcher", kind, ref)
			}
			name := variant
			if name == "default" {
				name = "*" // the default variant may be unnamed in the schema
			}
			var err error
			if ast, err = r.Fetch(ref, extType, name); err != nil {
				return fmt.Errorf("%s component: schema %s: %w", kind, ref, err)
			}
			if r.Finish != nil {
				if ast, err = r.Finish(ast); err != nil {
					return fmt.Errorf("%s component: schema %s: %w", kind, ref, err)
				}
			}
			cache[key] = ast
		}
		n["schema"] = deepCopy(ast)
		if err := keepReferenced(n["schema"].(map[string]any), ref, extType, variant, n["ui"], r, cache); err != nil {
			return fmt.Errorf("%s component: schema %s: %w", kind, ref, err)
		}
	} else if inline, ok := n["schema"].(map[string]any); ok && r.Finish != nil {
		done, err := r.Finish(inline)
		if err != nil {
			return fmt.Errorf("%s component: %w", kind, err)
		}
		n["schema"] = done
	}
	ui, hasUI := n["ui"].(map[string]any)
	if !hasUI {
		return nil
	}
	schema, ok := n["schema"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s component: a ui block needs a schema to lay over", kind)
	}
	block := deepCopy(ui).(map[string]any)
	block["type"] = extType
	name, _ := block["name"].(string)
	if name == "" {
		name = variant
		block["name"] = name
	}
	if err := layOver(schema, block); err != nil {
		return fmt.Errorf("%s component: %w", kind, err)
	}
	n["variant"] = name
	delete(n, "ui")
	return nil
}

// keepReferenced adds to a schema fetched for one `extType` variant the
// variants of other extension types that variant names (ExtensionReferences):
// each is fetched once and its extensions merged, type by type, into the
// schema — a ui block's references (laid over later) count too.
func keepReferenced(schema map[string]any, ref SchemaRef, extType, variant string, ui any, r ComponentResolver, cache map[string]map[string]any) error {
	fields := ExtensionReferences[extType]
	if len(fields) == 0 {
		return nil
	}
	sources := []map[string]any{}
	if block, ok := ui.(map[string]any); ok {
		sources = append(sources, block)
	}
	if ext := rootExtension(schema, extType, variant); ext != nil {
		sources = append(sources, ext)
	}
	seen := map[string]bool{}
	for _, src := range sources {
		for field, refType := range fields {
			name, _ := src[field].(string)
			if name == "" || seen[refType+"|"+name] {
				continue
			}
			seen[refType+"|"+name] = true
			key := ref.URI + "|" + ref.Action + "|" + refType + "|" + name
			got, hit := cache[key]
			if !hit {
				var err error
				if got, err = r.Fetch(ref, refType, name); err != nil {
					return err
				}
				cache[key] = got
			}
			mergeExtensions(schema, got, refType, name)
		}
	}
	return nil
}

// rootExtension is the root type's extension of a type and variant name.
func rootExtension(schema map[string]any, extType, variant string) map[string]any {
	root, _ := schema["root"].(string)
	types, _ := schema["types"].(map[string]any)
	rt, _ := types[root].(map[string]any)
	exts, _ := rt["extensions"].([]any)
	for _, e := range exts {
		if m, _ := e.(map[string]any); m != nil && m["type"] == extType && extName(m) == variant {
			return m
		}
	}
	return nil
}

// mergeExtensions copies from's extensions of a type and name onto the same
// types of into, skipping one into already holds.
func mergeExtensions(into, from map[string]any, extType, name string) {
	dst, _ := into["types"].(map[string]any)
	src, _ := from["types"].(map[string]any)
	for tn, tv := range src {
		st, _ := tv.(map[string]any)
		dt, _ := dst[tn].(map[string]any)
		if st == nil || dt == nil {
			continue
		}
		have, _ := dt["extensions"].([]any)
		exts, _ := st["extensions"].([]any)
		for _, e := range exts {
			m, _ := e.(map[string]any)
			if m == nil || m["type"] != extType || extName(m) != name || holds(have, extType, name) {
				continue
			}
			have = append(have, deepCopy(m))
		}
		if len(have) > 0 {
			dt["extensions"] = have
		}
	}
}

func holds(exts []any, extType, name string) bool {
	for _, e := range exts {
		if m, _ := e.(map[string]any); m != nil && m["type"] == extType && extName(m) == name {
			return true
		}
	}
	return false
}

// layOver puts an extension block on the schema's root type, replacing the
// extension of the same type and name.
func layOver(schema map[string]any, block map[string]any) error {
	root, _ := schema["root"].(string)
	types, _ := schema["types"].(map[string]any)
	rt, ok := types[root].(map[string]any)
	if !ok {
		return fmt.Errorf("the schema has no root type %q", root)
	}
	exts, _ := rt["extensions"].([]any)
	out := make([]any, 0, len(exts)+1)
	for _, e := range exts {
		m, _ := e.(map[string]any)
		if m != nil && m["type"] == block["type"] && extName(m) == block["name"] {
			continue
		}
		out = append(out, e)
	}
	rt["extensions"] = append(out, block)
	return nil
}

func extName(m map[string]any) string {
	if s, _ := m["name"].(string); s != "" {
		return s
	}
	return "default"
}

func deepCopy(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}
