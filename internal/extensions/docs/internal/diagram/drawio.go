// SPDX-License-Identifier: Apache-2.0

package diagram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
)

// drawioFileExts lists the file extensions handled by DrawioRenderer.
var drawioFileExts = []string{".drawio", ".drawio.png", ".drawio.svg", ".drawio.xml"}

// DrawioRenderer renders draw.io diagrams via the drawio engine's
// desktop-CLI export.
// It implements DiagramRenderer for .drawio files.
type DrawioRenderer struct{}

// NewDrawioRenderer creates a DrawioRenderer, locating the draw.io CLI.
// Returns nil (not an error) if draw.io is not installed -- the renderer
// simply won't be registered in the dispatcher.
func NewDrawioRenderer() *DrawioRenderer {
	if _, err := drawiorender.FindDrawioExecutable(); err != nil {
		return nil
	}
	return &DrawioRenderer{}
}

// MatchesLang returns false -- draw.io diagrams are file-based only,
// not fenced code blocks.
func (d *DrawioRenderer) MatchesLang(lang string) bool {
	return false
}

// MatchesFile returns true for .drawio, .drawio.png, .drawio.svg, .drawio.xml files.
func (d *DrawioRenderer) MatchesFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	for _, ext := range drawioFileExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// RenderSource is not supported for draw.io (file-based only).
func (d *DrawioRenderer) RenderSource(lang, source, format string) ([]byte, error) {
	return nil, fmt.Errorf("drawio: fenced code blocks not supported, use file references")
}

// RenderFile renders a draw.io file to image bytes via the desktop CLI.
func (d *DrawioRenderer) RenderFile(path string, format string) ([]byte, error) {
	name := strings.ToLower(filepath.Base(path))

	switch {
	case strings.HasSuffix(name, ".drawio.png"):
		// Already a rendered PNG -- just read it
		if format == "png" {
			return os.ReadFile(path)
		}
		// Need to extract XML and re-render
		return d.renderFromExtracted(path, format)

	case strings.HasSuffix(name, ".drawio.svg"):
		// Already a rendered SVG -- just read it
		if format == "svg" {
			return os.ReadFile(path)
		}
		// Need to extract XML and re-render
		return d.renderFromExtracted(path, format)

	case strings.HasSuffix(name, ".drawio"), strings.HasSuffix(name, ".drawio.xml"):
		return d.renderCLI(path, format)

	default:
		return nil, fmt.Errorf("drawio: unrecognized file type: %s", filepath.Base(path))
	}
}

// renderCLI renders a .drawio file through the drawio engine's export.
func (d *DrawioRenderer) renderCLI(sourcePath, outputFormat string) ([]byte, error) {
	return drawiorender.Export(sourcePath, outputFormat)
}

// renderFromExtracted extracts XML from a .drawio.png/.drawio.svg and re-renders.
func (d *DrawioRenderer) renderFromExtracted(path, format string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("drawio: read file: %w", err)
	}

	name := strings.ToLower(filepath.Base(path))
	var xmlData []byte
	if strings.HasSuffix(name, ".drawio.png") {
		xmlData, err = drawioapi.ExtractXMLFromPNG(data)
	} else {
		xmlData, err = drawioapi.ExtractXMLFromSVG(data)
	}
	if err != nil {
		return nil, fmt.Errorf("drawio: extract xml: %w", err)
	}

	// Write extracted XML to temp file, render via CLI
	tmpXML, err := os.CreateTemp("", "drawio_*.drawio")
	if err != nil {
		return nil, fmt.Errorf("drawio: create temp xml: %w", err)
	}
	tmpXMLPath := tmpXML.Name()
	tmpXML.Write(xmlData)
	tmpXML.Close()
	defer os.Remove(tmpXMLPath)

	return d.renderCLI(tmpXMLPath, format)
}
