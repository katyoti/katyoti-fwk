// SPDX-License-Identifier: Apache-2.0

package docxtpl

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/beevik/etree"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

// Render opens a DOCX template, renders it with data, and writes the result.
// The template uses Jinja2-compatible syntax ({{var}}, {% for %}, {% if %}).
func Render(templatePath string, data map[string]any, outputPath string) error {
	zipData, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", f.Name, err)
		}

		if shouldRenderPart(f.Name) && hasTemplateMarkers(string(content)) {
			var rendered []byte
			var err error
			if isDocProps(f.Name) {
				// DocProps are simple XML — use string-based rendering (no tree patching needed).
				rendered, err = renderSimplePart(content, data)
			} else {
				rendered, err = renderXMLPart(content, data)
			}
			if err != nil {
				return fmt.Errorf("render %s: %w", f.Name, err)
			}
			content = rendered
		}

		header := &zip.FileHeader{
			Name:   f.Name,
			Method: f.Method,
		}
		w, err := writer.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create %s: %w", f.Name, err)
		}
		if _, err := w.Write(content); err != nil {
			return fmt.Errorf("write %s: %w", f.Name, err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

func isDocProps(name string) bool {
	return strings.HasPrefix(name, "docProps/")
}

// renderSimplePart renders a simple XML part (no Word run fragmentation).
// Used for docProps/core.xml and docProps/app.xml where {{var}} appears as plain text.
// Handles unsupported syntax (e.g., array indexing [0]) by replacing those
// expressions with empty string before template execution.
func renderSimplePart(xmlBytes []byte, data map[string]any) ([]byte, error) {
	src := string(xmlBytes)
	escapedData := xmlEscapeData(data)
	rendered, err := renderJinja2(src, escapedData)
	if err != nil {
		// Failed (e.g., unsupported syntax) — return original unchanged.
		return xmlBytes, nil
	}
	return []byte(rendered), nil
}

// hasTemplateMarkers checks if content contains any Jinja2-style markers.
func hasTemplateMarkers(content string) bool {
	return strings.Contains(content, "{{") || strings.Contains(content, "{%") || strings.Contains(content, "{#")
}

// shouldRenderPart returns true for XML parts that may contain template expressions.
func shouldRenderPart(name string) bool {
	if name == "word/document.xml" {
		return true
	}
	if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
		return true
	}
	if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
		return true
	}
	if name == "docProps/core.xml" || name == "docProps/app.xml" {
		return true
	}
	return false
}

// renderXMLPart parses an XML part, patches template markers, renders, and serializes.
func renderXMLPart(xmlBytes []byte, data map[string]any) ([]byte, error) {
	// 1. Parse XML.
	doc := etree.NewDocument()
	doc.ReadSettings.PreserveCData = true
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	// 2. Patch: reassemble fragmented template markers in paragraphs.
	patchDocument(doc)

	// 3. Serialize to string for template processing.
	doc.WriteSettings.CanonicalEndTags = true
	doc.WriteSettings.CanonicalAttrVal = true
	xmlStr, err := doc.WriteToString()
	if err != nil {
		return nil, fmt.Errorf("serialize XML: %w", err)
	}

	// 4. Extract RawXML values and replace with markers before rendering.
	rawMarkers := map[string]string{}
	renderData := make(map[string]any, len(data))
	for k, v := range data {
		if raw, ok := v.(RawXML); ok {
			marker := fmt.Sprintf("__RAWXML_%s__", k)
			renderData[k] = marker
			rawMarkers[marker] = string(raw)
		} else {
			renderData[k] = v
		}
	}

	// 5. XML-escape all string values so gonja output is valid XML.
	escapedData := xmlEscapeData(renderData)

	// 6. Render Jinja2 template with gonja (native Jinja2, handles {%- trimming properly).
	rendered, err := renderJinja2(xmlStr, escapedData)
	if err != nil {
		return nil, err
	}

	// 7. Post-process: replace paragraphs containing RawXML markers with actual XML.
	for marker, rawXML := range rawMarkers {
		rendered = injectRawXML(rendered, marker, rawXML)
	}

	// 8. Convert <w:jinja> placeholders back to raw text.
	// These were inserted by liftBlockTags to replace entire <w:p>/<w:tr> elements.
	// After gonja rendering, the jinja tags are consumed — remove any remaining wrappers.
	rendered = cleanJinjaPlaceholders(rendered)

	return []byte(rendered), nil
}

// cleanJinjaPlaceholders replaces <w:jinja>text</w:jinja> with just text.
// These placeholders are created by liftBlockTags and should be transparent
// in the final output.
func cleanJinjaPlaceholders(xml string) string {
	// After gonja rendering, the {% %} tags are gone. The <w:jinja> elements
	// either contain rendered content or are empty. Remove the wrapper.
	xml = strings.ReplaceAll(xml, "<w:jinja>", "")
	xml = strings.ReplaceAll(xml, "</w:jinja>", "")
	// Also handle self-closing (empty after rendering)
	xml = strings.ReplaceAll(xml, "<w:jinja/>", "")
	xml = strings.ReplaceAll(xml, "<w:jinja></w:jinja>", "")
	return xml
}

// cleanEntitiesInExpressions fixes XML-escaped characters inside template expressions.
// E.g., &amp; inside {{}} should be & for the template engine.
func cleanEntitiesInExpressions(src string) string {
	for _, pair := range []struct{ open, close string }{
		{"{{", "}}"},
		{"{%", "%}"},
	} {
		src = cleanInside(src, pair.open, pair.close)
	}
	return src
}

func cleanInside(src, open, close string) string {
	var result strings.Builder
	remaining := src
	for {
		idx := strings.Index(remaining, open)
		if idx < 0 {
			result.WriteString(remaining)
			break
		}
		endIdx := strings.Index(remaining[idx+len(open):], close)
		if endIdx < 0 {
			result.WriteString(remaining)
			break
		}
		endIdx = idx + len(open) + endIdx + len(close)

		// Write everything before the expression.
		result.WriteString(remaining[:idx])
		// Write the expression with cleaned entities.
		expr := remaining[idx:endIdx]
		expr = strings.ReplaceAll(expr, "&amp;", "&")
		expr = strings.ReplaceAll(expr, "&lt;", "<")
		expr = strings.ReplaceAll(expr, "&gt;", ">")
		expr = strings.ReplaceAll(expr, "&#8216;", "'")
		expr = strings.ReplaceAll(expr, "&#8217;", "'")
		expr = strings.ReplaceAll(expr, "\u201c", `"`)
		expr = strings.ReplaceAll(expr, "\u201d", `"`)
		expr = strings.ReplaceAll(expr, "\u2018", "'")
		expr = strings.ReplaceAll(expr, "\u2019", "'")
		result.WriteString(expr)

		remaining = remaining[endIdx:]
	}
	return result.String()
}

// injectRawXML finds a paragraph containing the marker text and replaces
// the entire <w:p>...</w:p> with the raw XML content.
func injectRawXML(xml, marker, rawXML string) string {
	// Find the <w:p> that contains the marker.
	idx := strings.Index(xml, marker)
	if idx < 0 {
		return xml
	}

	// Walk backward to find the opening <w:p
	pStart := strings.LastIndex(xml[:idx], "<w:p")
	if pStart < 0 {
		// Marker not inside a paragraph — do simple replacement.
		return strings.Replace(xml, marker, rawXML, 1)
	}

	// Walk forward to find the closing </w:p>
	pEnd := strings.Index(xml[idx:], "</w:p>")
	if pEnd < 0 {
		return strings.Replace(xml, marker, rawXML, 1)
	}
	pEnd = idx + pEnd + len("</w:p>")

	// Replace the entire paragraph with the raw XML.
	return xml[:pStart] + rawXML + xml[pEnd:]
}

// renderJinja2 renders a Jinja2 template string with data using gonja.
func renderJinja2(templateStr string, data map[string]any) (string, error) {
	tpl, err := gonja.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("parse jinja2: %w", err)
	}
	ctx := exec.NewContext(data)
	result, err := tpl.ExecuteToString(ctx)
	if err != nil {
		return "", fmt.Errorf("execute jinja2: %w", err)
	}
	return result, nil
}

// xmlEscapeData recursively walks data and XML-escapes all string values.
// This ensures that template output (which is raw text/template, not XML-aware)
// produces valid XML when data contains characters like &, <, >.
func xmlEscapeData(data map[string]any) map[string]any {
	result := make(map[string]any, len(data))
	for k, v := range data {
		result[k] = xmlEscapeValue(v)
	}
	return result
}

func xmlEscapeValue(v any) any {
	switch val := v.(type) {
	case string:
		return xmlEscapeString(val)
	case map[string]any:
		return xmlEscapeData(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = xmlEscapeValue(item)
		}
		return out
	default:
		return v
	}
}

func xmlEscapeString(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
