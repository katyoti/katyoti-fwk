// SPDX-License-Identifier: Apache-2.0

// Package create_document implements the create-document agent tool.
// When an LLM agent wants to produce a text document (Markdown, HTML, mermaid,
// JSON, code…) that
// should open in its own tab in the GUI, it calls this tool with the full
// content. The tool fires a DocumentEvent on the EventBus, which the terminal
// translator picks up and converts into a DocumentContent message.
package create_document

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// CreateDocumentInput is the input schema for the create-document tool.
type CreateDocumentInput struct {
	Title     string `json:"title"     jsonschema:"Document title (used as the tab label)" required:"true"`
	MediaType string `json:"mediaType" jsonschema:"Media type of the content: text/markdown | text/html | text/vnd.mermaid | application/json | application/yaml | text/plain | text/x-<language>" required:"true"`
	Content   string `json:"content"   jsonschema:"Full document content"                  required:"true"`
}

// CreateDocumentOutput is the tool's result.
type CreateDocumentOutput struct {
	Success bool   `json:"success" jsonschema:"Whether the document was created"`
	Message string `json:"message" jsonschema:"Human-readable status message"`
}

// NewCreateDocumentTool creates the create-document tool.
func NewCreateDocumentTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"create-document",
		toolDescription,
		executeCreateDocument,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(describeCreateDocument),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📄", Label: "Document", Color: "#82AAFF", Template: "{{.title}}"}),
	)
}

func executeCreateDocument(ctx context.Context, in CreateDocumentInput) (CreateDocumentOutput, error) {
	// The stream plugin (stream_plugin.go) handles everything: progressive
	// DocumentDeltaEvent during streaming + final DocumentEvent on
	// tool_use_done. This tool function just validates the input and returns
	// a success message to the LLM. All UI rendering has already happened
	// by the time we get here.

	if in.Title == "" {
		return CreateDocumentOutput{}, fmt.Errorf("create-document: title is required")
	}
	if in.Content == "" {
		return CreateDocumentOutput{}, fmt.Errorf("create-document: content is required")
	}

	mediaType := render.BaseMediaType(in.MediaType)
	if !render.IsTextual(mediaType) {
		return CreateDocumentOutput{}, fmt.Errorf("create-document: %q is not a text media type (use text/markdown, text/html, text/vnd.mermaid, application/json, application/yaml, text/plain or text/x-<language>)", in.MediaType)
	}

	return CreateDocumentOutput{
		Success: true,
		Message: fmt.Sprintf("Document %q created (%s, %d bytes)", in.Title, mediaType, len(in.Content)),
	}, nil
}

func describeCreateDocument(raw json.RawMessage) string {
	var in CreateDocumentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "create-document <invalid>"
	}
	return fmt.Sprintf("create-document %q (%s, %d bytes)", in.Title, in.MediaType, len(in.Content))
}

const toolDescription = `Create a document (Markdown, HTML, a mermaid diagram, JSON, YAML or code) that opens as a new tab in the user's GUI.

Use this tool when you want to present structured content to the user — reports, analyses,
dashboards, diagrams, or any artifact that benefits from rich formatting beyond plain chat messages.

Parameters:
- title: Short document title, displayed as the tab label (e.g. "Analysis Report")
- mediaType: what the content is —
    "text/markdown" (recommended for text-heavy content),
    "text/html" (dashboards with inline styles, custom layouts),
    "text/vnd.mermaid" (one mermaid diagram),
    "application/json", "application/yaml", "text/plain",
    or "text/x-<language>" for source code (text/x-go, text/x-python, …)
- content: The full document content

Markdown supports GitHub-Flavored Markdown: headings, tables, code blocks with syntax
highlighting, lists, blockquotes, links — and ` + "```mermaid" + ` fenced blocks, drawn as diagrams.

HTML is rendered in a sandboxed iframe, so it's safe — inline CSS works, JavaScript is
permitted (use only when needed). Prefer self-contained HTML with inline styles.

Each call opens a new tab. The user can close tabs they no longer need.
`
