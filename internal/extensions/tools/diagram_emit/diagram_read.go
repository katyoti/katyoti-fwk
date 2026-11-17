// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"context"
	"encoding/json"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// ReadInput is the diagram-read tool's input schema.
type ReadInput struct {
	XML string `json:"xml" jsonschema:"A drawio document: the XML of a .drawio file (plain or compressed)" required:"true"`
}

// ReadOutput is the diagram-read tool's output schema.
type ReadOutput struct {
	Diagram *drawioapi.LogicalDiagram `json:"diagram"            jsonschema:"The logical diagram the drawing shows, with the user's edits"`
	Pinned  []string                  `json:"pinned,omitempty"   jsonschema:"Ids of the nodes the user locked in place"`
	Foreign bool                      `json:"foreign"            jsonschema:"True when the drawing was not made by diagram-emit: kinds are unknown"`
	Report  []string                  `json:"report,omitempty"   jsonschema:"What the read could not carry over or had to guess: boxes added by hand, dropped edges"`
}

// NewDiagramReadTool constructs the diagram-read function tool.
func NewDiagramReadTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"diagram-read",
		readDescription,
		executeDiagramRead,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(func(json.RawMessage) string { return "diagram-read" }),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🖼️", Label: "Read diagram", Color: "#A6E22E"}),
	)
}

func executeDiagramRead(_ context.Context, in ReadInput) (ReadOutput, error) {
	r, err := drawiorender.Read([]byte(in.XML))
	if err != nil {
		return ReadOutput{}, err
	}
	return ReadOutput{Diagram: r.Diagram, Pinned: r.Pinned, Foreign: r.Foreign, Report: r.Report}, nil
}

const readDescription = `Read a drawio diagram back into its logical diagram.

Input: xml — the drawio XML of a diagram (read the .drawio file the user
kept).

Output: the logical diagram it draws — the one diagram-emit takes — with
what the user changed in draw.io: values edited in Edit Data, boxes added
by hand (kind "unknown"), locked nodes (pinned). A drawing not made by
diagram-emit reads into nodes, edges and zones of kind "unknown".

Use it before refining a diagram the user may have edited: change the
diagram it returns, then call diagram-emit with it and previous = the same
XML.`
