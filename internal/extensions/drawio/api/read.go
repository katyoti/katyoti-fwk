// SPDX-License-Identifier: Apache-2.0

package api

// ReadResult is a .drawio file read back into the model: the
// LogicalDiagram it draws, where each thing sits, and what a person pinned.
type ReadResult struct {
	// Diagram is the model. For a diagram this engine wrote it is the
	// model it was written from (ids, kinds, attributes, zones); for a
	// foreign diagram, vertices, edges and containment, with kinds
	// recovered only as far as Report says.
	Diagram *LogicalDiagram `json:"diagram"`
	// Geometry is each node's and zone's box in page coordinates, by
	// model id.
	Geometry map[string]Box `json:"geometry"`
	// EdgePoints are each edge's bend points in page coordinates, by id.
	EdgePoints map[string][]Point `json:"edgePoints,omitempty"`
	// EdgeLabels are the boxes of the edges' labels in page coordinates,
	// by id: the label's estimated size (LabelSize) at the middle of the
	// edge plus the offset draw.io keeps.
	EdgeLabels map[string]Box `json:"edgeLabels,omitempty"`
	// Pinned lists the ids a person fixed in place (ktw-pinned="1", or
	// draw.io's locked style); layout keeps them where they are.
	Pinned []string `json:"pinned,omitempty"`
	// Foreign is true when the file carries no model identity: it was
	// not written by this engine.
	Foreign bool `json:"foreign"`
	// Report lists what the read could not carry over or had to guess:
	// cells added by hand, kinds not recovered, dropped edges.
	Report []string `json:"report,omitempty"`
	// Drawn is every cell of the page as draw.io paints it, in document
	// (paint) order, for the layout check; not serialised.
	Drawn []DrawnCell `json:"-"`
}

// DrawnCell is one painted cell of a page: its draw.io id, label, style
// and box in page coordinates. An edge's box is its label's estimated box
// (EdgeLabels), if it has one.
type DrawnCell struct {
	ID     string
	Parent string
	Label  string
	Style  string
	Box    Box
	Vertex bool
	Edge   bool
	// Hidden is a cell draw.io does not paint: visible="0" on it or on an
	// ancestor (a hidden layer), or opacity=0.
	Hidden bool
	// OnEdge is a label cell whose parent is an edge: its geometry is
	// relative to the edge's path.
	OnEdge bool
}

// Box is a positioned rectangle.
type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Point is a 2D coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// KindUnknown is the kind of a read element no notation claims.
const KindUnknown = "unknown"
