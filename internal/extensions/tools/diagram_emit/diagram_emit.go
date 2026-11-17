// SPDX-License-Identifier: Apache-2.0

// Package diagram_emit implements the diagram-emit agent tool — exposes
// the drawio engine as an LLM-callable typed tool.
//
// The tool is a thin adapter: the input is a LogicalDiagram (the model:
// typed nodes, edges and zones plus the viewpoint URI), the output is the
// rendered drawio XML plus the validation errors and lint warnings.
// Rendering happens in `drawiorender.Emit`: validate → lint → translate →
// layout → template instantiation.
//
//   - Streaming: this tool emits ONE final value at tool completion.
//   - Hard boundary: typed input only — never raw mxGraph XML.
package diagram_emit

import (
	"context"
	"encoding/json"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Input is the diagram-emit tool's input schema. The diagram is the
// model; no_layout is a renderer option, kept beside it so the model
// stays a pure statement of what the diagram says.
type Input struct {
	Diagram  *drawioapi.LogicalDiagram `json:"diagram"             jsonschema:"The logical diagram: viewpoint URI + title + nodes [{id label kind attributes zone}] + edges [{id source target kind label attributes}] + zones + layoutHints" required:"true"`
	NoLayout bool                      `json:"no_layout,omitempty" jsonschema:"Skip ELK layout. Default false."`
	Previous string                    `json:"previous,omitempty"  jsonschema:"The drawio XML of the previous version as the user left it. Nothing it shows moves and its locks stay; new nodes are placed in their zone or beside a neighbour."`
}

// Output is the diagram-emit tool's output schema.
type Output struct {
	XML      string                   `json:"xml"                 jsonschema:"Rendered diagram (drawio XML)"`
	Errors   []string                 `json:"errors,omitempty"    jsonschema:"Validation errors: unknown viewpoint or kind, a relationship the notation forbids, a dangling edge"`
	Warnings []string                 `json:"warnings,omitempty"  jsonschema:"Lint warnings and template problems: orphan node, empty zone, long label, a kind the viewpoint does not show"`
	Quality  *drawioapi.QualityReport `json:"quality,omitempty" jsonschema:"How the drawing reads: overlaps (must be 0), crossings, edges through nodes, bends, extent"`
}

// NewDiagramEmitTool constructs the function tool.
func NewDiagramEmitTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"diagram-emit",
		toolDescription,
		executeDiagramEmit,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(describeDiagramEmit),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{
			Icon:     "🖼️",
			Label:    "Diagram",
			Color:    "#A6E22E",
			Template: "{{.diagram.viewpoint}}",
		}),
	)
}

func executeDiagramEmit(ctx context.Context, in Input) (Output, error) {
	// The stream plugin drew this call while it streamed: return that
	// drawing, the one the user sees in the tab.
	result, ok := recall(in)
	if !ok {
		var err error
		if result, err = draw(ctx, in); err != nil {
			return Output{}, err
		}
	}
	return Output{
		XML:      result.XML,
		Errors:   result.Errors,
		Warnings: result.Warnings,
		Quality:  result.Quality,
	}, nil
}

func describeDiagramEmit(raw json.RawMessage) string {
	var in Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return "diagram-emit <invalid>"
	}
	if in.Diagram == nil || in.Diagram.Viewpoint == "" {
		return "diagram-emit"
	}
	return "diagram-emit " + in.Diagram.Viewpoint
}

const toolDescription = `Render an editable drawio diagram from a logical diagram.

Input:
  - diagram: the logical diagram.
      {
        "viewpoint": "ktw-docs:/viewpoint/archimate3-layered",
        "title": "...",
        "nodes": [{"id":"n1","label":"Billing","kind":"Application","attributes":{"domain":"reference"},"zone":"z1"}],
        "edges": [{"id":"e1","source":"n1","target":"n2","kind":"Flow","attributes":{"interface_codes":[{"code":"IF-7349","url":"..."}]}}],
        "zones": [{"id":"z1","label":"Production","order":1}],   (a zone may name its "parent" zone and a "kind")
        "layoutHints": {"preset":"archi-layered"}
      }
    viewpoint is the type of drawing, a catalog URI. Built-in choices:
      ktw-docs:/viewpoint/archimate3-layered                  — ArchiMate, every element and relationship
      ktw-docs:/viewpoint/archimate3-application-cooperation  — ArchiMate applications, services, data
      ktw-docs:/viewpoint/archimate3-motivation               — ArchiMate stakeholders, drivers, goals, requirements
      ktw-docs:/viewpoint/archimate3-strategy                 — ArchiMate capabilities, resources, value streams
      ktw-docs:/viewpoint/archimate3-technology               — ArchiMate infrastructure and physical
      ktw-docs:/viewpoint/archimate3-implementation-migration — ArchiMate work packages, deliverables, plateaus
      ktw-docs:/viewpoint/c4-context                          — C4 people and software systems
      ktw-docs:/viewpoint/c4-container                        — C4 containers in a system boundary (a zone)
      ktw-docs:/viewpoint/c4-component                        — C4 components in a container boundary (a zone)
      ktw-docs:/viewpoint/bpmn2-process                       — BPMN process; a zone is a pool, a zone with a parent a lane
      ktw-docs:/viewpoint/aws-architecture                    — AWS services; a zone of kind Region, VPC, PublicSubnet… is that group
      ktw-docs:/viewpoint/uml2-class                          — UML classes, interfaces, packages
      ktw-docs:/viewpoint/uml2-overview                       — every UML 2.5 element kind
    Other sidecars publish their own viewpoints: ktw-docs drawio viewpoints lists them.
    A node's kind is an element kind of the viewpoint's notation, an edge's
    kind a relationship kind (list them: ktw-docs drawio notations <notation
    URI>). A notation that declares rules refuses a relationship its language
    never allows (ArchiMate does). attributes are free-form; the viewpoint's templates read
    them (e.g. domain → colour, external_url → link).
  - no_layout: optional — skip ELK auto-layout.
  - previous: optional — the drawio XML of the version the user has (read
    the file they kept). Pass it whenever you re-emit a diagram: nothing the
    user has seen moves, their locks stay, new nodes are placed in their zone
    or beside a neighbour. (To tidy the whole drawing, the user runs
    'ktw-docs drawio layout'.) Build the diagram from diagram-read(previous) so
    the user's own edits (values changed in Edit Data, boxes added by hand)
    are kept.

Output:
  - xml: the rendered drawio XML, paste-ready into a .drawio file.
  - errors: validation errors to fix (unknown viewpoint or kind, a relationship
    the notation forbids, a dangling edge).
  - warnings: lint issues and template problems.
  - quality: how the drawing reads — overlaps (must be 0), crossings, edges
    through nodes, bends, width and height.

In the GUI the call opens a draw.io tab as soon as the first node is written
and draws the diagram as it grows (write viewpoint and title first, then
zones, nodes, edges); do not open it again with another tool.

Boundary: this tool accepts ONLY the typed diagram above, never raw mxGraph XML.`
