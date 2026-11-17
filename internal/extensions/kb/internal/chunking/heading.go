// SPDX-License-Identifier: Apache-2.0

// Package chunking implements text chunking strategies for vector DB ingestion.
package chunking

import (
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// HeadingChunker splits markdown content on heading boundaries (# ## ###).
// Merges small sections, splits large ones.
type HeadingChunker struct {
	MinSize int // merge sections smaller than this (default 200)
	MaxSize int // split sections larger than this (default 1500)
}

func (c *HeadingChunker) Name() string { return "markdown-heading" }

func (c *HeadingChunker) Chunk(content string, meta kbapi.ChunkMeta) ([]kbapi.Chunk, error) {
	minSize := c.MinSize
	maxSize := c.MaxSize
	if minSize <= 0 {
		minSize = 200
	}
	if maxSize <= 0 {
		maxSize = 1500
	}

	// Split on heading lines
	sections := splitOnHeadings(content)

	// Merge small sections, split large ones
	var chunks []kbapi.Chunk
	idx := 0

	var buffer strings.Builder
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		// If buffer + section fits, accumulate
		if buffer.Len() > 0 && buffer.Len()+len(section) < minSize {
			buffer.WriteString("\n\n")
			buffer.WriteString(section)
			continue
		}

		// Flush buffer if non-empty
		if buffer.Len() > 0 {
			for _, ch := range splitIfTooLarge(buffer.String(), maxSize, idx) {
				chunks = append(chunks, ch)
				idx++
			}
			buffer.Reset()
		}

		// Start new buffer
		buffer.WriteString(section)
	}

	// Flush remaining
	if buffer.Len() > 0 {
		for _, ch := range splitIfTooLarge(buffer.String(), maxSize, idx) {
			chunks = append(chunks, ch)
			idx++
		}
	}

	return chunks, nil
}

// splitOnHeadings splits markdown text at heading lines (lines starting with #).
func splitOnHeadings(content string) []string {
	lines := strings.Split(content, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}

// splitIfTooLarge splits a section into chunks if it exceeds maxSize.
func splitIfTooLarge(text string, maxSize, startIdx int) []kbapi.Chunk {
	if len(text) <= maxSize {
		return []kbapi.Chunk{{Content: text, Index: startIdx}}
	}

	var chunks []kbapi.Chunk
	idx := startIdx
	remaining := text

	for len(remaining) > maxSize {
		// Find a good split point (paragraph break or sentence end)
		splitAt := maxSize
		if pos := strings.LastIndex(remaining[:maxSize], "\n\n"); pos > maxSize/2 {
			splitAt = pos
		} else if pos := strings.LastIndex(remaining[:maxSize], ". "); pos > maxSize/2 {
			splitAt = pos + 1
		}

		chunks = append(chunks, kbapi.Chunk{Content: strings.TrimSpace(remaining[:splitAt]), Index: idx})
		idx++
		remaining = strings.TrimSpace(remaining[splitAt:])
	}

	if len(remaining) > 0 {
		chunks = append(chunks, kbapi.Chunk{Content: remaining, Index: idx})
	}

	return chunks
}

// FixedSizeChunker splits text at character boundaries with overlap.
type FixedSizeChunker struct {
	MaxSize int // chunk size in characters (default 1500)
	Overlap int // overlap between chunks (default 200)
}

func (c *FixedSizeChunker) Name() string { return "fixed-size" }

func (c *FixedSizeChunker) Chunk(content string, _ kbapi.ChunkMeta) ([]kbapi.Chunk, error) {
	maxSize := c.MaxSize
	overlap := c.Overlap
	if maxSize <= 0 {
		maxSize = 1500
	}
	if overlap <= 0 {
		overlap = 200
	}
	if overlap >= maxSize {
		overlap = maxSize / 4
	}

	content = strings.TrimSpace(content)
	if len(content) <= maxSize {
		return []kbapi.Chunk{{Content: content, Index: 0}}, nil
	}

	var chunks []kbapi.Chunk
	idx := 0
	pos := 0

	for pos < len(content) {
		end := pos + maxSize
		if end > len(content) {
			end = len(content)
		}

		// Try to break at a sentence or paragraph boundary
		if end < len(content) {
			if p := strings.LastIndex(content[pos:end], "\n\n"); p > maxSize/2 {
				end = pos + p
			} else if p := strings.LastIndex(content[pos:end], ". "); p > maxSize/2 {
				end = pos + p + 1
			}
		}

		chunks = append(chunks, kbapi.Chunk{
			Content: strings.TrimSpace(content[pos:end]),
			Index:   idx,
		})
		idx++

		pos = end - overlap
		if pos <= chunks[len(chunks)-1].Index {
			pos = end // prevent infinite loop
		}
	}

	return chunks, nil
}
