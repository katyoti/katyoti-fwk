// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// RunValidator is the exported entry point for re-validating an AST after
// manual edits (used by the service's Validate method). It delegates to the
// internal validate function.
func RunValidator(s *schemaapi.Schema) []schemaapi.Diagnostic { return validate(s) }

// validate walks the AST and reports diagnostics. It runs after shorthand
// expansion (S4) so all references are resolved to either name refs or
// inline types.
//
// Replaces the S5 stub. Rules:
//
//  1. Orphan refs   → error  ORPHAN_REF
//  2. Object: idField default — informational, not an error
//  3. Tree invariant: LinkMode==many2one, LinkType==part_of (auto-corrected, with warning if user set otherwise)
//  4. Content-type compatibility (string→@email ok, number→@email not ok)
//  5. Object catchall vs explicit fields conflict → warning
//  6. Top-level Type kind must be a known kind or a known user-type ref
func validate(s *schemaapi.Schema) []schemaapi.Diagnostic {
	if s == nil {
		return nil
	}
	v := &validator{
		schema: s,
		known:  knownTypeNames(s),
	}
	for name, t := range s.Types {
		v.walkType(t, "types."+name)
	}
	if s.Root != "" {
		if !v.known[s.Root] {
			v.errors = append(v.errors, schemaapi.Error("ORPHAN_ROOT",
				fmt.Sprintf("root type %q not declared", s.Root), "root"))
		}
	}
	return append(append([]schemaapi.Diagnostic(nil), v.errors...), v.warnings...)
}

type validator struct {
	schema   *schemaapi.Schema
	known    map[string]bool
	errors   []schemaapi.Diagnostic
	warnings []schemaapi.Diagnostic
}

func knownTypeNames(s *schemaapi.Schema) map[string]bool {
	out := make(map[string]bool, len(s.Types))
	for k := range s.Types {
		out[k] = true
	}
	return out
}

func (v *validator) err(code, msg, path string) {
	v.errors = append(v.errors, schemaapi.Error(code, msg, path))
}

func (v *validator) warn(code, msg, path string) {
	v.warnings = append(v.warnings, schemaapi.Warning(code, msg, path))
}

func (v *validator) walkType(t *schemaapi.Type, path string) {
	if t == nil {
		return
	}
	v.checkKind(t, path)
	v.checkContent(t, path)
	v.checkEnum(t, path)
	v.checkLinkOrTree(t, path)

	switch t.Kind {
	case schemaapi.KindObject:
		v.walkObject(t, path)
	case schemaapi.KindArray:
		if t.Items != nil {
			v.walkRef(t.Items, path+".items")
		}
	case schemaapi.KindChoice:
		for i, c := range t.Cases {
			v.walkRef(c, fmt.Sprintf("%s.cases[%d]", path, i))
		}
	}

	for _, ext := range t.Extensions {
		_ = ext // extensions are opaque at this layer; UI generators validate them.
	}
}

func (v *validator) walkObject(t *schemaapi.Type, path string) {
	if len(t.Extends) > 0 {
		for _, base := range t.Extends {
			if !v.known[base] {
				v.err("ORPHAN_EXTENDS",
					fmt.Sprintf("extends %q is not a declared type", base),
					path+".extends")
			}
		}
	}
	if t.Catchall != nil {
		v.walkRef(t.Catchall, path+".catchall")
		if len(t.Fields) > 0 {
			// catchall + explicit fields is allowed but worth flagging.
			v.warn("CATCHALL_WITH_FIELDS",
				"object has both `catchall` and explicit `fields`; ensure semantics are intentional",
				path)
		}
	}
	for fname, f := range t.Fields {
		v.walkType(f, path+".fields."+fname)
	}
}

func (v *validator) walkRef(r *schemaapi.TypeRef, path string) {
	if r == nil {
		return
	}
	if r.IsName() {
		if !v.known[r.Name] {
			v.err("ORPHAN_REF",
				fmt.Sprintf("type %q is not declared in this schema", r.Name),
				path)
		}
		return
	}
	if r.IsInline() {
		v.walkType(r.Inline, path)
	}
}

func (v *validator) checkKind(t *schemaapi.Type, path string) {
	if isKnownKind(t.Kind) {
		return
	}
	// Possibly a user-type reference embedded as a Type (e.g. inline `type: User`
	// in a long-form block). Allow if the kind is a known type name.
	if v.known[string(t.Kind)] {
		return
	}
	v.err("UNKNOWN_KIND",
		fmt.Sprintf("unknown type kind %q (not a primitive and not a declared user type)", t.Kind),
		path)
}

func (v *validator) checkContent(t *schemaapi.Type, path string) {
	if t.Content == "" {
		return
	}
	if schemaapi.IsRefContent(t.Content) {
		v.checkRef(t, path)
		return
	}
	allowed, ok := contentTypeCatalog[t.Kind]
	if !ok {
		v.err("CONTENT_NOT_ALLOWED",
			fmt.Sprintf("content type %q on %q is not supported (only string/number/boolean/array carry content types)",
				t.Content, t.Kind),
			path+".content")
		return
	}
	if _, ok := allowed[t.Content]; !ok {
		// Content/field types are defined by the ktw-display-lib field-type
		// manifests (app/ktw-display-lib/src/ui-builders/fields/<name>/manifest.yaml)
		// and are EXTENSIBLE — the Go parser MUST NOT gate on a hardcoded list,
		// or a new field added in the display library would 404 here. The
		// catalog below is only a soft hint for common typos: an unknown content
		// type WARNS but still parses + registers, deferring authority to the
		// display library that actually renders it.
		v.warn("UNKNOWN_CONTENT",
			fmt.Sprintf("content type %q is not in the common set for %s (common: %s) — accepted; authority is the ktw-display-lib field-type manifests",
				t.Content, t.Kind, joinSorted(allowed)),
			path+".content")
	}
}

// checkEnum verifies that t.Enum (when set):
//   - rides on a primitive base kind that admits an enum (string or number);
//     boolean enums are pointless and rejected,
//   - has no duplicate values within a single declaration,
//   - and that each value's Go type is compatible with the base kind.
//
// An empty t.Enum slice never reaches the validator (the parser produces a
// non-empty slice or fails with EMPTY_ENUM); the len > 0 guard is defensive.
func (v *validator) checkEnum(t *schemaapi.Type, path string) {
	if len(t.Enum) == 0 {
		return
	}
	switch t.Kind {
	case schemaapi.KindString:
		seen := make(map[string]struct{}, len(t.Enum))
		for i, val := range t.Enum {
			s, ok := val.(string)
			if !ok {
				v.err("ENUM_TYPE_MISMATCH",
					fmt.Sprintf("string@enum value at index %d is %T, not string", i, val),
					path+".enum")
				continue
			}
			if _, dup := seen[s]; dup {
				v.err("ENUM_DUPLICATE",
					fmt.Sprintf("duplicate string@enum value %q", s), path+".enum")
			}
			seen[s] = struct{}{}
		}
	case schemaapi.KindNumber:
		seen := make(map[float64]struct{}, len(t.Enum))
		for i, val := range t.Enum {
			var f float64
			switch n := val.(type) {
			case int:
				f = float64(n)
			case int64:
				f = float64(n)
			case float64:
				f = n
			default:
				v.err("ENUM_TYPE_MISMATCH",
					fmt.Sprintf("number@enum value at index %d is %T, not numeric", i, val),
					path+".enum")
				continue
			}
			if _, dup := seen[f]; dup {
				v.err("ENUM_DUPLICATE",
					fmt.Sprintf("duplicate number@enum value %v", f), path+".enum")
			}
			seen[f] = struct{}{}
		}
	default:
		v.err("ENUM_NOT_ALLOWED",
			fmt.Sprintf("@enum is only valid on string or number (got %s)", t.Kind),
			path+".enum")
	}
}

func (v *validator) checkLinkOrTree(t *schemaapi.Type, path string) {
	if t.Kind != schemaapi.KindLink && t.Kind != schemaapi.KindTree {
		return
	}
	if t.Target == "" {
		v.err("LINK_NO_TARGET",
			fmt.Sprintf("%s requires `target`", t.Kind), path)
		return
	}
	if !v.known[t.Target] {
		v.err("ORPHAN_LINK_TARGET",
			fmt.Sprintf("link target %q is not declared", t.Target),
			path+".target")
	}
	if t.Kind == schemaapi.KindTree {
		if t.LinkMode != "" && t.LinkMode != schemaapi.LinkMany2One {
			v.warn("TREE_MODE_OVERRIDE",
				"tree.mode is always many2one (override ignored)", path+".mode")
		}
		if t.LinkType != "" && t.LinkType != "part_of" {
			v.warn("TREE_LINKTYPE_OVERRIDE",
				"tree.linkType is always part_of (override ignored)", path+".linkType")
		}
	}
}

// checkRef holds a reference field (`object-ref`, `panel-ref`) to its
// grammar: a string whose params are an ObjectFilter. Its params are left in
// their one form — every list a list of names — so every reader (the JSON
// Schema projection, a picker) reads them the same way.
func (v *validator) checkRef(t *schemaapi.Type, path string) {
	if t.Kind != schemaapi.KindString {
		v.err("REF_NOT_STRING",
			fmt.Sprintf("content type %q holds an id: it goes on a string, not on %q", t.Content, t.Kind),
			path+".content")
		return
	}
	filter, err := schemaapi.NormalizeRefFilter(t.ContentParams)
	if err != nil {
		v.err("REF_FILTER", fmt.Sprintf("%s: %v", t.Content, err), path+".content")
		return
	}
	if len(filter) == 0 {
		t.ContentParams = nil
		return
	}
	t.ContentParams = filter
}

// isKnownKind reports whether the kind is a built-in type kind.
func isKnownKind(k schemaapi.TypeKind) bool {
	switch k {
	case schemaapi.KindString, schemaapi.KindNumber, schemaapi.KindBoolean,
		schemaapi.KindObject, schemaapi.KindArray, schemaapi.KindChoice,
		schemaapi.KindLink, schemaapi.KindTree, schemaapi.KindAny, schemaapi.KindNull:
		return true
	}
	return false
}

// contentTypeCatalog is a SOFT HINT set of the common content types per
// primitive kind — NOT an allow-list. The authoritative, extensible registry
// is the ktw-display-lib field-type manifests
// (app/ktw-display-lib/src/ui-builders/fields/<name>/manifest.yaml); the Go
// parser does not restrict to this set (checkContent only WARNS on a miss).
// TODO(dsl-for-the-dsl): generate/embed this set from those manifests so the
// hint stays in sync, and reconcile drift (e.g. date-time/time/duration are
// `pattern` configs of the `date` field type, not standalone content types;
// boolean/toggle/enum-multi exist in the library but are absent here).
var contentTypeCatalog = map[schemaapi.TypeKind]map[string]struct{}{
	schemaapi.KindString: setOf(
		"text", "email", "url", "uuid",
		"date", "date-time", "time", "duration",
		"html", "markdown", "avatar", "label", "icon",
		"image", "phone", "password", "code", "json", "color",
		"file", // specialized
		// the planning marks, in the Workflows view's palette
		"status", "object-id", "priority", "stage", "alert",
		// a reference (checkRef)
		schemaapi.ContentObjectRef, schemaapi.ContentPanelRef,
	),
	schemaapi.KindNumber: setOf(
		"number", "currency", "percentage", "rating", "slider", "progress",
		"fibonacci", "cod",
	),
	schemaapi.KindBoolean: setOf(
		"boolean", "switch", "warning", "protected", "checked",
	),
	// Array field-types carry content too (basePrimitive: array in the
	// display-lib): enum-multi renders a bounded string[] as a MultiSelect /
	// checkbox group. Presence of the key lets content ride on arrays; unknown
	// array content types still soft-warn (extensible, per checkContent).
	schemaapi.KindArray: setOf(
		"enum-multi",
		// the planning marks: five counts as one segmented
		// bar, gates as dots, steps as joined pills
		"progress-segments", "gate-dots", "stepper",
	),
}

func setOf(items ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, it := range items {
		out[it] = struct{}{}
	}
	return out
}

func joinSorted(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// inline insertion sort (small size)
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && keys[j-1] > keys[j] {
			keys[j-1], keys[j] = keys[j], keys[j-1]
			j--
		}
	}
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}
