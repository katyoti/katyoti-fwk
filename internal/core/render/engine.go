// SPDX-License-Identifier: Apache-2.0

package render

import "strings"

// EngineDecl is a display engine a sidecar contributes: the body of a
// `display-engine` contribution (embed/display-engines/<name>.yaml, published
// with manifestcontrib.ContributeKindResources and
// WithSiblingLayer("widget", ".html")). The GUI registers it on the same
// registry as its built-in engines; a built-in engine wins a tie.
//
// The widget is the contribution's `widget` layer: one self-contained HTML
// page (<name>.html beside the declaration), which the GUI opens through the
// contribution lane and loads in a sandboxed iframe — no URL, no port, the
// sidecar need not be serving anything. The page receives the document by
// postMessage — `{type: "value", value: <Document>, mode: "view"|"edit"}`, once
// loaded and again when the document or the mode changes — and in edit mode
// commits with `{type: "change", value: <content>}` (the host keeps it and
// leaves editing). For each format in `capabilities.export` the host may ask
// `{type: "export", id, mediaType}`; the page answers
// `{type: "exported", id, value: <Document>}` or `{type: "exported", id, error}`.
type EngineDecl struct {
	Label        string             `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=Name shown for the engine (the document badge)"`
	Accepts      []string           `json:"accepts" yaml:"accepts" jsonschema:"description=Media-type patterns the engine shows: exact (application/vnd.acme.graph+json) or with parameters or type/* (image/*)"`
	Capabilities EngineCapabilities `json:"capabilities,omitempty" yaml:"capabilities,omitempty" jsonschema:"description=What the engine can do beyond viewing"`
	Files        []EngineFile       `json:"files,omitempty" yaml:"files,omitempty" jsonschema:"description=File names that hold the engine's documents: a file ending with the suffix opens with that media type"`
}

// EngineCapabilities says what an engine does beyond viewing a document.
type EngineCapabilities struct {
	Stream bool           `json:"stream,omitempty" yaml:"stream,omitempty" jsonschema:"description=Shows the document while its producer is still writing it"`
	Edit   bool           `json:"edit,omitempty" yaml:"edit,omitempty" jsonschema:"description=Lets the user edit the document"`
	Save   bool           `json:"save,omitempty" yaml:"save,omitempty" jsonschema:"description=The document can be saved to a file"`
	Export []ExportFormat `json:"export,omitempty" yaml:"export,omitempty" jsonschema:"description=Formats the engine converts a document to (the Export menu); the widget answers an export request"`
}

// ExportFormat is one format an engine exports a document to.
type ExportFormat struct {
	MediaType string `json:"mediaType" yaml:"mediaType" jsonschema:"description=Media type of the exported document (application/pdf; image/svg+xml)"`
	Label     string `json:"label" yaml:"label" jsonschema:"description=Name shown in the Export menu (PDF; SVG)"`
}

// EngineFile types a file by its name for a contributed engine: a file whose
// name ends with Suffix (".landscape.json") is a document of MediaType. The
// GUI consults the contributed engines before its own table (render.Resolve),
// so a sidecar's files open with its engine.
type EngineFile struct {
	Suffix    string `json:"suffix" yaml:"suffix" jsonschema:"description=End of the file name (.landscape.json)"`
	MediaType string `json:"mediaType" yaml:"mediaType" jsonschema:"description=Media type of such a file"`
}

// MediaTypeForName returns the media type the engines' files declare for a
// file name, or "" — the longest matching suffix wins, case-insensitively.
func MediaTypeForName(engines []EngineDecl, name string) string {
	lower := strings.ToLower(name)
	best, mt := 0, ""
	for _, e := range engines {
		for _, f := range e.Files {
			suffix := strings.ToLower(f.Suffix)
			if suffix != "" && len(suffix) > best && strings.HasSuffix(lower, suffix) {
				best, mt = len(suffix), f.MediaType
			}
		}
	}
	return mt
}
