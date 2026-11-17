// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// OfficePdfExtractor converts PPTX/DOCX to PDF via Office COM (Windows),
// then extracts content using pymupdf4llm or pymupdf-text.
// Chain: PPTX → PDF (Office COM) → pymupdf4llm → markdown
type OfficePdfExtractor struct{}

func (e *OfficePdfExtractor) Name() string      { return "office-pdf-extract" }
func (e *OfficePdfExtractor) Formats() []string { return []string{"pptx", "ppt"} }

func (e *OfficePdfExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	// Step 1: Convert PPTX → PDF via Office COM
	pdfPath := filepath.Join(outputDir, "source.pdf")

	script := fmt.Sprintf(`
import sys, os
try:
    import win32com.client
except ImportError:
    print("ERROR: pywin32 not installed. Run: pip install pywin32", file=sys.stderr)
    sys.exit(1)

pptx_path = os.path.abspath(%q)
pdf_path = os.path.abspath(%q)

powerpoint = win32com.client.Dispatch("PowerPoint.Application")
try:
    presentation = powerpoint.Presentations.Open(pptx_path, WithWindow=False)
    presentation.SaveAs(pdf_path, FileFormat=32)  # 32 = ppSaveAsPDF
    presentation.Close()
finally:
    powerpoint.Quit()
print("OK")
`, inputPath, pdfPath)

	cmd := exec.CommandContext(ctx, pythonBin(), "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("office COM convert: %w\n%s", err, string(output))
	}

	defer os.Remove(pdfPath) // clean up intermediate PDF

	// Step 2: Extract from PDF via pymupdf4llm (best) or pymupdf-text (fallback)
	if Pymupdf4llmAvailable() {
		ext := &Pymupdf4llmExtractor{}
		return ext.Extract(ctx, pdfPath, outputDir, config, limits)
	}
	if PymupdfAvailable() {
		ext := &PymupdfTextExtractor{}
		return ext.Extract(ctx, pdfPath, outputDir, config, limits)
	}

	return nil, fmt.Errorf("pymupdf or pymupdf4llm required for PDF extraction after Office conversion")
}

// OfficePdfVisionExtractor converts PPTX to PDF via Office COM,
// then renders each page as image and sends to Vision API.
// Chain: PPTX → PDF (Office COM) → pymupdf render → Vision AI per page
type OfficePdfVisionExtractor struct {
	VisionFn func(ctx context.Context, imagePath string, prompt string) (string, error)
}

func (e *OfficePdfVisionExtractor) Name() string      { return "office-pdf-vision" }
func (e *OfficePdfVisionExtractor) Formats() []string { return []string{"pptx", "ppt"} }

func (e *OfficePdfVisionExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	if e.VisionFn == nil {
		return nil, fmt.Errorf("office-pdf-vision requires VisionFn")
	}

	// Step 1: Convert PPTX → PDF via Office COM
	pdfPath := filepath.Join(outputDir, "source.pdf")

	script := fmt.Sprintf(`
import sys, os
try:
    import win32com.client
except ImportError:
    print("ERROR: pywin32 not installed", file=sys.stderr)
    sys.exit(1)

pptx_path = os.path.abspath(%q)
pdf_path = os.path.abspath(%q)

powerpoint = win32com.client.Dispatch("PowerPoint.Application")
try:
    presentation = powerpoint.Presentations.Open(pptx_path, WithWindow=False)
    presentation.SaveAs(pdf_path, FileFormat=32)
    presentation.Close()
finally:
    powerpoint.Quit()
print("OK")
`, inputPath, pdfPath)

	cmd := exec.CommandContext(ctx, pythonBin(), "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("office COM convert: %w\n%s", err, string(output))
	}

	defer os.Remove(pdfPath)

	// Step 2: Render PDF pages + Vision AI
	renderExt := &PymupdfRenderVisionExtractor{VisionFn: e.VisionFn}
	return renderExt.Extract(ctx, pdfPath, outputDir, config, limits)
}

// OfficeComAvailable checks if Office COM automation is available (Windows + pywin32).
func OfficeComAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	cmd := exec.Command(pythonBin(), "-c", "import win32com.client; print('ok')")
	return cmd.Run() == nil
}
