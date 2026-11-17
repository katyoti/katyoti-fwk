// SPDX-License-Identifier: Apache-2.0

// lint.go — soft / warning checks for LogicalDiagram.
//
// Lint runs AFTER Validate. Validate catches errors that block rendering;
// Lint catches issues that don't block but the LLM should fix on the
// next iteration:
//   - Label length above a readable threshold
//   - Orphan node (no edges in/out, no zone membership)
//   - Empty zone (declared but no members)
//   - Unknown layout preset (falls back to default at render time)
//   - A kind outside the viewpoint (the notation knows it; this picture leaves it out)
//   - Future cross-checks against stencil-set resolver paths
//
// Same ValidationResult shape as Validate but populated under Warnings.
package render

import (
	"context"
	"fmt"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// labelMaxLength is the soft threshold above which a node label triggers
// a length warning. drawio renders truncate / wrap labels longer than
// the cell width can hold; ~80 chars is roughly the limit for the
// default 180px node width.
const labelMaxLength = 80

// Lint runs all soft / warning checks. Returns a ValidationResult with
// findings in Warnings (Errors stays empty unless an infrastructure
// failure surfaces).
//
// Lint does NOT re-run the schema / kind-lookup / referential checks
// from Validate — callers SHOULD call Validate first; Lint assumes the
// diagram is structurally valid.
func Lint(ctx context.Context, ld *drawioapi.LogicalDiagram) (*ValidationResult, error) {
	r := &ValidationResult{}
	if ld == nil {
		return r, nil
	}

	// Build the node-edge index for orphan detection.
	connected := make(map[string]bool, len(ld.Nodes))
	for _, e := range ld.Edges {
		connected[e.Source] = true
		connected[e.Target] = true
	}
	zoneMembers := make(map[string]int, len(ld.Zones)) // zone id → member count
	for i, n := range ld.Nodes {
		// Label length warning.
		if len(n.Label) > labelMaxLength {
			r.Warnings = append(r.Warnings, ValidationIssue{
				Path: fmt.Sprintf("nodes[%d].label", i), Code: "long-label",
				Message: fmt.Sprintf("node %q label is %d chars (>%d); drawio will truncate or wrap",
					n.ID, len(n.Label), labelMaxLength),
			})
		}
		// Orphan node — no incoming/outgoing edges AND no zone.
		if !connected[n.ID] && n.Zone == "" {
			r.Warnings = append(r.Warnings, ValidationIssue{
				Path: fmt.Sprintf("nodes[%d]", i), Code: "orphan-node",
				Message: fmt.Sprintf("node %q is orphan (no edges in/out, no zone)", n.ID),
			})
		}
		// Track zone membership for the empty-zone check.
		if n.Zone != "" {
			zoneMembers[n.Zone]++
		}
	}

	// Account for explicit zone.members too, and for the zones drawn
	// inside a zone (a pool's lanes).
	for _, z := range ld.Zones {
		zoneMembers[z.ID] += len(z.Members)
		if z.Parent != "" {
			zoneMembers[z.Parent]++
		}
	}

	// Empty zone — declared but no members and no node.Zone references it.
	for i, z := range ld.Zones {
		if zoneMembers[z.ID] == 0 {
			r.Warnings = append(r.Warnings, ValidationIssue{
				Path: fmt.Sprintf("zones[%d]", i), Code: "empty-zone",
				Message: fmt.Sprintf("zone %q has no members", z.ID),
			})
		}
	}

	// Unknown layout preset (soft — falls back to the default layout).
	// Checked only when the host loaded its presets: without them there
	// is nothing to check against.
	if ld.LayoutHints != nil && ld.LayoutHints.Preset != "" {
		if names := archdiagram.PresetNames(); len(names) > 0 && archdiagram.FindPreset(ld.LayoutHints.Preset) == nil {
			r.Warnings = append(r.Warnings, ValidationIssue{
				Path: "layoutHints.preset", Code: "unknown-preset",
				Message: fmt.Sprintf("layout preset %q is unknown (known: %s); the default layout is used", ld.LayoutHints.Preset, strings.Join(names, ", ")),
			})
		}
	}

	// Kinds the viewpoint does not show: the notation knows them, but this
	// kind of picture leaves them out. Checked when the viewpoint resolves;
	// an unresolved viewpoint is Validate's error.
	if vp, err := drawioapi.ResolveViewpoint(ctx, ld.Viewpoint); err == nil {
		for i, n := range ld.Nodes {
			if n.Kind != "" && !vp.ShowsElement(n.Kind) {
				r.Warnings = append(r.Warnings, ValidationIssue{
					Path: fmt.Sprintf("nodes[%d].kind", i), Code: "outside-viewpoint",
					Message: fmt.Sprintf("node %q kind %q is not one viewpoint %s shows (%s)", n.ID, n.Kind, vp.ID, strings.Join(vp.Elements, ", ")),
				})
			}
		}
		for i, e := range ld.Edges {
			if e.Kind != "" && !vp.ShowsRelationship(e.Kind) {
				r.Warnings = append(r.Warnings, ValidationIssue{
					Path: fmt.Sprintf("edges[%d].kind", i), Code: "outside-viewpoint",
					Message: fmt.Sprintf("edge %q kind %q is not one viewpoint %s shows (%s)", e.ID, e.Kind, vp.ID, strings.Join(vp.Relationships, ", ")),
				})
			}
		}
	}

	return r, nil
}
