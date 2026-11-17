// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// --- pymupdf-text: raw text extraction via pymupdf CLI ---

// PymupdfTextExtractor extracts text from PDF using `python -m pymupdf gettext`.
type PymupdfTextExtractor struct{}

func (e *PymupdfTextExtractor) Name() string      { return "pymupdf-text" }
func (e *PymupdfTextExtractor) Formats() []string { return []string{"pdf"} }

func (e *PymupdfTextExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	contentFile := "content.md"
	outPath := filepath.Join(outputDir, contentFile)

	args := []string{"-m", "pymupdf", "gettext", "-mode", "layout", "-output", outPath}
	if limits != nil && limits.MaxPages > 0 {
		args = append(args, "-pages", fmt.Sprintf("1-%d", limits.MaxPages))
	}
	args = append(args, inputPath)

	cmd := exec.CommandContext(ctx, pythonBin(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pymupdf gettext: %w\n%s", err, string(output))
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		TotalParts: 1,
	}, nil
}

// --- pymupdf4llm: LLM-optimized markdown extraction ---

// Pymupdf4llmExtractor extracts PDF to LLM-friendly markdown via pymupdf4llm.
// Handles tables, multi-column layout, images extraction.
type Pymupdf4llmExtractor struct{}

func (e *Pymupdf4llmExtractor) Name() string      { return "pymupdf4llm-md" }
func (e *Pymupdf4llmExtractor) Formats() []string { return []string{"pdf"} }

func (e *Pymupdf4llmExtractor) Extract(ctx context.Context, inputPath string, outputDir string, _ map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	contentFile := "content.md"
	outPath := filepath.Join(outputDir, contentFile)

	// pymupdf4llm is a library, not a CLI — use a small inline Python script
	maxPages := 0
	if limits != nil && limits.MaxPages > 0 {
		maxPages = limits.MaxPages
	}

	script := fmt.Sprintf(`
import pymupdf4llm
max_pages = %d
pages = list(range(max_pages)) if max_pages > 0 else None
md = pymupdf4llm.to_markdown(%q, pages=pages)
with open(%q, "w", encoding="utf-8") as f:
    f.write(md)
`, maxPages, inputPath, outPath)

	cmd := exec.CommandContext(ctx, pythonBin(), "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pymupdf4llm: %w\n%s", err, string(output))
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		TotalParts: 1,
	}, nil
}

// --- pymupdf page render: PDF pages to PNG images ---

// PymupdfRenderVisionExtractor renders each PDF page as PNG via pymupdf,
// then sends to Vision API for markdown conversion.
type PymupdfRenderVisionExtractor struct {
	VisionFn func(ctx context.Context, imagePath string, prompt string) (string, error)
}

func (e *PymupdfRenderVisionExtractor) Name() string      { return "pymupdf-render-vision" }
func (e *PymupdfRenderVisionExtractor) Formats() []string { return []string{"pdf"} }

func (e *PymupdfRenderVisionExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	if e.VisionFn == nil {
		return nil, fmt.Errorf("pymupdf-render-vision requires VisionFn")
	}

	attachDir := filepath.Join(outputDir, "attachments")
	_ = os.MkdirAll(attachDir, 0o755)

	dpi := 150
	if config != nil {
		if d, ok := config["dpi"].(int); ok {
			dpi = d
		}
		if d, ok := config["dpi"].(float64); ok {
			dpi = int(d)
		}
	}

	maxPages := 0
	if limits != nil && limits.MaxPages > 0 {
		maxPages = limits.MaxPages
	}

	// Render all pages to PNG via pymupdf
	script := fmt.Sprintf(`
import pymupdf, os
doc = pymupdf.open(%q)
n = len(doc)
max_pages = %d
if max_pages > 0 and max_pages < n:
    n = max_pages
for i in range(n):
    page = doc[i]
    pix = page.get_pixmap(dpi=%d)
    out = os.path.join(%q, f"page_{i+1:03d}.png")
    pix.save(out)
print(n)
`, inputPath, maxPages, dpi, attachDir)

	cmd := exec.CommandContext(ctx, pythonBin(), "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pymupdf render: %w\n%s", err, string(output))
	}

	// Collect rendered PNGs
	pngs, _ := filepath.Glob(filepath.Join(attachDir, "page_*.png"))
	if len(pngs) == 0 {
		return nil, fmt.Errorf("pymupdf produced no PNG files")
	}

	prompt := "Convert this PDF page to clean Markdown. Preserve headings, tables, lists. Describe images and diagrams. Output only Markdown."
	if config != nil {
		if p, ok := config["prompt"].(string); ok {
			prompt = p
		}
	}

	var contentFiles []kbapi.ContentFile
	var attachments []kbapi.Attachment

	for i, pngPath := range pngs {
		imgName := filepath.Base(pngPath)
		imgRelPath := "attachments/" + imgName

		attachments = append(attachments, kbapi.Attachment{
			File: imgRelPath,
			Type: "page_render",
		})

		md, err := e.VisionFn(ctx, pngPath, prompt)
		if err != nil {
			md = fmt.Sprintf("![Page %d](%s)\n", i+1, imgRelPath)
		}

		contentFile := fmt.Sprintf("content_page_%03d.md", i+1)
		if err := os.WriteFile(filepath.Join(outputDir, contentFile), []byte(md), 0o644); err != nil {
			continue
		}

		contentFiles = append(contentFiles, kbapi.ContentFile{
			File:  contentFile,
			Part:  "page",
			Index: i + 1,
		})
		attachments[len(attachments)-1].ReferencedBy = contentFile
	}

	if len(contentFiles) == 0 {
		return nil, fmt.Errorf("no pages extracted from PDF")
	}

	return &kbapi.ExtractResult{
		ContentFiles: contentFiles,
		Attachments:  attachments,
		TotalParts:   len(pngs),
	}, nil
}

// --- Availability checks ---

// PymupdfAvailable checks if pymupdf is importable.
func PymupdfAvailable() bool {
	cmd := exec.Command(pythonBin(), "-c", "import pymupdf")
	return cmd.Run() == nil
}

// Pymupdf4llmAvailable checks if pymupdf4llm is importable.
func Pymupdf4llmAvailable() bool {
	cmd := exec.Command(pythonBin(), "-c", "import pymupdf4llm")
	return cmd.Run() == nil
}

// pythonBin is the Python pip installs the PDF packages with
// (publish.PythonCommand, the Windows Store's stub skipped); python3 while
// there is none.
func pythonBin() string {
	if p := publish.PythonCommand(); p != "" {
		return p
	}
	return "python3"
}

// Legacy aliases for poppler-based extractors (kept for backward compat in profiles)
type MupdfTextExtractor = PymupdfTextExtractor

// Unused poppler checks — kept as stubs
func PdftotextAvailable() bool { return false }
func PdftoppmAvailable() bool  { return false }
