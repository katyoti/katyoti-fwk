// SPDX-License-Identifier: Apache-2.0

package api

// QualityReport measures how a laid-out diagram reads. Counts are "lower is
// better"; Overlaps must be zero.
type QualityReport struct {
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`
	// Overlaps counts pairs of nodes whose boxes intersect, a box drawn
	// wholly inside another (a grouping) aside.
	Overlaps int `json:"overlaps"`
	// Crossings counts pairs of edge segments that cross (edges sharing an
	// end are not counted at that end).
	Crossings int `json:"crossings"`
	// EdgesThroughNodes counts (edge, node) pairs where an edge passes
	// through a node that is neither of its ends.
	EdgesThroughNodes int `json:"edgesThroughNodes"`
	// Bends counts the bend points of every edge (a waypoint on an end
	// node's border is an attachment, not a bend).
	Bends int `json:"bends"`
	// ZoneEscapes counts nodes drawn outside the box of their zone.
	ZoneEscapes int `json:"zoneEscapes"`
	// ZoneOverlaps counts pairs of zones whose boxes intersect, a zone
	// drawn wholly inside another (nesting) aside.
	ZoneOverlaps int `json:"zoneOverlaps"`
	// LabelCollisions counts edge labels drawn over another edge's label
	// or over a node.
	LabelCollisions int `json:"labelCollisions"`
	// ZoneIntrusions counts nodes drawn over a zone they are not in (a
	// member of a zone nested inside it aside).
	ZoneIntrusions int `json:"zoneIntrusions"`
	// TextOverlaps counts pairs of labels whose painted text intersects
	// (by more than LayoutTolerance both ways): the one layout finding
	// that fails a check.
	TextOverlaps int `json:"textOverlaps"`
	// Duplicates counts pairs of labels with the same text in the same box.
	Duplicates int `json:"duplicates"`
	// TextOverflows counts labels painted out of their own shape.
	TextOverflows int `json:"textOverflows"`
	// BoxCovers counts filled shapes covering each other (neither holding
	// the other) and labels hidden under a later opaque shape.
	BoxCovers int `json:"boxCovers"`
	// Width and Height are the drawing's bounding box.
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Density is the share of the bounding box covered by nodes (0 to 1):
	// low means sprawl, high means crowding.
	Density float64 `json:"density"`
	// MeanEdgeLength is the mean length of the edges' polylines.
	MeanEdgeLength float64 `json:"meanEdgeLength"`
}

// LayoutTolerance is how far, in pixels, two boxes may intersect both ways
// before a layout finding counts them: anti-aliasing and side bearings
// make touching boxes meet by a pixel or two.
const LayoutTolerance = 4.0

// The kinds of layout finding.
const (
	// FindingTextOverlap is two labels painted over each other — gated.
	FindingTextOverlap = "text-overlap"
	// FindingDuplicate is the same text painted twice in the same box.
	FindingDuplicate = "duplicate"
	// FindingTextOverflow is a label painted out of its own shape.
	FindingTextOverflow = "text-overflow"
	// FindingBoxCover is a filled shape covering another, or a label.
	FindingBoxCover = "box-cover"
	// FindingNotMeasured is a label the check cannot place (a label cell
	// on an edge).
	FindingNotMeasured = "not-measured"
)

// Finding is one layout problem of a drawing: its kind, whether it fails a
// check (Gated), the cells it involves with their labels, and the box where
// they meet (Depth: how far they intersect, the smaller of both ways).
type Finding struct {
	Kind    string   `json:"kind"`
	Gated   bool     `json:"gated"`
	CellIDs []string `json:"cellIds"`
	Labels  []string `json:"labels,omitempty"`
	Box     Box      `json:"box"`
	Depth   float64  `json:"depth"`
	// Page names the page it is on, in a drawing of several pages.
	Page string `json:"page,omitempty"`
}
