// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// KnowledgeIndexer is the strategy interface for vector DB backends.
type KnowledgeIndexer interface {
	// Init creates/opens the collection with the given dimension and metadata.
	Init(ctx context.Context, collection string, dimension int, meta map[string]string) error

	// Index upserts chunks into the vector DB.
	Index(ctx context.Context, chunks []KnowledgeChunk) error

	// Delete removes chunks matching the filter.
	// Delete removes the chunks the filter names and answers how many; an
	// empty filter is refused (it would empty the collection).
	Delete(ctx context.Context, filter ChunkFilter) (int, error)

	// Query performs semantic search.
	Query(ctx context.Context, query QueryRequest) ([]QueryResult, error)

	// CollectionMeta returns metadata stored on the collection (embedding model, etc.).
	CollectionMeta(ctx context.Context, collection string) (map[string]string, error)
}

// KnowledgeChunk is a chunk ready for vector DB insertion.
type KnowledgeChunk struct {
	ID       string            // unique chunk identifier
	Content  string            // text content
	Vector   []float32         // embedding vector
	Metadata map[string]string // provenance metadata (kb_name, source_path, part, etc.)
}

// ChunkFilter selects chunks for deletion.
type ChunkFilter struct {
	SourceHash string // delete all chunks from this source document
	Slug       string // delete all chunks from this bundle
}

// QueryRequest is a semantic search query.
type QueryRequest struct {
	Vector []float32         // query embedding
	Limit  int               // max results
	Filter map[string]string // metadata filters
}
