// SPDX-License-Identifier: Apache-2.0

package diagram

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DiagramRenderer renders diagrams to image bytes (output-agnostic).
// Implementations include KrokiRenderer (remote API) and DrawioRenderer (local CLI).
type DiagramRenderer interface {
	// MatchesLang returns true if this renderer handles a fenced code block
	// with the given language identifier.
	MatchesLang(lang string) bool

	// MatchesFile returns true if this renderer handles a file at the given path
	// (checked by extension, e.g. ".drawio", ".mmd").
	MatchesFile(path string) bool

	// RenderSource renders diagram source code to image bytes.
	RenderSource(lang, source, format string) ([]byte, error)

	// RenderFile renders a diagram file to image bytes.
	RenderFile(path string, format string) ([]byte, error)
}

// Dispatcher routes diagram requests to the first matching renderer.
type Dispatcher struct {
	renderers []DiagramRenderer
}

// NewDispatcher creates a Dispatcher from the given renderers.
// Renderers are tried in order; the first match wins.
func NewDispatcher(renderers ...DiagramRenderer) *Dispatcher {
	return &Dispatcher{renderers: renderers}
}

// FindLang returns the first renderer that matches the given language.
func (d *Dispatcher) FindLang(lang string) DiagramRenderer {
	for _, r := range d.renderers {
		if r.MatchesLang(lang) {
			return r
		}
	}
	return nil
}

// FindFile returns the first renderer that matches the given file path.
func (d *Dispatcher) FindFile(path string) DiagramRenderer {
	for _, r := range d.renderers {
		if r.MatchesFile(path) {
			return r
		}
	}
	return nil
}

// --- Preprocessing ---

// codeBlockRe matches fenced code blocks: ```lang ... ```
// It captures (1) the language tag and (2) the block content.
var codeBlockRe = regexp.MustCompile("(?m)^```(\\w[\\w-]*)\\s*\\n([\\s\\S]*?)^```\\s*$")

// imageRefRe matches markdown image references: ![alt](path)
// It captures (1) alt text and (2) the path.
var imageRefRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// PreprocessDiagrams scans markdown for diagram code blocks and image references,
// renders them via the dispatcher, and replaces them with image references.
// baseDir is the directory of the input markdown file (used to resolve relative image paths).
// outputDir is the directory of the output file (image paths are made relative to this).
// Non-diagram code blocks and non-diagram images are left untouched.
func PreprocessDiagrams(markdown []byte, dispatch *Dispatcher, imgDir string, outputFormat string, baseDir string, outputDir string) ([]byte, error) {
	isHTMLOutput := outputFormat == "html" || outputFormat == "html5"

	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image dir: %w", err)
	}

	var errs []string

	// 1. Replace fenced code blocks with rendered diagrams
	result := codeBlockRe.ReplaceAllFunc(markdown, func(match []byte) []byte {
		sub := codeBlockRe.FindSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		lang := string(sub[1])
		source := string(sub[2])

		renderer := dispatch.FindLang(lang)
		if renderer == nil {
			return match // not a diagram — keep as-is
		}

		// Determine image format
		imgFormat := effectiveFormat(lang, "svg", isHTMLOutput)

		data, err := renderer.RenderSource(lang, source, imgFormat)
		if err != nil {
			slog.Warn("diagram render failed, keeping code block", "lang", lang, "error", err)
			errs = append(errs, fmt.Sprintf("%s: %v", lang, err))
			return match
		}

		// Write to cache file
		path, err := writeCached(imgDir, lang, source, imgFormat, data)
		if err != nil {
			slog.Warn("diagram cache write failed", "lang", lang, "error", err)
			errs = append(errs, fmt.Sprintf("%s: %v", lang, err))
			return match
		}

		slog.Info("rendered diagram", "lang", lang, "path", path)
		relPath := relativeTo(path, outputDir)
		return []byte(fmt.Sprintf("![%s diagram](%s)", lang, relPath))
	})

	// 2. Replace image references to diagram files (e.g. ![arch](arch.drawio))
	// Process line-by-line to skip content inside fenced code blocks.
	result = replaceImageRefs(result, dispatch, imgDir, isHTMLOutput, baseDir, outputDir, &errs)

	if len(errs) > 0 {
		slog.Warn("some diagrams failed to render", "errors", strings.Join(errs, "; "))
	}

	return result, nil
}

// codeFenceStartRe matches the start of a fenced code block (``` or ~~~).
var codeFenceStartRe = regexp.MustCompile(`^(\x60{3,}|~{3,})`)

// replaceImageRefs processes image references line-by-line, skipping content
// inside fenced code blocks so example references aren't treated as real diagrams.
func replaceImageRefs(input []byte, dispatch *Dispatcher, imgDir string, isHTMLOutput bool, baseDir, outputDir string, errs *[]string) []byte {
	lines := strings.Split(string(input), "\n")
	var fenceMarker string // tracks the opening fence marker (empty = not in fence)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks
		if m := codeFenceStartRe.FindString(trimmed); m != "" {
			if fenceMarker == "" {
				fenceMarker = m[:1] // remember ` or ~
			} else if strings.HasPrefix(trimmed, fenceMarker[:1]) {
				fenceMarker = ""
			}
			continue
		}
		if fenceMarker != "" {
			continue // inside a code fence — skip
		}

		// Also skip indented code blocks (4+ spaces or tab)
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}

		lines[i] = string(imageRefRe.ReplaceAllFunc([]byte(line), func(match []byte) []byte {
			sub := imageRefRe.FindSubmatch(match)
			if len(sub) < 3 {
				return match
			}
			alt := string(sub[1])
			imgPath := string(sub[2])

			renderer := dispatch.FindFile(imgPath)
			if renderer == nil {
				return match // not a diagram file — keep as-is
			}

			// Resolve relative paths against the markdown file's directory
			resolvedPath := imgPath
			if !filepath.IsAbs(imgPath) && baseDir != "" {
				resolvedPath = filepath.Join(baseDir, imgPath)
			}

			imgFormat := "svg"
			if !isHTMLOutput {
				imgFormat = "png" // DOCX/PDF prefer PNG for local renders
			}

			// A diagram that cannot be drawn is named in the text: left as an
			// image reference to its source, it makes Pandoc fail the whole
			// document (a pptx refuses a missing image, no format embeds a
			// diagram source).
			data, err := renderer.RenderFile(resolvedPath, imgFormat)
			if err != nil {
				slog.Warn("diagram file render failed, naming it in the text", "path", imgPath, "error", err)
				*errs = append(*errs, fmt.Sprintf("%s: %v", imgPath, err))
				return notRendered(alt, imgPath)
			}

			// Write rendered image
			baseName := strings.TrimSuffix(filepath.Base(imgPath), filepath.Ext(imgPath))
			outPath := filepath.Join(imgDir, baseName+"."+imgFormat)
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				slog.Warn("write rendered image failed", "path", outPath, "error", err)
				*errs = append(*errs, fmt.Sprintf("%s: %v", imgPath, err))
				return notRendered(alt, imgPath)
			}

			slog.Info("rendered diagram file", "path", imgPath, "output", outPath)
			relPath := relativeTo(outPath, outputDir)
			if alt == "" {
				alt = baseName + " diagram"
			}
			return []byte(fmt.Sprintf("![%s](%s)", alt, relPath))
		}))
	}
	return []byte(strings.Join(lines, "\n"))
}

// notRendered is the text standing for a diagram file that could not be
// drawn: its caption and its file, in emphasis.
func notRendered(alt, imgPath string) []byte {
	label := strings.TrimSpace(alt)
	if label == "" {
		label = filepath.Base(imgPath)
	}
	return []byte(fmt.Sprintf("*%s — diagram not rendered: %s*", label, imgPath))
}

// relativeTo computes a path relative to refDir with forward slashes.
// If refDir is empty, falls back to CWD-relative.
func relativeTo(path, refDir string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if refDir == "" {
		refDir, _ = os.Getwd()
	}
	absRef, err := filepath.Abs(refDir)
	if err != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(absRef, abs)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// effectiveFormat returns the image format to use for a given diagram language.
func effectiveFormat(lang, defaultFmt string, isHTML bool) string {
	if isHTML {
		return "svg"
	}
	// Non-HTML: use PNG for foreignObject types
	resolved := ResolveDiagramType(lang)
	if ForeignObjectTypes[resolved] {
		return "png"
	}
	return defaultFmt
}

// writeCached writes rendered diagram data to a content-addressed cache file.
// Returns the path to the file.
func writeCached(dir, lang, source, format string, data []byte) (string, error) {
	filename := CacheKey(lang, source, format)
	path := filepath.Join(dir, filename)

	// Skip write if already cached
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write cache: %w", err)
	}
	return path, nil
}
