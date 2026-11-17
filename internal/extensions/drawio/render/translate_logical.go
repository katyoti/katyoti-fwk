// SPDX-License-Identifier: Apache-2.0

// translate_logical.go — bridges the model (a LogicalDiagram) to the
// renderer's internal ArchDiagram pivot.
//
// Pipeline:
//
//  1. Resolve the viewpoint, its notation and its template library from the
//     universe catalog (`drawioapi.ResolveDrawing`).
//  2. Project nodes / edges / zones one-for-one. Field-name mapping:
//     LogicalNode.Kind       → archdiagram.Node.Type
//     LogicalNode.Attributes → archdiagram.Node.Data
//     LogicalEdge.Kind       → archdiagram.Edge.EdgeStyle
//     LogicalEdge.Attributes → archdiagram.Edge.Data
//  3. Project the layout: the diagram's own hints win field by field over
//     the viewpoint's layout profile; presets expand on both sides;
//     archdiagram's defaults backstop the rest.
package render

import (
	"context"
	"fmt"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// TranslateLogicalDiagram resolves the viewpoint and projects a
// *LogicalDiagram into an *ArchDiagram pivot. Returns the pivot AND the
// resolved drawing (so the caller renders without a second lookup).
//
// Returns a Go error for infrastructure failures (URI parse, catalog miss).
// Content-level issues (unknown kinds, dangling edges) are NOT errors here —
// catch them with drawioapi.Validate / Lint before calling TranslateLogicalDiagram.
func TranslateLogicalDiagram(ctx context.Context, ld *drawioapi.LogicalDiagram) (*archdiagram.ArchDiagram, *drawioapi.Drawing, error) {
	if ld == nil {
		return nil, nil, fmt.Errorf("TranslateLogicalDiagram: nil diagram")
	}

	dr, err := drawioapi.ResolveDrawing(ctx, ld.Viewpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve viewpoint %q: %w", ld.Viewpoint, err)
	}

	d := &archdiagram.ArchDiagram{Title: ld.Title, Viewpoint: ld.Viewpoint, Hints: ld.LayoutHints, Page: ld.Page}

	d.Nodes = make([]*archdiagram.Node, 0, len(ld.Nodes))
	for i := range ld.Nodes {
		n := &ld.Nodes[i]
		d.Nodes = append(d.Nodes, &archdiagram.Node{
			ID:    n.ID,
			Label: n.Label,
			Type:  n.Kind,
			Zone:  n.Zone,
			Data:  n.Attributes,
		})
	}

	d.Edges = make([]*archdiagram.Edge, 0, len(ld.Edges))
	for i := range ld.Edges {
		e := &ld.Edges[i]
		d.Edges = append(d.Edges, &archdiagram.Edge{
			ID:        e.ID,
			Source:    e.Source,
			Target:    e.Target,
			EdgeStyle: e.Kind,
			Label:     e.Label,
			Data:      e.Attributes,
		})
	}

	// A zone may list its members instead of each node naming its zone;
	// a node's own zone wins.
	byID := make(map[string]*archdiagram.Node, len(d.Nodes))
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	for _, z := range ld.Zones {
		for _, m := range z.Members {
			if n, ok := byID[m]; ok && n.Zone == "" {
				n.Zone = z.ID
			}
		}
	}

	d.Zones = make([]*archdiagram.Zone, 0, len(ld.Zones))
	for i := range ld.Zones {
		z := &ld.Zones[i]
		d.Zones = append(d.Zones, &archdiagram.Zone{
			ID:     z.ID,
			Label:  z.Label,
			Order:  z.Order,
			Parent: z.Parent,
			Kind:   z.Kind,
		})
	}

	d.Layout = mergeLayout(projectLayoutHints(ld.LayoutHints), projectLayoutHints(dr.Viewpoint.Layout))

	return d, dr, nil
}

// mergeLayout fills the fields the diagram left empty from the viewpoint's
// profile. Either may be nil.
func mergeLayout(own, profile *archdiagram.LayoutOptions) *archdiagram.LayoutOptions {
	if own == nil {
		return profile
	}
	if profile == nil {
		return own
	}
	if own.Algorithm == "" {
		own.Algorithm = profile.Algorithm
	}
	if own.Direction == "" {
		own.Direction = profile.Direction
	}
	if own.NodeSpacing == 0 {
		own.NodeSpacing = profile.NodeSpacing
	}
	if own.LayerSpacing == 0 {
		own.LayerSpacing = profile.LayerSpacing
	}
	if own.PartitionSpacing == 0 {
		own.PartitionSpacing = profile.PartitionSpacing
	}
	own.Lanes = own.Lanes || profile.Lanes
	if own.LaneHeader == "" {
		own.LaneHeader = profile.LaneHeader
	}
	for k, v := range profile.ExtraOptions {
		if own.ExtraOptions == nil {
			own.ExtraOptions = map[string]string{}
		}
		if _, set := own.ExtraOptions[k]; !set {
			own.ExtraOptions[k] = v
		}
	}
	return own
}

// projectLayoutHints converts LayoutHints (a diagram's or a viewpoint's)
// into archdiagram's internal LayoutOptions shape.
//
// Precedence (per the zone-partitioned-layout 2026-05-28 analysis):
//
//  1. Explicit Algorithm + Direction fields apply first (the LLM's
//     coarse-grained intent).
//  2. Preset — if h.Preset names a registered preset, its layout options
//     OVERRIDE the explicit Algorithm + Direction. Rationale: a named
//     preset IS the canonical recipe; the LLM names it on purpose; the
//     preset wins over loose individual fields.
//  3. Explicit ElkOptions OVERRIDE preset options per-key (most
//     fine-grained wins — the LLM is tweaking specific knobs).
//  4. mergeLayout fills the diagram's still-empty fields from the
//     viewpoint's profile, projected the same way.
//  5. archdiagram.DefaultLayout() backstops anything still empty.
//
// Unknown preset names are ignored here (the explicit fields path still
// works); Lint reports them.
func projectLayoutHints(h *drawioapi.LayoutHints) *archdiagram.LayoutOptions {
	return projectLayoutHintsWith(h, archdiagram.FindPreset)
}

// presetFinder is the function shape projectLayoutHints uses to resolve
// a preset name → *LayoutPreset. Extracted so tests can inject a fake
// without touching archdiagram's global preset registry.
type presetFinder func(name string) *archdiagram.LayoutPreset

// projectLayoutHintsWith is projectLayoutHints with the preset lookup
// injected. Production code calls projectLayoutHints (which threads the
// real archdiagram.FindPreset in); tests construct their own finder.
func projectLayoutHintsWith(h *drawioapi.LayoutHints, find presetFinder) *archdiagram.LayoutOptions {
	if h == nil {
		return nil
	}
	out := &archdiagram.LayoutOptions{
		Algorithm:  h.Algorithm,
		Direction:  h.Direction,
		Lanes:      h.Lanes,
		LaneHeader: h.LaneHeader,
	}

	// Preset (if any) wins over the explicit Algorithm + Direction set above.
	if h.Preset != "" && find != nil {
		if p := find(h.Preset); p != nil {
			p.ApplyToLayout(out)
			out.ExtraOptions = p.ExtraELKOptions()
		}
	}

	// Caller's explicit ElkOptions override the preset's options per-key.
	if len(h.ElkOptions) > 0 {
		if out.ExtraOptions == nil {
			out.ExtraOptions = make(map[string]string, len(h.ElkOptions))
		}
		for k, v := range h.ElkOptions {
			out.ExtraOptions[k] = v
		}
	}

	return out
}

// copyStringMap clones a map[string]string. Nil-safe.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
