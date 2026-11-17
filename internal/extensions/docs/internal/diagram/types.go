// SPDX-License-Identifier: Apache-2.0

package diagram

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

// DiagramTypes lists all supported Kroki diagram types.
var DiagramTypes = []string{
	"blockdiag", "bpmn", "bytefield", "seqdiag", "actdiag", "nwdiag",
	"packetdiag", "rackdiag", "c4plantuml", "ditaa", "erd", "excalidraw",
	"graphviz", "mermaid", "nomnoml", "plantuml", "svgbob", "umlet",
	"vega", "vegalite", "wavedrom", "d2", "dbml", "pikchr", "structurizr",
	"tikz", "wireviz",
}

// DiagramSynonyms maps alternative names to canonical Kroki type names.
var DiagramSynonyms = map[string]string{
	"dot":       "graphviz",
	"c4":        "c4plantuml",
	"vega-lite": "vegalite",
}

// ForeignObjectTypes are diagram types whose SVG output uses <foreignObject>,
// which is incompatible with Typst and Word. These must be rendered as PNG when
// the target format is not HTML.
var ForeignObjectTypes = map[string]bool{
	"mermaid": true,
	"nomnoml": true,
	"svgbob":  true,
}

// SupportedFormats lists the output formats Kroki supports.
var SupportedFormats = []string{"svg", "png", "jpg", "pdf"}

// FileExtToType maps common file extensions to diagram types.
var FileExtToType = map[string]string{
	".mmd":      "mermaid",
	".mermaid":  "mermaid",
	".puml":     "plantuml",
	".plantuml": "plantuml",
	".dot":      "graphviz",
	".gv":       "graphviz",
	".d2":       "d2",
	".erd":      "erd",
	".dbml":     "dbml",
	".ditaa":    "ditaa",
	".bpmn":     "bpmn",
	".tikz":     "tikz",
}

// diagramTypeSet is a lookup set built from DiagramTypes + DiagramSynonyms.
var diagramTypeSet map[string]bool

func init() {
	diagramTypeSet = make(map[string]bool, len(DiagramTypes)+len(DiagramSynonyms))
	for _, t := range DiagramTypes {
		diagramTypeSet[t] = true
	}
	for syn := range DiagramSynonyms {
		diagramTypeSet[syn] = true
	}
}

// ResolveDiagramType returns the canonical type name, resolving synonyms.
// Returns empty string if the type is unknown.
func ResolveDiagramType(name string) string {
	if canonical, ok := DiagramSynonyms[name]; ok {
		return canonical
	}
	if diagramTypeSet[name] {
		return name
	}
	return ""
}

// IsValidFormat checks whether a format is supported.
func IsValidFormat(format string) bool {
	for _, f := range SupportedFormats {
		if f == format {
			return true
		}
	}
	return false
}

// DetectDiagramType tries to infer the diagram type from the file extension.
// Returns empty string if unknown.
func DetectDiagramType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	return FileExtToType[ext]
}

// CacheKey returns a deterministic filename for caching: {type}-{hash12}.{format}.
func CacheKey(diagramType, source, format string) string {
	h := sha256.Sum256([]byte(source))
	hash12 := fmt.Sprintf("%x", h[:])[:12]
	return fmt.Sprintf("%s-%s.%s", diagramType, hash12, format)
}

// EffectiveFormat returns the format to use, accounting for foreignObject
// types that need PNG when rendering for Typst.
func EffectiveFormat(diagramType, requestedFormat string, forTypst bool) string {
	if forTypst && ForeignObjectTypes[diagramType] && requestedFormat == "svg" {
		return "png"
	}
	return requestedFormat
}
