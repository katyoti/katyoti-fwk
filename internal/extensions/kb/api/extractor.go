// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// Extractor is the core interface for knowledge extraction.
// Both compiled Go extractors and external command wrappers implement this.
type Extractor interface {
	// Name returns the extractor identifier used in profile config.
	Name() string

	// Formats returns the file extensions this extractor can handle.
	Formats() []string

	// Extract processes a source file and writes output to the bundle dir.
	Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *ExtractLimits) (*ExtractResult, error)
}

// ExtractLimits controls extraction boundaries.
type ExtractLimits struct {
	MaxFileSize int64 `yaml:"max_file_size" json:"max_file_size"` // skip if larger (bytes, 0 = no limit)
	MaxPages    int   `yaml:"max_pages" json:"max_pages"`         // PDF/PPTX: first N pages (0 = all)
	MaxRows     int   `yaml:"max_rows" json:"max_rows"`           // XLSX: first N rows per sheet (0 = all)
}

// ExtractResult describes the output of an extraction.
type ExtractResult struct {
	ContentFiles []ContentFile // produced markdown files
	Attachments  []Attachment  // produced attachment files
	TotalParts   int           // total parts in the source document
}

// ContentFile describes a produced markdown content file.
type ContentFile struct {
	File  string // relative path within bundle: "content.md" or "content_page_001.md"
	Part  string // part type: "page", "slide", "sheet", "segment", "whole"
	Index int    // 1-based index within the document (0 = single-part, no index)
}

// Attachment describes a produced attachment file.
type Attachment struct {
	File         string // relative path: "attachments/page_003.png"
	Type         string // "page_render", "table", "figure", "audio_segment"
	ReferencedBy string // which content file references this
}
