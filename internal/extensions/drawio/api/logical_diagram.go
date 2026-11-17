// SPDX-License-Identifier: Apache-2.0

// logical_diagram.go — the LLM-facing pivot for the v2 drawio chain.
//
// The chain:
//
//	[Source-specific extractor]    →    LogicalDiagram (this file)    →    ArchDiagram (internal)    →    drawio
//	(LeanIX walk, Dynatrace, ...)        (LLM intent — kind+attributes)        (renderer pivot)
//
// A `LogicalDiagram` carries the LLM's intent at the right abstraction level:
//   - WHAT the diagram contains (nodes, edges, zones)
//   - WHICH viewpoint draws it (viewpoint URI: a notation, its template
//     library and a layout profile)
//   - the attributes the templates' placeholders and bindings read
//
// What it doesn't carry — and what makes it different from `ArchDiagram`:
//   - Pre-computed positions (ELK auto-layout owns that)
//   - Styles (the template library owns them)
//
// Field names match the template scope: `kind` (not `type`), `attributes`
// (not `data`, not `factsheet`). Bindings read paths like
// `attributes.domain`, `attributes.fl_codes`; a template reads a scalar
// attribute as `%domain%`.

package api

// LogicalDiagram is the model the drawio engine draws. Authored by an LLM
// (or a skill-driven transform), validated against the JSON Schema at
// logical_diagram.schema.json and against its viewpoint's notation, then
// translated into the internal pivot via render.TranslateLogicalDiagram.
type LogicalDiagram struct {
	// Viewpoint is the catalog URI of the viewpoint that draws the
	// diagram (e.g. "ktw-docs:/viewpoint/archimate3-layered"). Required.
	Viewpoint string `json:"viewpoint"`

	// Title is an optional human-readable title.
	Title string `json:"title,omitempty"`

	// Nodes is the list of diagram nodes. At least one is required.
	Nodes []LogicalNode `json:"nodes"`

	// Edges is the list of directed edges between nodes. Optional.
	Edges []LogicalEdge `json:"edges,omitempty"`

	// Zones partition nodes into named groups (swimlanes / containers).
	// Optional. When set, the renderer can enable ELK partitioning
	// to lay out nodes inside their zones.
	Zones []LogicalZone `json:"zones,omitempty"`

	// LayoutHints control the ELK layout algorithm. Optional —
	// defaults come from the viewpoint's layout profile, then
	// from the global default.
	LayoutHints *LayoutHints `json:"layoutHints,omitempty"`

	// Page picks the frame the diagram is drawn in and fills it. Optional —
	// the viewpoint names a page by default.
	Page *PageRef `json:"page,omitempty"`

	// Mark is the production mark under the diagram (docs ProductionMark:
	// ArchiFactory, its version, the date): on when empty, off removes it, or
	// a text of its own. The universe switches it off for every diagram
	// (ProductionMark.yaml).
	Mark string `json:"mark,omitempty"`
}

// PageRef names a page template of the viewpoint's library and gives the
// values of its placeholders (title and date are filled from the diagram
// unless given here).
type PageRef struct {
	// Name is the page template ("dossier" for page:dossier); empty keeps
	// the viewpoint's page.
	Name string `json:"name,omitempty"`
	// Values fill the page's %placeholders% (project_code, project_name, …).
	Values map[string]string `json:"values,omitempty"`
}

// LogicalNode is one node in a LogicalDiagram.
//
// Satisfies the `archdiagram.ResolverNode` interface — the v2 resolver
// reads GetKind() for the template lookup and GetAttributes() for the
// placeholders and bindings.
type LogicalNode struct {
	// ID is a stable identifier for the node, unique within the
	// diagram. Used by edges (source/target), zones (members),
	// and as the visual cell's anchor.
	ID string `json:"id"`

	// Label is the human-readable display name.
	Label string `json:"label"`

	// Kind is an element kind of the viewpoint's notation
	// (e.g. "Application", "BusinessActor", "Class"). The renderer
	// looks up the per-kind style block + label spec via this key.
	Kind string `json:"kind"`

	// Attributes is free-form per-node data the template library's
	// resolver expressions read via paths like `attributes.domain`
	// or `attributes.markers`. The shape depends on the template library
	// in use — for an application-flux notation it carries `{domain, markers,
	// description, external_url, external_url_label}`; for UML
	// Class it carries `{attributes: [...], operations: [...]}`.
	Attributes map[string]any `json:"attributes,omitempty"`

	// Zone is the id of the zone this node belongs to (optional).
	// Must reference an existing zone in the diagram's zones[] list.
	Zone string `json:"zone,omitempty"`
}

// GetID returns the node's stable identifier.
func (n *LogicalNode) GetID() string { return n.ID }

// GetLabel returns the node's human-readable display name.
func (n *LogicalNode) GetLabel() string { return n.Label }

// GetKind returns the element kind.
func (n *LogicalNode) GetKind() string { return n.Kind }

// GetAttributes returns the free-form per-node attribute map.
func (n *LogicalNode) GetAttributes() map[string]any { return n.Attributes }

// LogicalEdge is a directed edge between two nodes.
type LogicalEdge struct {
	// ID is a stable identifier for the edge, unique within the diagram.
	ID string `json:"id"`

	// Source is the id of the originating node.
	Source string `json:"source"`

	// Target is the id of the destination node.
	Target string `json:"target"`

	// Kind is a relationship kind of the viewpoint's notation
	// (e.g. "Flow", "triggering", "association").
	Kind string `json:"kind"`

	// Label is an optional inline label for the edge.
	Label string `json:"label,omitempty"`

	// Attributes is free-form per-edge data the template library's
	// resolver expressions read via paths like
	// `attributes.fl_codes[].url` or `attributes.flow_type`.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// LogicalZone defines a named partition (swimlane / container) over
// a subset of nodes.
type LogicalZone struct {
	// ID is a stable identifier for the zone, unique within the diagram.
	// Referenced by LogicalNode.Zone.
	ID string `json:"id"`

	// Label is the human-readable display name of the zone.
	Label string `json:"label"`

	// Order is the partition ordering (left-to-right or top-to-bottom
	// depending on the layout's direction). Zero-based, monotonic
	// within a diagram.
	Order int `json:"order"`

	// Members is an optional explicit member list. When set, validator
	// cross-checks that every id exists in nodes[]. Either this field
	// OR each node's Zone field can be used to assign membership —
	// both is allowed.
	Members []string `json:"members,omitempty"`

	// Parent is the zone this one is drawn inside (a lane in its pool, a
	// subnet in its network). Optional; empty is a top-level zone.
	Parent string `json:"parent,omitempty"`

	// Kind picks the zone's look: the library's `zone:<kind>` template
	// (an AWS Region, a VPC, a public subnet). Optional.
	Kind string `json:"kind,omitempty"`
}
