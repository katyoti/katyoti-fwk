// SPDX-License-Identifier: Apache-2.0

// Package core holds the DocService implementation and config.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/docxtpl"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/mdrender"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/pandoc"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
)

// docServicesImpl implements api.DocService.
type docServicesImpl struct {
	settings DocServiceConfig
}

// NewDocService creates a DocService from config. Used by DI registration.
func NewDocService(_ context.Context, cfg *DocServiceConfig) (api.DocService, error) {
	return &docServicesImpl{settings: *cfg}, nil
}

// LayoutDiagram runs the elkjs layout engine over an ELK graph and returns
// the laid-out graph. Thin typed wrapper over the drawio engine's ELK
// runner (drawiorender.LayoutELKGraph), exposed as a callable method.
func (d *docServicesImpl) LayoutDiagram(ctx context.Context, in api.LayoutDiagramInput) (api.LayoutDiagramOutput, error) {
	if strings.TrimSpace(in.Graph) == "" {
		return api.LayoutDiagramOutput{}, fmt.Errorf("layout-diagram: graph is required")
	}
	out, err := drawiorender.LayoutELKGraph(ctx, []byte(in.Graph))
	if err != nil {
		return api.LayoutDiagramOutput{}, err
	}
	return api.LayoutDiagramOutput{Graph: string(out)}, nil
}

func (d *docServicesImpl) Render(ctx context.Context, in api.RenderInput) (api.RenderOutput, error) {
	executor := pandoc.NewPandocExecutor()

	kroki := diagram.NewKrokiRenderer(d.settings.KrokiURL)
	executor.Diagrams = pandoc.BuildDiagramDispatcher(kroki)

	output := in.Output
	format := in.Format
	if format == "" && output != "" {
		switch strings.ToLower(filepath.Ext(output)) {
		case ".pdf":
			format = "pdf"
		case ".docx":
			format = "docx"
		case ".pptx":
			format = "pptx"
		case ".html", ".htm":
			format = "html"
		default:
			format = "pdf"
		}
	} else if format == "" {
		format = "pdf"
	}

	step(ctx, "Preparing the template", 0, renderSteps)
	template, cleanup, err := resolveTemplate(ctx, in.Template, format)
	if err != nil {
		return api.RenderOutput{}, err
	}
	defer cleanup()

	opts := pandoc.RenderOpts{
		InputPath:    in.Input,
		BaseDir:      in.BaseDir,
		OutputPath:   output,
		OutputFormat: format,
		Template:     template,
		ReferenceDoc: in.ReferenceDoc,
		FilterPath:   in.Filter,
		ExtraArgs:    in.ExtraArgs,
		MarkLine:     markLineOf(ctx, in.Input),
	}

	step(ctx, "Converting to "+format+" with Pandoc", 1, renderSteps)
	if err := executor.Render(opts); err != nil {
		return api.RenderOutput{}, err
	}
	step(ctx, "Written", renderSteps, renderSteps)

	return api.RenderOutput{OutputPath: output, Format: format}, nil
}

// The steps an export reports: Render and Diagram are long
// methods, and each step goes out through progress.Emit as a panel.Progress -
// into the panel of the method run that called it (panel.Lend), or nowhere
// when no run lent one.
const (
	renderSteps  = 2 // the template, the conversion
	diagramSteps = 2 // the rendering, the file written
)

// step reports how far an export is.
func step(ctx context.Context, label string, current, total int64) {
	progress.Emit(ctx, panel.Progress{Label: label, Current: current, Total: total})
}

func (d *docServicesImpl) Diagram(ctx context.Context, in api.DiagramInput) (api.DiagramOutput, error) {
	format := in.Format
	if format == "" {
		format = "svg"
	}

	kroki := diagram.NewKrokiRenderer(d.settings.KrokiURL)
	drawioRenderer := diagram.NewDrawioRenderer()
	dispatcher := diagram.NewDispatcher(drawioRenderer, kroki)

	output := in.Output
	if output == "" {
		base := strings.TrimSuffix(filepath.Base(in.Source), filepath.Ext(in.Source))
		output = filepath.Join(filepath.Dir(in.Source), base+"."+format)
	}

	diagramType := in.Type

	if renderer := dispatcher.FindFile(in.Source); renderer != nil {
		typeName := diagramType
		if typeName == "" {
			typeName = diagram.DetectDiagramType(in.Source)
			if typeName == "" {
				typeName = "drawio"
			}
		}

		step(ctx, "Rendering the "+typeName+" diagram to "+format, 0, diagramSteps)
		data, err := renderer.RenderFile(in.Source, format)
		if err != nil {
			return api.DiagramOutput{}, err
		}

		step(ctx, "Writing the image", 1, diagramSteps)
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return api.DiagramOutput{}, fmt.Errorf("write output: %w", err)
		}
		step(ctx, "Written", diagramSteps, diagramSteps)

		return api.DiagramOutput{OutputPath: output, DiagramType: typeName}, nil
	}

	if diagramType == "" {
		diagramType = diagram.DetectDiagramType(in.Source)
		if diagramType == "" {
			return api.DiagramOutput{}, fmt.Errorf("cannot detect diagram type from %q — specify it with --type", in.Source)
		}
	}

	resolved := diagram.ResolveDiagramType(diagramType)
	if resolved == "" {
		return api.DiagramOutput{}, fmt.Errorf("unsupported diagram type: %q", diagramType)
	}

	source, err := os.ReadFile(in.Source)
	if err != nil {
		return api.DiagramOutput{}, fmt.Errorf("read source: %w", err)
	}

	step(ctx, "Rendering the "+resolved+" diagram to "+format, 0, diagramSteps)
	result, err := kroki.Render(diagram.RenderRequest{
		Type:   resolved,
		Source: string(source),
		Format: format,
	})
	if err != nil {
		return api.DiagramOutput{}, err
	}

	step(ctx, "Writing the image", 1, diagramSteps)
	if err := os.WriteFile(output, result.Data, 0o644); err != nil {
		return api.DiagramOutput{}, fmt.Errorf("write output: %w", err)
	}
	step(ctx, "Written", diagramSteps, diagramSteps)

	return api.DiagramOutput{OutputPath: output, DiagramType: resolved}, nil
}

func (d *docServicesImpl) RenderTemplate(ctx context.Context, in api.RenderTemplateInput) (api.RenderTemplateOutput, error) {
	// Load data.
	data, err := loadTemplateData(in.Data, in.DataFile)
	if err != nil {
		return api.RenderTemplateOutput{}, err
	}

	// If DataFile is .md → parse frontmatter as vars, render body via Pandoc at {{content}}.
	if mdrender.IsMarkdownFile(in.DataFile) {
		if err := mdrender.LoadMarkdownData(in.DataFile, in.Template, data); err != nil {
			return api.RenderTemplateOutput{}, fmt.Errorf("load markdown content: %w", err)
		}
	}

	// Detect template type.
	tplType := in.Type
	if tplType == "" {
		tplType = detectTemplateType(in.Template)
	}
	if tplType == "" {
		return api.RenderTemplateOutput{}, fmt.Errorf("cannot detect template type from %q — specify it with --type", in.Template)
	}

	// Generate output path.
	output := in.Output
	if output == "" {
		base := strings.TrimSuffix(filepath.Base(in.Template), filepath.Ext(in.Template))
		output = filepath.Join(filepath.Dir(in.Template), base+"-rendered.docx")
	} else {
		// Validate: output extension must match template type to avoid
		// writing binary content (docx) to a text-extension path (.md, .txt).
		outExt := strings.ToLower(filepath.Ext(output))
		if tplType == "docx" && outExt != ".docx" && outExt != "" {
			// Auto-correct: replace wrong extension with .docx
			output = strings.TrimSuffix(output, filepath.Ext(output)) + ".docx"
		}
	}

	// Extract numbering data before rendering (injected by LoadMarkdownData).
	var numbering []byte
	if nb, ok := data["__numbering"].([]byte); ok {
		numbering = nb
		delete(data, "__numbering")
	}

	// The production mark: the document's own settings are its data (a
	// markdown data file's frontmatter); a .docx template places
	// {{ production_mark }} where it wants it.
	markLine := ""
	if m, ok := api.MarkOf(ctx, api.DocMarkOf(data)); ok {
		markLine = m.Line()
	}
	data["production_mark"] = markLine

	switch tplType {
	case "docx":
		if err := docxtpl.Render(in.Template, data, output); err != nil {
			return api.RenderTemplateOutput{}, fmt.Errorf("render docx template: %w", err)
		}
	case "md":
		if err := mdrender.RenderMarkdownTemplate(in.Template, data, output, markLine); err != nil {
			return api.RenderTemplateOutput{}, fmt.Errorf("render md template: %w", err)
		}
	default:
		return api.RenderTemplateOutput{}, fmt.Errorf("unsupported template type: %q", tplType)
	}

	// Merge Pandoc numbering definitions into the output (for bullet/number lists).
	if numbering != nil {
		if err := docxtpl.MergeNumbering(output, numbering); err != nil {
			return api.RenderTemplateOutput{}, fmt.Errorf("merge numbering: %w", err)
		}
	}

	return api.RenderTemplateOutput{OutputPath: output, Type: tplType}, nil
}

// --- Helpers ---

// markLineOf is the production mark's line for a markdown document, read
// with its frontmatter's own settings; empty when it carries none.
func markLineOf(ctx context.Context, input string) string {
	var meta map[string]any
	if raw, err := os.ReadFile(input); err == nil {
		meta, _, _ = mdrender.ParseFrontmatter(raw)
	}
	m, ok := api.MarkOf(ctx, api.DocMarkOf(meta))
	if !ok {
		return ""
	}
	return m.Line()
}

func detectTemplateType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		return "docx"
	case ".md", ".mdx", ".markdown":
		return "md"
	default:
		return ""
	}
}

func loadTemplateData(inline map[string]any, dataFile string) (map[string]any, error) {
	if len(inline) > 0 {
		return inline, nil
	}
	if dataFile == "" {
		return map[string]any{}, nil
	}
	// If markdown, frontmatter loading is handled separately.
	if mdrender.IsMarkdownFile(dataFile) {
		return map[string]any{}, nil
	}

	raw, err := os.ReadFile(dataFile)
	if err != nil {
		return nil, fmt.Errorf("read data file: %w", err)
	}

	var data map[string]any
	switch strings.ToLower(filepath.Ext(dataFile)) {
	case ".json":
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	default:
		if err := json.Unmarshal(raw, &data); err != nil {
			if err2 := yaml.Unmarshal(raw, &data); err2 != nil {
				return nil, fmt.Errorf("cannot parse %q as JSON or YAML", dataFile)
			}
		}
	}
	return data, nil
}
