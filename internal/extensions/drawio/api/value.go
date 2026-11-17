// SPDX-License-Identifier: Apache-2.0

// value.go — the value primitives a template library's bindings are made
// of (scalar, const, lookup, match, coalesce, expr), and the layout hints
// a diagram or a viewpoint carries.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// LayoutAlgorithm names the ELK algorithm preset.
type LayoutAlgorithm = string

const (
	LayoutLayered LayoutAlgorithm = "layered"
	LayoutForce   LayoutAlgorithm = "force"
	LayoutStress  LayoutAlgorithm = "stress"
	LayoutRadial  LayoutAlgorithm = "radial"
)

// AllLayoutAlgorithms is the closed set of algorithms. Declared here; the kind's data-type is the check.
var AllLayoutAlgorithms = []LayoutAlgorithm{LayoutLayered, LayoutForce, LayoutStress, LayoutRadial}

// LayoutHints carries ELK layout preferences: a viewpoint's layout profile,
// which a LogicalDiagram may override field by field.
//
// Field precedence at translate time:
// when `Preset` is set, the preset's layout options WIN over explicit
// Algorithm/Direction values from the same struct (the LLM/skill named
// a preset intentionally; the preset is the canonical recipe). Preset
// expansion happens in `drawio/render/translate_logical.go::projectLayoutHints`
// via `archdiagram.FindPreset(name)`.
type LayoutHints struct {
	Algorithm  LayoutAlgorithm   `json:"algorithm,omitempty"  jsonschema:"description=ELK algorithm: layered or force or stress or radial"`
	Direction  string            `json:"direction,omitempty"  jsonschema:"description=Reading direction: RIGHT or DOWN or LEFT or UP"`
	ElkOptions map[string]string `json:"elkOptions,omitempty" jsonschema:"description=Raw ELK layout options that win over the preset's"`
	// Lanes draws the zones as lanes: bands across the reading direction
	// that every flow crosses (BPMN pools and lanes).
	Lanes bool `json:"lanes,omitempty" jsonschema:"description=Draw zones as lanes: bands across the reading direction (BPMN pools and lanes)"`
	// LaneHeader places a lane's label: side (the leading edge, the
	// default) or top (above its content, boxes inside boxes).
	LaneHeader string `json:"laneHeader,omitempty" jsonschema:"description=Where a lane's label sits: side (the default) or top (boxes inside boxes)"`

	// Preset is the name of a built-in ELK layout preset
	// (e.g. "archi-layered", "layered-right", "force"). When set, the
	// preset's layout options are applied first; the dedicated fields
	// above act as fine-grained overrides for anything the preset
	// doesn't already declare. Resolved via archdiagram.FindPreset at
	// translate time.
	Preset string `json:"preset,omitempty" jsonschema:"description=Name of a layout preset of the host (embed/elk-layouts)"`
}

// ─────────────────────────────────────────────────────────────────────
// Resolver — closed union of 6 primitives
// ─────────────────────────────────────────────────────────────────────

// ResolverKind discriminates the active variant of a Resolver.
type ResolverKind int

const (
	ResolverInvalid ResolverKind = iota
	ResolverScalar
	ResolverConst
	ResolverLookup
	ResolverMatch
	ResolverCoalesce
	ResolverExpr
)

// String returns the kind name for diagnostics.
func (k ResolverKind) String() string {
	switch k {
	case ResolverScalar:
		return "Scalar"
	case ResolverConst:
		return "Const"
	case ResolverLookup:
		return "Lookup"
	case ResolverMatch:
		return "Match"
	case ResolverCoalesce:
		return "Coalesce"
	case ResolverExpr:
		return "Expr"
	}
	return "Invalid"
}

// Resolver is a closed union of 6 primitives. Exactly one variant is
// populated; UnmarshalJSON dispatches on JSON shape (scalar vs object)
// and discriminator key (const / lookup / match / coalesce / expr).
//
// Marshal/Unmarshal logic:
//   - Scalar  → bare value (string, number, bool)
//   - Const   → {"const": "PATH"}
//   - Lookup  → {"lookup": "PATH", "table": "NAME", "field": "K", "default": V}
//   - Match   → {"match": "PATH", "cases": {...}, "default": V}
//   - Coalesce→ {"coalesce": [Resolver, ...]}
//   - Expr    → {"expr": "EXPR-LANG"}
type Resolver struct {
	// Scalar holds a bare value when the JSON input was not an object.
	// Type is string / float64 / bool depending on the input.
	Scalar any

	// One of the object-variant fields is populated for object inputs:
	Const    string      // discriminator: "const" key present
	Lookup   *LookupSpec // discriminator: "lookup" key present
	Match    *MatchSpec  // discriminator: "match"  key present
	Coalesce []Resolver  // discriminator: "coalesce" key present
	Expr     string      // discriminator: "expr"  key present
}

// OpenSchema marks Resolver's wire form as open in generated schemas: a
// bare value or a one-key object its UnmarshalJSON reads, not its fields.
func (Resolver) OpenSchema() {}

// LookupSpec is the LookupRef body — variant of Resolver.
type LookupSpec struct {
	Path    string `json:"lookup"`
	Table   string `json:"table"`
	Field   string `json:"field,omitempty"`
	Default any    `json:"default"`
}

// MatchSpec is the MatchRef body — variant of Resolver.
type MatchSpec struct {
	Path    string         `json:"match"`
	Cases   map[string]any `json:"cases"`
	Default any            `json:"default"`
}

// Kind returns the active variant of this Resolver, or ResolverInvalid
// when none is populated (or multiple are — shouldn't happen if
// constructed via UnmarshalJSON, but defensive for hand-built Resolvers).
func (r Resolver) Kind() ResolverKind {
	count := 0
	var kind ResolverKind
	if r.Scalar != nil {
		count++
		kind = ResolverScalar
	}
	if r.Const != "" {
		count++
		kind = ResolverConst
	}
	if r.Lookup != nil {
		count++
		kind = ResolverLookup
	}
	if r.Match != nil {
		count++
		kind = ResolverMatch
	}
	if r.Coalesce != nil {
		count++
		kind = ResolverCoalesce
	}
	if r.Expr != "" {
		count++
		kind = ResolverExpr
	}
	if count != 1 {
		return ResolverInvalid
	}
	return kind
}

// IsZero reports whether the Resolver has no variant populated.
func (r Resolver) IsZero() bool {
	return r.Scalar == nil && r.Const == "" && r.Lookup == nil &&
		r.Match == nil && r.Coalesce == nil && r.Expr == ""
}

// UnmarshalJSON dispatches on JSON shape (scalar vs object) and on the
// discriminator key for objects.
func (r *Resolver) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("resolver: empty JSON")
	}
	// Scalar: any non-object non-array. (Arrays as top-level resolvers
	// would be ambiguous; we don't allow them — Coalesce is the array form
	// but it's wrapped in {coalesce: [...]}.)
	if trimmed[0] != '{' {
		return json.Unmarshal(data, &r.Scalar)
	}
	// Object — probe discriminator key.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("resolver: %w", err)
	}
	switch {
	case probe["const"] != nil:
		return json.Unmarshal(probe["const"], &r.Const)
	case probe["expr"] != nil:
		return json.Unmarshal(probe["expr"], &r.Expr)
	case probe["coalesce"] != nil:
		return json.Unmarshal(probe["coalesce"], &r.Coalesce)
	case probe["lookup"] != nil:
		r.Lookup = &LookupSpec{}
		return json.Unmarshal(data, r.Lookup)
	case probe["match"] != nil:
		r.Match = &MatchSpec{}
		return json.Unmarshal(data, r.Match)
	default:
		// Build a sorted key list for the error message.
		keys := make([]string, 0, len(probe))
		for k := range probe {
			keys = append(keys, k)
		}
		return fmt.Errorf("resolver: object has no recognized discriminator (const/lookup/match/coalesce/expr); got keys %v", keys)
	}
}

// MarshalJSON emits the active variant in its canonical form. Multiple
// variants set is treated as a programming error (returns Invalid).
func (r Resolver) MarshalJSON() ([]byte, error) {
	switch r.Kind() {
	case ResolverScalar:
		return json.Marshal(r.Scalar)
	case ResolverConst:
		return json.Marshal(struct {
			Const string `json:"const"`
		}{Const: r.Const})
	case ResolverExpr:
		return json.Marshal(struct {
			Expr string `json:"expr"`
		}{Expr: r.Expr})
	case ResolverCoalesce:
		return json.Marshal(struct {
			Coalesce []Resolver `json:"coalesce"`
		}{Coalesce: r.Coalesce})
	case ResolverLookup:
		return json.Marshal(r.Lookup)
	case ResolverMatch:
		return json.Marshal(r.Match)
	case ResolverInvalid:
		return nil, fmt.Errorf("resolver: zero or multiple variants set; cannot marshal")
	}
	return nil, fmt.Errorf("resolver: unknown kind %d", r.Kind())
}
