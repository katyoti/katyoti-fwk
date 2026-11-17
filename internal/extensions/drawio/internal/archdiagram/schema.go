// SPDX-License-Identifier: Apache-2.0

// Package archdiagram is the drawio engine's internal pipeline: the pivot
// diagram (typed nodes, edges, zones), ELK layout via elkjs, and the
// template-driven drawio emitter.
package archdiagram

import drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"

// ArchDiagram is the pivot the pipeline lays out and emits. It is built
// from a LogicalDiagram by the translator; positions and sizes are filled
// by the pipeline, never by the caller.
type ArchDiagram struct {
	Title string `json:"title,omitempty"`
	// Viewpoint and Hints are the LogicalDiagram's own, kept so the
	// emitted diagram records them and can be read back into the model.
	Viewpoint string                 `json:"viewpoint,omitempty"`
	Hints     *drawioapi.LayoutHints `json:"layoutHints,omitempty"`
	Page      *drawioapi.PageRef     `json:"page,omitempty"`
	Layout    *LayoutOptions         `json:"layout,omitempty"`
	Zones     []*Zone                `json:"zones,omitempty"`
	Nodes     []*Node                `json:"nodes"`
	Edges     []*Edge                `json:"edges,omitempty"`
	// Mark is the production mark's line drawn under the diagram (docs
	// ProductionMark, resolved by the caller); none when empty.
	Mark string `json:"-"`
}

// LayoutOptions controls the ELK layout algorithm.
// Maps internally to ELK layoutOptions — the field names here are ours, not ELK's.
type LayoutOptions struct {
	Direction        string            `json:"direction,omitempty"`        // RIGHT, DOWN, LEFT, UP (default DOWN)
	Algorithm        string            `json:"algorithm,omitempty"`        // layered, force, stress, radial (default layered)
	NodeSpacing      float64           `json:"nodeSpacing,omitempty"`      // spacing between nodes (default 80)
	LayerSpacing     float64           `json:"layerSpacing,omitempty"`     // spacing between layers (default 120)
	PartitionSpacing float64           `json:"partitionSpacing,omitempty"` // spacing between zone partitions (default 30)
	ExtraOptions     map[string]string `json:"extraOptions,omitempty"`     // additional ELK options (from layout presets)
	Lanes            bool              `json:"lanes,omitempty"`            // zones are bands across the reading direction
	LaneHeader       string            `json:"laneHeader,omitempty"`       // side (default) or top
}

// Zone is a visual grouping. ELK uses Order for partitioning; the emitter
// draws the container around its members after layout.
type Zone struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"` // maps to ELK elk.partitioning.partition
	// Parent is the zone this one is drawn inside.
	Parent string `json:"parent,omitempty"`
	// Kind picks the zone's template (zone:<kind>).
	Kind string `json:"kind,omitempty"`
	// Bounds is populated by layout — not set in input.
	Bounds *Rect `json:"bounds,omitempty"`
}

// Node is a diagram element.
type Node struct {
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Type  string         `json:"type"`                 // the element kind
	Zone  string         `json:"zone,omitempty"`       // zone ID
	Link  string         `json:"link,omitempty"`       // clickable URL (e.g., LeanIX factsheet)
	Data  map[string]any `json:"attributes,omitempty"` // resolver paths read this as `attributes.<key>`
	// Size is set from the stencil set before layout, so layout and emit
	// use the same box.
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	// Position is populated by layout — not set in input.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
}

// Edge is a directed relationship between two nodes.
// Bidirectional flows = two separate edges (A→B, B→A).
type Edge struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Target    string         `json:"target"`
	EdgeStyle string         `json:"edgeStyle,omitempty"`  // the relationship kind
	Label     string         `json:"label,omitempty"`      // the model's own label; the kind's labelSpec may compose another
	Data      map[string]any `json:"attributes,omitempty"` // resolver paths read this as `attributes.<key>`
	// BendPoints is populated by layout — not set in input.
	BendPoints []Point `json:"bendPoints,omitempty"`
	// LabelW and LabelH are the label's estimated size, given to the
	// layout; LabelAt is where the layout put the label's centre.
	LabelW  float64 `json:"-"`
	LabelH  float64 `json:"-"`
	LabelAt *Point  `json:"-"`
}

// Rect is a positioned rectangle (used for zone bounds).
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Point is a 2D coordinate (used for edge bend points).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DefaultLayout returns the default layout options.
func DefaultLayout() *LayoutOptions {
	return &LayoutOptions{
		Direction:        "DOWN",
		Algorithm:        "layered",
		NodeSpacing:      80,
		LayerSpacing:     120,
		PartitionSpacing: 30,
	}
}

// ApplyDefaults fills the layout fields the diagram leaves empty.
func (d *ArchDiagram) ApplyDefaults() {
	dl := DefaultLayout()
	if d.Layout == nil {
		d.Layout = dl
		return
	}
	if d.Layout.Direction == "" {
		d.Layout.Direction = dl.Direction
	}
	if d.Layout.Algorithm == "" {
		d.Layout.Algorithm = dl.Algorithm
	}
	if d.Layout.NodeSpacing == 0 {
		d.Layout.NodeSpacing = dl.NodeSpacing
	}
	if d.Layout.LayerSpacing == 0 {
		d.Layout.LayerSpacing = dl.LayerSpacing
	}
	if d.Layout.PartitionSpacing == 0 {
		d.Layout.PartitionSpacing = dl.PartitionSpacing
	}
}
