// SPDX-License-Identifier: Apache-2.0

// Package render is the public entry point for in-memory drawio
// rendering. It bridges callers outside the drawio extension (the
// `diagram-emit` agent tool, the `drawio render` CLI verb) and the internal
// archdiagram pipeline.
//
// Emit is a package-level function, NOT a method on DocService, so it
// does not auto-surface as a second sidecar tool beside the typed
// diagram-emit tool.
//
// The package sits beside `drawio/api` rather than inside it to break the
// import cycle: `drawio/internal/archdiagram` depends on `drawio/api`, so
// `drawio/api` cannot depend on archdiagram in turn.
package render

import (
	"context"
	"fmt"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/template"
)

// EmitResult carries the rendered XML alongside soft diagnostics.
// Hard infrastructure failures (catalog unreachable, render crash) come
// back as a Go error; Errors holds content errors the LLM should act on.
type EmitResult struct {
	XML      string   `json:"xml"                  desc:"Rendered diagram (drawio XML)"`
	Errors   []string `json:"errors,omitempty"     desc:"Validation errors (unknown viewpoint or kind, forbidden relationship, dangling edge)"`
	Warnings []string `json:"warnings,omitempty"   desc:"Lint warnings and template problems (orphan node, kind outside the viewpoint, a binding that failed)"`
	// Quality measures the drawing (nil when nothing was drawn).
	Quality *drawioapi.QualityReport `json:"quality,omitempty" desc:"How the drawing reads: overlaps, crossings, edges through nodes, bends"`
}

// Emit validates, lints, translates, lays out and draws a LogicalDiagram
// with its viewpoint's templates, and returns the drawio XML with the
// diagnostics.
//
//   - Go error: infrastructure failure (catalog unreachable, render crash)
//   - Errors: validation issues. Render still runs when they are
//     recoverable (an unknown kind draws with the library's default
//     template, a dangling edge is skipped).
//   - Warnings: lint issues and template problems.
func Emit(ctx context.Context, ld *drawioapi.LogicalDiagram, noLayout bool) (EmitResult, error) {
	if ld == nil {
		return EmitResult{Errors: []string{"diagram is required"}}, nil
	}

	// 1. Validate — surface content errors without bailing on render.
	vr, err := Validate(ctx, ld)
	if err != nil {
		return EmitResult{}, fmt.Errorf("validate: %w", err)
	}
	errs := make([]string, 0, len(vr.Errors))
	for _, e := range vr.Errors {
		errs = append(errs, fmt.Sprintf("%s: %s", e.Path, e.Message))
	}

	// 2. Lint — collect warnings.
	lr, err := Lint(ctx, ld)
	if err != nil {
		return EmitResult{Errors: errs}, fmt.Errorf("lint: %w", err)
	}
	warnings := make([]string, 0, len(lr.Warnings))
	for _, w := range lr.Warnings {
		warnings = append(warnings, fmt.Sprintf("%s: %s", w.Path, w.Message))
	}

	if blocksRender(vr) {
		return EmitResult{Errors: errs, Warnings: warnings}, nil
	}

	// 3. Translate — the pivot and the resolved drawing.
	diagram, dr, err := TranslateLogicalDiagram(ctx, ld)
	if err != nil {
		return EmitResult{Errors: errs, Warnings: warnings}, fmt.Errorf("translate: %w", err)
	}
	// The production mark: the one definition (docs), the diagram's own setting.
	if m, ok := docsapi.MarkOf(ctx, docsapi.DocMark{Mark: ld.Mark}); ok {
		diagram.Mark = m.Line()
	}
	lib, err := template.Parse(dr.Library)
	if err != nil {
		return EmitResult{Errors: append(errs, "templates: "+err.Error()), Warnings: warnings}, nil
	}

	// 4. Render.
	xml, problems, err := archdiagram.Render(ctx, diagram, dr, lib, noLayout)
	if err != nil {
		return EmitResult{Errors: errs, Warnings: warnings}, fmt.Errorf("render: %w", err)
	}
	out := EmitResult{XML: string(xml), Errors: errs, Warnings: append(warnings, problems...)}
	if q, err := Score(xml); err == nil {
		out.Quality = q
		if q.Overlaps > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("quality: %d overlapping node pair(s)", q.Overlaps))
		}
	}
	return out, nil
}

// blocksRender reports whether the validation issues block rendering
// outright: a schema violation or a viewpoint that does not resolve. Other
// content errors (unknown kind, dangling edge) do not — the renderer
// draws around them and the errors tell the LLM what to fix.
func blocksRender(vr *ValidationResult) bool {
	for _, e := range vr.Errors {
		switch e.Code {
		case "nil-diagram", "schema", "viewpoint-not-found":
			return true
		}
	}
	return false
}
