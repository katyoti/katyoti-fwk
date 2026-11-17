// SPDX-License-Identifier: Apache-2.0

package diagram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// KrokiRenderer renders diagrams via the Kroki API.
// It implements DiagramRenderer for all Kroki-supported diagram types.
type KrokiRenderer struct {
	ServerURL  string
	HTTPClient *http.Client
}

// NewKrokiRenderer creates a renderer pointing at the given Kroki server URL.
func NewKrokiRenderer(serverURL string) *KrokiRenderer {
	return &KrokiRenderer{
		ServerURL: serverURL,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// MatchesLang returns true if the language is a known Kroki diagram type.
func (k *KrokiRenderer) MatchesLang(lang string) bool {
	return ResolveDiagramType(lang) != ""
}

// MatchesFile returns true if the file extension maps to a known diagram type.
func (k *KrokiRenderer) MatchesFile(path string) bool {
	return DetectDiagramType(path) != ""
}

// RenderSource sends diagram source to Kroki and returns the image bytes.
func (k *KrokiRenderer) RenderSource(lang, source, format string) ([]byte, error) {
	diagramType := ResolveDiagramType(lang)
	if diagramType == "" {
		return nil, fmt.Errorf("unsupported diagram type: %q", lang)
	}
	return k.render(diagramType, source, format)
}

// RenderFile reads a diagram file and renders it via Kroki.
func (k *KrokiRenderer) RenderFile(path string, format string) ([]byte, error) {
	diagramType := DetectDiagramType(path)
	if diagramType == "" {
		return nil, fmt.Errorf("cannot detect diagram type from %q", path)
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read diagram file: %w", err)
	}

	return k.render(diagramType, string(source), format)
}

// RenderRequest describes a diagram to render (used by server/CLI for direct access).
type RenderRequest struct {
	Type   string // diagram type (e.g. "mermaid", "plantuml", "dot")
	Source string // diagram source code
	Format string // output format: "svg", "png", "jpg", "pdf"
}

// RenderResult holds the rendered image data.
type RenderResult struct {
	Data        []byte // raw image bytes
	ContentType string // MIME type of the response
}

// Render sends a RenderRequest to Kroki (direct API, used by server endpoints).
func (k *KrokiRenderer) Render(req RenderRequest) (*RenderResult, error) {
	diagramType := ResolveDiagramType(req.Type)
	if diagramType == "" {
		return nil, fmt.Errorf("unsupported diagram type: %q", req.Type)
	}

	format := req.Format
	if format == "" {
		format = "svg"
	}
	if !IsValidFormat(format) {
		return nil, fmt.Errorf("unsupported format: %q", format)
	}

	data, contentType, err := k.doRequest(diagramType, req.Source, format)
	if err != nil {
		return nil, err
	}

	return &RenderResult{Data: data, ContentType: contentType}, nil
}

// RenderCached renders a diagram with file caching. Returns the cached file path.
func (k *KrokiRenderer) RenderCached(req RenderRequest, outputDir string) (string, error) {
	diagramType := ResolveDiagramType(req.Type)
	if diagramType == "" {
		return "", fmt.Errorf("unsupported diagram type: %q", req.Type)
	}

	format := req.Format
	if format == "" {
		format = "svg"
	}

	filename := CacheKey(diagramType, req.Source, format)
	path := fmt.Sprintf("%s/%s", outputDir, filename)

	// Check cache
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	result, err := k.Render(req)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(path, result.Data, 0o644); err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}

	return path, nil
}

// render is the internal render method used by DiagramRenderer interface methods.
func (k *KrokiRenderer) render(diagramType, source, format string) ([]byte, error) {
	if format == "" {
		format = "svg"
	}
	if !IsValidFormat(format) {
		return nil, fmt.Errorf("unsupported format: %q", format)
	}

	data, _, err := k.doRequest(diagramType, source, format)
	return data, err
}

// doRequest sends the HTTP request to the Kroki API.
func (k *KrokiRenderer) doRequest(diagramType, source, format string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/%s/%s", k.ServerURL, diagramType, format)

	payload, err := json.Marshal(map[string]string{
		"diagram_source": source,
	})
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "ktw-docs/1.0")
	if format == "svg" {
		httpReq.Header.Set("Accept", "image/svg+xml")
	} else {
		httpReq.Header.Set("Accept", "image/"+format)
	}

	resp, err := k.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("kroki request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("kroki returned HTTP %d: %s", resp.StatusCode, extractKrokiError(body))
	}

	return body, resp.Header.Get("Content-Type"), nil
}

// extractKrokiError pulls a human-readable message from a Kroki error response.
func extractKrokiError(body []byte) string {
	s := string(body)

	if strings.Contains(s, "<svg") {
		re := regexp.MustCompile(`<tspan[^>]*>([^<]*Error[^<]*)</tspan>`)
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		re = regexp.MustCompile(`<tspan[^>]*>([^<]+)</tspan>`)
		matches := re.FindAllStringSubmatch(s, 3)
		var lines []string
		for _, m := range matches {
			if t := strings.TrimSpace(m[1]); t != "" {
				lines = append(lines, t)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, " ")
		}
	}

	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
