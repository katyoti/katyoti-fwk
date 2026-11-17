// SPDX-License-Identifier: Apache-2.0

// Package api defines the public types for the docs extension.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// DocService is the contract for the docs service.
// It provides document rendering and diagram generation.
type DocService interface {
	Render(ctx context.Context, in RenderInput) (RenderOutput, error)
	Diagram(ctx context.Context, in DiagramInput) (DiagramOutput, error)
	RenderTemplate(ctx context.Context, in RenderTemplateInput) (RenderTemplateOutput, error)
	LayoutDiagram(ctx context.Context, in LayoutDiagramInput) (LayoutDiagramOutput, error)
}

// DocServiceDef describes the DocService interface for auto-generation.
var DocServiceDef = meta.ServiceDef{
	Name:    "docs",
	Short:   "Document and diagram transformation service",
	Long:    `Renders Markdown documents to PDF, DOCX, PPTX, and HTML via Pandoc, and diagrams (Mermaid, PlantUML, Graphviz, DrawIO) to SVG or PNG via Kroki. Supports templates, reference documents, and custom Pandoc filters.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Render": long(meta.MethodLong(
			"Render a Markdown document to PDF, DOCX, PPTX, or HTML via Pandoc",
			`Converts a Markdown file to the specified output format using Pandoc.
Diagrams embedded as code blocks (mermaid, plantuml, dot) are automatically
rendered to images via Kroki before conversion. Supports templates,
reference documents for DOCX/PPTX styling, and custom Pandoc filters.`,
		)),
		"Diagram": long(meta.MethodLong(
			"Render an existing diagram source file to a static image (SVG/PNG/PDF) via Kroki",
			`Renders a diagram source file (Mermaid, PlantUML, Graphviz, DrawIO, D2, Ditaa, etc.)
to a static image format. The diagram type is auto-detected from the file extension.
Uses Kroki for server-side rendering or local DrawIO CLI when available.

When NOT to use this: if you want to AUTHOR a new architecture diagram from
structured data (logical nodes + edges → editable drawio XML), use the
diagram-emit agent tool (or the ktw-docs drawio CLI) instead: it drives the
drawio engine (ArchiMate / C4 / BPMN / AWS / UML) and produces editable drawio XML,
NOT a static image. Diagram (this method) is for rendering pre-existing source
files into static images you can embed in a document.`,
		)),
		"RenderTemplate": meta.MethodLong(
			"Render data into a document template (DOCX or Markdown)",
			`Renders data into a template using Jinja2 syntax (gonja engine).
Supported templates: .docx (Office template), .md (Markdown → DOCX via Pandoc).
Data from JSON/YAML file or inline. When DataFile is .md, frontmatter provides
variables and body is injected at {{content}} via Pandoc + Lua filter.
Supports <!--ktw --> macros for table/paragraph style control.`,
		),
		"LayoutDiagram": meta.MethodLong(
			"Compute node/edge positions for an ELK graph via the elkjs layout engine",
			`Runs the elkjs layout engine over an ELK graph (the Eclipse Layout Kernel
JSON format) and returns the same graph annotated with computed x/y/width/
height. This is a thin typed wrapper over the embedded elk-layout script —
it is the low-level layout step the drawio engine uses internally,
exposed as a callable method. Requires bun on PATH (elkjs is installed into
a scratch dir on first run).`,
		),
	},
}

// long declares a document export method long: run from
// the action surface it runs as a method run owning a display panel, and the
// method reports its steps into it through progress.Emit (a panel.Progress
// each) - an export no longer waits on the bus's 5 s fallback with a busy
// badge only. LayoutDiagram stays synchronous: its callers (the drawio
// engine, the portfolio map) need its graph at once.
func long(d meta.MethodDef) meta.MethodDef {
	d.LongRunning = true
	return d
}

// LayoutDiagramInput is the input for the LayoutDiagram method: an ELK graph
// in the Eclipse Layout Kernel JSON format.
type LayoutDiagramInput struct {
	Graph string `json:"graph" cli:"arg" desc:"ELK graph JSON" required:"true" jsonschema:"An ELK graph document (Eclipse Layout Kernel JSON: id, children[], edges[]) to lay out"`
}

// LayoutDiagramOutput is the laid-out ELK graph: the same structure with
// computed x/y/width/height on every node.
type LayoutDiagramOutput struct {
	Graph string `json:"graph" desc:"Laid-out ELK graph JSON (positions computed)"`
}

// RenderInput defines the input for document rendering.
type RenderInput struct {
	Input        string   `json:"input"                  cli:"arg"           desc:"Input Markdown file path"     required:"true" jsonschema:"Input Markdown file path"`
	Output       string   `json:"output"                 cli:"output,o"      desc:"Output file path"             required:"true" jsonschema:"Output file path"`
	Format       string   `json:"format,omitempty"       cli:"format,t"      desc:"Output format"                enum:"pdf,docx,pptx,html" jsonschema:"Output format: pdf, docx, pptx, html (inferred from output extension if omitted)"`
	Template     string   `json:"template,omitempty"     cli:"template"      desc:"Template name or path"        jsonschema:"Template name or path for styled output"`
	ReferenceDoc string   `json:"referenceDoc,omitempty" cli:"reference-doc" desc:"Reference doc for DOCX/PPTX"  jsonschema:"Reference document for DOCX/PPTX styling"`
	Filter       string   `json:"filter,omitempty"       cli:"filter"        desc:"Pandoc filter binary path"    jsonschema:"Pandoc filter binary path"`
	ExtraArgs    []string `json:"extraArgs,omitempty"    cli:"-"             jsonschema:"Extra Pandoc arguments"`
	BaseDir      string   `json:"baseDir,omitempty"      cli:"base-dir"      desc:"Folder relative references resolve against" jsonschema:"Folder the document's relative images and diagrams resolve against - the source file's folder when the input is a copy (default: the input's folder)"`
}

// RenderOutput is the result of document rendering.
type RenderOutput struct {
	OutputPath string `json:"outputPath" desc:"Path to rendered file"`
	Format     string `json:"format"     desc:"Output format used"`
}

// DiagramInput defines the input for diagram rendering.
type DiagramInput struct {
	Source string `json:"source"           cli:"arg"      desc:"Source file path" required:"true" jsonschema:"Diagram source file path (Mermaid, PlantUML, Graphviz, DrawIO, D2, Ditaa, ...)"`
	Output string `json:"output,omitempty" cli:"output,o" desc:"Output file path" jsonschema:"Output file path (auto-generated if omitted)"`
	Format string `json:"format,omitempty" cli:"format,f" desc:"Output format"    enum:"svg,png,jpg,pdf" default:"svg" jsonschema:"Output image format: svg, png, jpg, pdf"`
	Type   string `json:"type,omitempty"   cli:"type,T"   desc:"Diagram type"     jsonschema:"Diagram type (auto-detected from extension if omitted)"`
}

// DiagramOutput is the result of diagram rendering.
type DiagramOutput struct {
	OutputPath  string `json:"outputPath"  desc:"Path to rendered diagram"`
	DiagramType string `json:"diagramType" desc:"Diagram type used"`
}

// RenderTemplateInput defines the input for template rendering.
//
// Supported template types (auto-detected from extension):
//   - .docx — DOCX template with Jinja2 expressions (gonja engine)
//   - .md   — Markdown template with Jinja2 expressions → rendered to DOCX via Pandoc
//
// Data sources:
//   - Data (inline JSON map) or DataFile (.json, .yaml)
//   - When DataFile is .md: frontmatter provides variables, body provides {{content}}
//     (rendered via Pandoc + Lua filter for <!--ktw --> macros)
type RenderTemplateInput struct {
	Template string         `json:"template"          cli:"arg"          desc:"Template file path (.docx, .md)" required:"true" jsonschema:"Template file path (.docx or .md)"`
	Type     string         `json:"type,omitempty"    cli:"type,T"       desc:"Template type"       jsonschema:"Template type: docx, md (auto-detected from file extension if omitted)"`
	Data     map[string]any `json:"data,omitempty"                       desc:"Template variables"  jsonschema:"Template variables as inline JSON map"`
	DataFile string         `json:"dataFile,omitempty" cli:"data-file,d" desc:"Data file"           jsonschema:"Data file path: .json, .yaml, or .md (frontmatter = vars, body = content)"`
	Output   string         `json:"output,omitempty"  cli:"output,o"    desc:"Output file path"    jsonschema:"Output file path (auto-generated if omitted)"`
}

// RenderTemplateOutput is the result of template rendering.
type RenderTemplateOutput struct {
	OutputPath string `json:"outputPath" desc:"Path to rendered document"`
	Type       string `json:"type"       desc:"Template type used"`
}
