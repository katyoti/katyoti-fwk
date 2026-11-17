// SPDX-License-Identifier: Apache-2.0

package pandoc

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
)

// PandocFilter processes a Pandoc JSON AST, replacing diagram code blocks
// with rendered images via Kroki.
type PandocFilter struct {
	Kroki     *diagram.KrokiRenderer
	OutputDir string // directory to write cached images
	Format    string // pandoc output format (html, typst, etc.)
	ImgFormat string // default image format (svg, png)
}

// RunFilter reads Pandoc JSON AST from r, processes it, and writes to w.
func (f *PandocFilter) RunFilter(r io.Reader, w io.Writer) error {
	var doc any
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return fmt.Errorf("decode pandoc AST: %w", err)
	}

	f.walkAST(doc)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

func (f *PandocFilter) walkAST(node any) {
	switch v := node.(type) {
	case map[string]any:
		if t, ok := v["t"].(string); ok && t == "CodeBlock" {
			// Don't modify in place during walk — handled by walkList
		}
		for _, val := range v {
			f.walkAST(val)
		}
	case []any:
		f.walkList(v)
	}
}

func (f *PandocFilter) walkList(list []any) {
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			f.walkAST(item)
			continue
		}

		t, _ := m["t"].(string)
		if t == "CodeBlock" {
			if replacement := f.processCodeBlock(m); replacement != nil {
				list[i] = replacement
				continue
			}
		}

		f.walkAST(item)
	}
}

func (f *PandocFilter) processCodeBlock(node map[string]any) map[string]any {
	c, ok := node["c"].([]any)
	if !ok || len(c) < 2 {
		return nil
	}

	attr, ok := c[0].([]any)
	if !ok || len(attr) < 3 {
		return nil
	}

	classes, ok := attr[1].([]any)
	if !ok || len(classes) == 0 {
		return nil
	}

	className, ok := classes[0].(string)
	if !ok {
		return nil
	}

	diagramType := diagram.ResolveDiagramType(className)
	if diagramType == "" {
		return nil
	}

	content, ok := c[1].(string)
	if !ok {
		return nil
	}

	imgFormat := f.effectiveImgFormat(diagramType)

	req := diagram.RenderRequest{
		Type:   diagramType,
		Source: content,
		Format: imgFormat,
	}

	path, err := f.Kroki.RenderCached(req, f.OutputDir)
	if err != nil {
		slog.Warn("failed to render diagram", "type", diagramType, "error", err)
		return nil
	}

	refPath := f.relativePath(path)
	slog.Info("rendered diagram", "type", diagramType, "path", refPath)

	ident, _ := attr[0].(string)
	keyvals, _ := attr[2].([]any)
	if keyvals == nil {
		keyvals = []any{}
	}

	image := map[string]any{
		"t": "Image",
		"c": []any{
			[]any{ident, []any{}, keyvals},
			[]any{},
			[]any{refPath, "fig:" + diagramType},
		},
	}

	para := map[string]any{
		"t": "Para",
		"c": []any{image},
	}

	return para
}

func (f *PandocFilter) effectiveImgFormat(diagramType string) string {
	format := f.ImgFormat
	if format == "" {
		format = "svg"
	}

	switch f.Format {
	case "html", "html5", "s5", "slidy":
		format = "svg"
	}

	if f.Format == "typst" && diagram.ForeignObjectTypes[diagramType] {
		format = "png"
	}

	return format
}

// OutputDirFromInputFile derives an image output directory from the input file path.
func OutputDirFromInputFile(inputFile string) string {
	dir := filepath.Dir(inputFile)
	base := filepath.Base(inputFile)
	for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
		base = base[:len(base)-len(ext)]
	}
	return filepath.Join(dir, base+"-kroki-images")
}

func (f *PandocFilter) relativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return strings.ReplaceAll(rel, "\\", "/")
}
