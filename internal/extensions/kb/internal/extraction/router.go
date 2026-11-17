// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// Router selects the right extractor for a file based on the profile.
type Router struct {
	extractors map[string]kbapi.Extractor // name → extractor
	profiles   map[string][]string        // profile name → ordered list of extractor names
}

// NewRouter creates a Router with all available extractors and default profiles.
func NewRouter() *Router {
	r := &Router{
		extractors: make(map[string]kbapi.Extractor),
		profiles:   make(map[string][]string),
	}

	// Register built-in extractors (always available)
	r.Register(&PassthroughExtractor{})
	r.Register(&ExcelExtractor{})
	r.Register(&ItemDataExtractor{}) // an item's data rendered as text

	// PDF extraction — pymupdf4llm (best), pymupdf-text (fallback)
	if Pymupdf4llmAvailable() {
		r.Register(&Pymupdf4llmExtractor{})
	}
	if PymupdfAvailable() {
		r.Register(&PymupdfTextExtractor{})
	}

	// Register extractors that depend on external tools
	if PandocAvailable() {
		r.Register(&PandocExtractor{})
	}
	if DoclingAvailable() {
		r.Register(&DoclingExtractor{})
	}
	if MarkerAvailable() {
		r.Register(&MarkerExtractor{})
	}
	if OfficeComAvailable() {
		r.Register(&OfficePdfExtractor{})
	}
	if AssemblyAIAvailable() {
		r.Register(&AssemblyAIExtractor{APIKey: os.Getenv("ASSEMBLY_AI_API_KEY")})
	}

	// Vision-based extractors (Anthropic first, OpenAI fallback)
	visionFn := NewVisionFn()
	anthropic := NewAnthropicVisionClient()
	openai := NewOpenAIVisionClient()
	if anthropic.Available() || openai.Available() {
		r.RegisterVisionExtractors(visionFn)
	}
	// Default profiles — ordered by preference, first matching format wins
	r.profiles["fast"] = []string{
		"office-pdf-extract",    // pptx → pdf (Office COM) → pymupdf4llm (best for slides)
		"pandoc-md",             // docx, pptx fallback, odt, rtf, epub
		"pymupdf4llm-md",        // pdf (LLM-optimized markdown, best quality, no AI)
		"pymupdf-text",          // pdf fallback (raw text)
		"excelize-md",           // xlsx, xls
		"assemblyai-transcribe", // audio, video
		"text-passthrough",      // md, txt, csv, json, xml, html
	}

	r.profiles["cost-effective"] = []string{
		"office-pdf-extract",    // pptx → pdf (Office COM) → pymupdf4llm
		"pandoc-md",             // docx, pptx fallback, odt, rtf, epub
		"docling-md",            // pdf (layout-aware, tables, figures)
		"pymupdf4llm-md",        // pdf fallback (LLM-optimized)
		"pymupdf-text",          // pdf fallback (raw text)
		"excelize-md",           // xlsx, xls
		"assemblyai-transcribe", // audio, video
		"text-passthrough",      // md, txt, csv, json, xml, html
	}

	r.profiles["agentic"] = []string{
		"office-pdf-vision",     // pptx → pdf (Office COM) → render + Vision AI per slide
		"office-pdf-extract",    // pptx fallback without vision
		"pandoc-md",             // docx, odt, rtf, epub
		"docling-vision",        // pdf (docling + AI for figures)
		"docling-md",            // pdf fallback without vision
		"pymupdf-render-vision", // pdf (pymupdf render + AI per page)
		"pymupdf4llm-md",        // pdf fallback (LLM-optimized, no AI)
		"pymupdf-text",          // pdf last resort
		"office-pdf-vision",     // pptx (render slides + AI)
		"excelize-md",           // xlsx, xls
		"assemblyai-transcribe", // audio, video
		"vision-ocr",            // images (AI describes)
		"text-passthrough",      // md, txt, csv, json, xml, html
	}

	r.profiles["agentic-plus"] = r.profiles["agentic"] // same extractors, config differs (verify: true)

	return r
}

// RegisterVisionExtractors adds extractors that require a Vision API function.
// Only registers extractors whose underlying tools are also available.
func (r *Router) RegisterVisionExtractors(visionFn func(ctx context.Context, imagePath string, prompt string) (string, error)) {
	r.Register(&VisionOCRExtractor{VisionFn: visionFn})
	if PymupdfAvailable() {
		r.Register(&PymupdfRenderVisionExtractor{VisionFn: visionFn})
	}
	if DoclingAvailable() {
		r.Register(&DoclingVisionExtractor{VisionFn: visionFn})
	}
	if OfficeComAvailable() {
		r.Register(&OfficePdfVisionExtractor{VisionFn: visionFn})
	}
}

// Register adds an extractor to the router.
func (r *Router) Register(ext kbapi.Extractor) {
	r.extractors[ext.Name()] = ext
}

// Resolve finds the first extractor in the profile that handles the given file extension.
func (r *Router) Resolve(profileName, filePath string) (kbapi.Extractor, error) {
	// An item's data file is read as its data rendered as text, in every
	// profile.
	if IsItemData(filePath) {
		if e, ok := r.extractors[ItemDataExtractorName]; ok {
			return e, nil
		}
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))
	if ext == "" {
		return nil, fmt.Errorf("file has no extension: %s", filePath)
	}

	profile, ok := r.profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", profileName)
	}

	for _, extractorName := range profile {
		extractor, exists := r.extractors[extractorName]
		if !exists {
			continue // extractor not registered (tool missing)
		}
		for _, supported := range extractor.Formats() {
			if supported == ext {
				return extractor, nil
			}
		}
	}

	return nil, fmt.Errorf("no extractor in profile %q handles .%s files", profileName, ext)
}

// Extract runs the full extraction: resolve extractor + run it.
func (r *Router) Extract(ctx context.Context, profileName, inputPath, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (string, *kbapi.ExtractResult, error) {
	extractor, err := r.Resolve(profileName, inputPath)
	if err != nil {
		return "", nil, err
	}

	result, err := extractor.Extract(ctx, inputPath, outputDir, config, limits)
	if err != nil {
		return extractor.Name(), nil, err
	}

	return extractor.Name(), result, nil
}

// AvailableExtractors returns the names of all registered extractors.
func (r *Router) AvailableExtractors() []string {
	var names []string
	for name := range r.extractors {
		names = append(names, name)
	}
	return names
}
