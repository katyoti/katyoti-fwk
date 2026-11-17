// SPDX-License-Identifier: Apache-2.0

// validate.go — structural, referential and notation validation for a
// LogicalDiagram.
//
// Validate (this file) catches errors: missing fields, an unknown
// viewpoint, kinds the notation does not declare, relationships its
// rules forbid, dangling edges, duplicate IDs. Lint (in lint.go) catches
// soft issues that do not make the model wrong: long labels, orphan
// nodes, kinds outside the viewpoint, unknown layout presets.
//
// Both produce a ValidationResult with structured (Path, Code, Message)
// issues. CLI verbs in cli-discovery wrap these Go APIs for the
// user-facing surface.
package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// ValidationResult aggregates errors + warnings from a Validate / Lint
// call. Empty Errors means the diagram is renderable; Warnings are
// soft issues the LLM should fix on the next iteration but don't block
// the current render.
type ValidationResult struct {
	Errors   []ValidationIssue
	Warnings []ValidationIssue
}

// ValidationIssue is one error or warning. Code is a stable identifier
// (suitable for unit tests + LLM error-handling switches); Message is
// human-readable.
type ValidationIssue struct {
	Path    string // e.g. "nodes[2].kind"
	Code    string // e.g. "unknown-kind"
	Message string
}

// HasErrors reports whether validation produced any errors.
func (r *ValidationResult) HasErrors() bool { return len(r.Errors) > 0 }

// Validate runs all structural + referential checks. Returns a
// ValidationResult; errors there are blocking, warnings are not.
//
// Returns a Go error ONLY for infrastructure failures (catalog
// unreachable, schema-compile failure). Content errors land in
// Result.Errors.
func Validate(ctx context.Context, ld *drawioapi.LogicalDiagram) (*ValidationResult, error) {
	r := &ValidationResult{}

	if ld == nil {
		r.Errors = append(r.Errors, ValidationIssue{Path: "", Code: "nil-diagram", Message: "diagram is nil"})
		return r, nil
	}

	// 1. JSON Schema validation — catches structural shape errors
	//    (missing required fields, wrong types, invalid URI pattern, etc.).
	if issues, err := validateSchema(ld); err != nil {
		return nil, fmt.Errorf("schema compile: %w", err)
	} else {
		r.Errors = append(r.Errors, issues...)
	}

	// If the schema check failed catastrophically, return early — the
	// other checks assume a well-formed shape.
	if len(r.Errors) > 0 && hasCode(r.Errors, "schema") {
		return r, nil
	}

	// 2. The viewpoint, its notation and its template library resolve.
	dr, err := drawioapi.ResolveDrawing(ctx, ld.Viewpoint)
	if err != nil {
		r.Errors = append(r.Errors, ValidationIssue{
			Path:    "viewpoint",
			Code:    "viewpoint-not-found",
			Message: fmt.Sprintf("viewpoint %q does not resolve: %v", ld.Viewpoint, err),
		})
		return r, nil // the kind checks need the notation
	}
	checkModel(ld, dr.Notation, r)
	return r, nil
}

// checkModel runs the checks that need the notation but no catalog: kinds,
// relationship rules, uniqueness, references.
func checkModel(ld *drawioapi.LogicalDiagram, n *drawioapi.NotationBody, r *ValidationResult) {
	// 3-4. Node and edge kinds — every kind is one the notation declares.
	kindOf := make(map[string]string, len(ld.Nodes))
	for i, nd := range ld.Nodes {
		kindOf[nd.ID] = nd.Kind
		if nd.Kind == "" {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("nodes[%d].kind", i), Code: "missing-kind",
				Message: fmt.Sprintf("node %q has empty kind", nd.ID),
			})
			continue
		}
		if _, ok := n.Elements[nd.Kind]; !ok {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("nodes[%d].kind", i), Code: "unknown-kind",
				Message: fmt.Sprintf("node %q kind %q is not an element kind of notation %s (available: %s)",
					nd.ID, nd.Kind, n.ID, sortedKeys(n.Elements)),
			})
		}
	}
	for i, e := range ld.Edges {
		if e.Kind == "" {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("edges[%d].kind", i), Code: "missing-kind",
				Message: fmt.Sprintf("edge %q has empty kind", e.ID),
			})
			continue
		}
		if _, ok := n.Relationships[e.Kind]; !ok {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("edges[%d].kind", i), Code: "unknown-edge-kind",
				Message: fmt.Sprintf("edge %q kind %q is not a relationship kind of notation %s (available: %s)",
					e.ID, e.Kind, n.ID, sortedKeys(n.Relationships)),
			})
			continue
		}
		// The notation's rules: a relationship it forbids between these
		// two kinds is an error, with the relationships it does allow.
		src, tgt := kindOf[e.Source], kindOf[e.Target]
		if src != "" && tgt != "" && !n.Allows(src, e.Kind, tgt) {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("edges[%d].kind", i), Code: "forbidden-relationship",
				Message: fmt.Sprintf("edge %q: %s does not allow %s from %s to %s (allowed: %s)",
					e.ID, n.ID, e.Kind, src, tgt, allowedBetween(n, src, tgt)),
			})
		}
	}

	// 5-7. Uniqueness — node, edge, zone IDs unique within the diagram.
	nodeIDs := make(map[string]int, len(ld.Nodes))
	for i, n := range ld.Nodes {
		if prev, dup := nodeIDs[n.ID]; dup {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("nodes[%d].id", i), Code: "duplicate-id",
				Message: fmt.Sprintf("duplicate node id %q (first at nodes[%d])", n.ID, prev),
			})
		} else {
			nodeIDs[n.ID] = i
		}
	}
	edgeIDs := make(map[string]int, len(ld.Edges))
	for i, e := range ld.Edges {
		if prev, dup := edgeIDs[e.ID]; dup {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("edges[%d].id", i), Code: "duplicate-id",
				Message: fmt.Sprintf("duplicate edge id %q (first at edges[%d])", e.ID, prev),
			})
		} else {
			edgeIDs[e.ID] = i
		}
	}
	zoneIDs := make(map[string]int, len(ld.Zones))
	for i, z := range ld.Zones {
		if prev, dup := zoneIDs[z.ID]; dup {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("zones[%d].id", i), Code: "duplicate-id",
				Message: fmt.Sprintf("duplicate zone id %q (first at zones[%d])", z.ID, prev),
			})
		} else {
			zoneIDs[z.ID] = i
		}
	}

	// 8-9. Referential — edges reference existing nodes.
	for i, e := range ld.Edges {
		if e.Source != "" {
			if _, ok := nodeIDs[e.Source]; !ok {
				r.Errors = append(r.Errors, ValidationIssue{
					Path: fmt.Sprintf("edges[%d].source", i), Code: "dangling-source",
					Message: fmt.Sprintf("edge %q source %q not in nodes[]", e.ID, e.Source),
				})
			}
		}
		if e.Target != "" {
			if _, ok := nodeIDs[e.Target]; !ok {
				r.Errors = append(r.Errors, ValidationIssue{
					Path: fmt.Sprintf("edges[%d].target", i), Code: "dangling-target",
					Message: fmt.Sprintf("edge %q target %q not in nodes[]", e.ID, e.Target),
				})
			}
		}
	}

	// 10. Node.Zone references an existing zone.
	for i, n := range ld.Nodes {
		if n.Zone != "" {
			if _, ok := zoneIDs[n.Zone]; !ok {
				r.Errors = append(r.Errors, ValidationIssue{
					Path: fmt.Sprintf("nodes[%d].zone", i), Code: "dangling-zone",
					Message: fmt.Sprintf("node %q zone %q not in zones[]", n.ID, n.Zone),
				})
			}
		}
	}

	// 10b. Zone.Parent references an existing zone, without a cycle.
	parentOf := make(map[string]string, len(ld.Zones))
	for _, z := range ld.Zones {
		parentOf[z.ID] = z.Parent
	}
	for i, z := range ld.Zones {
		if z.Parent == "" {
			continue
		}
		if _, ok := zoneIDs[z.Parent]; !ok {
			r.Errors = append(r.Errors, ValidationIssue{
				Path: fmt.Sprintf("zones[%d].parent", i), Code: "dangling-zone",
				Message: fmt.Sprintf("zone %q parent %q not in zones[]", z.ID, z.Parent),
			})
			continue
		}
		seen := map[string]bool{z.ID: true}
		for p := z.Parent; p != ""; p = parentOf[p] {
			if seen[p] {
				r.Errors = append(r.Errors, ValidationIssue{
					Path: fmt.Sprintf("zones[%d].parent", i), Code: "zone-cycle",
					Message: fmt.Sprintf("zone %q is inside itself through its parents", z.ID),
				})
				break
			}
			seen[p] = true
		}
	}

	// 11. Zone.Members references existing nodes.
	for i, z := range ld.Zones {
		for j, mid := range z.Members {
			if _, ok := nodeIDs[mid]; !ok {
				r.Errors = append(r.Errors, ValidationIssue{
					Path: fmt.Sprintf("zones[%d].members[%d]", i, j), Code: "dangling-member",
					Message: fmt.Sprintf("zone %q member %q not in nodes[]", z.ID, mid),
				})
			}
		}
	}

}

// validateSchema runs the JSON Schema check using the embedded schema.
// Returns []ValidationIssue with Code="schema" prefix on every issue.
func validateSchema(ld *drawioapi.LogicalDiagram) ([]ValidationIssue, error) {
	raw, err := json.Marshal(ld)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("logical_diagram.schema.json", bytes.NewReader(drawioapi.LogicalDiagramSchema())); err != nil {
		return nil, err
	}
	schema, err := compiler.Compile("logical_diagram.schema.json")
	if err != nil {
		return nil, err
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return nil, err
	}
	if err := schema.Validate(instance); err != nil {
		// Single combined issue — santhosh-tekuri's error already includes
		// per-field detail in its Error() string.
		return []ValidationIssue{{
			Path:    "",
			Code:    "schema",
			Message: err.Error(),
		}}, nil
	}
	return nil, nil
}

// sortedKeys lists a map's keys, sorted, for error messages.
func sortedKeys[V any](m map[string]V) string {
	if len(m) == 0 {
		return "<none>"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// allowedBetween lists the relationship kinds the notation allows from src
// to tgt, for a forbidden-relationship message.
func allowedBetween(n *drawioapi.NotationBody, src, tgt string) string {
	var ok []string
	for rel := range n.Relationships {
		if n.Allows(src, rel, tgt) {
			ok = append(ok, rel)
		}
	}
	if len(ok) == 0 {
		return "none"
	}
	sort.Strings(ok)
	return strings.Join(ok, ", ")
}

// hasCode reports whether any issue in the slice has the given code prefix.
func hasCode(issues []ValidationIssue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
