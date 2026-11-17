// SPDX-License-Identifier: Apache-2.0

package api

// Chunker splits markdown content into chunks suitable for vector DB ingestion.
type Chunker interface {
	Name() string
	Chunk(content string, meta ChunkMeta) ([]Chunk, error)
}

// ChunkMeta provides context about the content being chunked.
type ChunkMeta struct {
	SourcePath string // for context
	Part       string // "page", "slide", etc.
	PartIndex  int
}

// Chunk is a single piece of text ready for embedding and indexing.
type Chunk struct {
	Content   string         // the text chunk
	Index     int            // position within the part
	StartLine int            // line offset in source content file
	Metadata  map[string]any // additional chunk-level metadata
}
