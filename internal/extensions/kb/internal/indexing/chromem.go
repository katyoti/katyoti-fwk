// SPDX-License-Identifier: Apache-2.0

package indexing

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/philippgille/chromem-go"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// ChromemIndexer implements KnowledgeIndexer using chromem-go (embedded, pure Go).
// Data is persisted to <kb_root>/.index/ — no server needed.
type ChromemIndexer struct {
	db          *chromem.DB
	collection  *chromem.Collection
	collName    string
	persistPath string
}

// NewChromemIndexer creates a chromem-go indexer.
// config["path"] = path to persist the index (default: <kb_root>/.index)
func NewChromemIndexer(config map[string]any) (*ChromemIndexer, error) {
	persistPath := ""
	if config != nil {
		if p, ok := config["path"].(string); ok {
			persistPath = p
		}
	}
	// chromem-go creates a persistent DB at this path
	db, err := chromem.NewPersistentDB(persistPath, false)
	if err != nil {
		return nil, fmt.Errorf("create chromem db: %w", err)
	}
	return &ChromemIndexer{db: db, persistPath: persistPath}, nil
}

func (c *ChromemIndexer) Init(ctx context.Context, collection string, dimension int, meta map[string]string) error {
	c.collName = collection

	// Check if collection exists
	col := c.db.GetCollection(collection, nil)
	if col != nil {
		c.collection = col
		return nil
	}

	// Create collection — chromem-go doesn't need dimension upfront,
	// it infers from the first insert. We pass a no-op embedding function
	// since we provide pre-computed embeddings.
	col, err := c.db.CreateCollection(collection, map[string]string(meta), nil)
	if err != nil {
		return fmt.Errorf("create collection %q: %w", collection, err)
	}
	c.collection = col
	return nil
}

func (c *ChromemIndexer) Index(ctx context.Context, chunks []kbapi.KnowledgeChunk) error {
	if c.collection == nil {
		return fmt.Errorf("collection not initialized")
	}
	if len(chunks) == 0 {
		return nil
	}

	docs := make([]chromem.Document, len(chunks))
	for i, ch := range chunks {
		// Convert metadata map[string]string to map[string]string (already correct type)
		docs[i] = chromem.Document{
			ID:        ch.ID,
			Content:   ch.Content,
			Metadata:  ch.Metadata,
			Embedding: ch.Vector,
		}
	}

	return c.collection.AddDocuments(ctx, docs, runtime_concurrency())
}

func (c *ChromemIndexer) Delete(ctx context.Context, filter kbapi.ChunkFilter) (int, error) {
	if c.collection == nil {
		return 0, fmt.Errorf("collection not initialized")
	}
	where := filterWhere(filter)
	if len(where) == 0 {
		return 0, fmt.Errorf("delete: no filter - it would empty the collection")
	}
	before := c.collection.Count()
	if err := c.collection.Delete(ctx, where, nil); err != nil {
		return 0, err
	}
	return before - c.collection.Count(), nil
}

// filterWhere is a chunk filter as metadata equalities.
func filterWhere(filter kbapi.ChunkFilter) map[string]string {
	where := map[string]string{}
	if filter.Slug != "" {
		where["slug"] = filter.Slug
	}
	if filter.SourceHash != "" {
		where["source_hash"] = filter.SourceHash
	}
	return where
}

func (c *ChromemIndexer) Query(ctx context.Context, query kbapi.QueryRequest) ([]kbapi.QueryResult, error) {
	if c.collection == nil {
		return nil, fmt.Errorf("collection not initialized")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	// chromem-go returns an error when n_results exceeds the number of documents
	// in the collection. Clamp to the collection size so a small KB (fewer docs
	// than the requested limit) still returns what it has instead of failing.
	if n := c.collection.Count(); limit > n {
		limit = n
	}
	if limit == 0 {
		// Empty collection — nothing to return, and QueryEmbedding(0) would error.
		return nil, nil
	}

	// Build where filter
	var whereFilter map[string]string
	if len(query.Filter) > 0 {
		whereFilter = query.Filter
	}

	// Query with pre-computed embedding
	results, err := c.collection.QueryEmbedding(ctx, query.Vector, limit, whereFilter, nil)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	var out []kbapi.QueryResult
	for _, r := range results {
		source := kbapi.SourceInfo{
			KBName:      r.Metadata["kb_name"],
			SourcePath:  r.Metadata["source_path"],
			RemotePath:  r.Metadata["remote_path"],
			SyncSource:  r.Metadata["sync_source"],
			Part:        r.Metadata["part"],
			ExtractedAt: r.Metadata["extracted_at"],
		}
		if v, err := strconv.Atoi(r.Metadata["part_index"]); err == nil {
			source.PartIndex = v
		}
		if v, err := strconv.Atoi(r.Metadata["chunk_index"]); err == nil {
			source.ChunkIndex = v
		}

		out = append(out, kbapi.QueryResult{
			Content: r.Content,
			Score:   float64(r.Similarity),
			Source:  source,
		})
	}

	return out, nil
}

// CollectionMeta returns the metadata the collection was created with (the
// embedding model and dimensions recorded at index time). chromem-go keeps
// it unexported, so it is read from the collection's persisted metadata file
// (<path>/<hash of the name>/00000000.gob). A collection that does not exist,
// or an in-memory index, has no metadata.
func (c *ChromemIndexer) CollectionMeta(ctx context.Context, collection string) (map[string]string, error) {
	if c.persistPath == "" || c.db.GetCollection(collection, nil) == nil {
		return map[string]string{}, nil
	}
	sum := sha256.Sum256([]byte(collection))
	path := filepath.Join(c.persistPath, hex.EncodeToString(sum[:4]), "00000000.gob")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read collection %q metadata: %w", collection, err)
	}
	defer f.Close()
	var pc struct {
		Name     string
		Metadata map[string]string
	}
	if err := gob.NewDecoder(f).Decode(&pc); err != nil {
		return nil, fmt.Errorf("decode collection %q metadata: %w", collection, err)
	}
	if pc.Metadata == nil {
		pc.Metadata = map[string]string{}
	}
	return pc.Metadata, nil
}

// Drop deletes the collection and its persisted documents; dropping a
// collection that does not exist is not an error.
func (c *ChromemIndexer) Drop(ctx context.Context, collection string) error {
	if c.db.GetCollection(collection, nil) == nil {
		return nil
	}
	if err := c.db.DeleteCollection(collection); err != nil {
		return fmt.Errorf("drop collection %q: %w", collection, err)
	}
	if c.collName == collection {
		c.collection = nil
	}
	return nil
}

// Count returns the number of documents in the collection.
func (c *ChromemIndexer) Count() int {
	if c.collection == nil {
		return 0
	}
	return c.collection.Count()
}

// --- helpers ---

func float32ToFloat64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}

func float64ToFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(f)
	}
	return out
}

func runtime_concurrency() int {
	return int(math.Max(1, 4))
}

// StoragePath returns the index path for a KB root.
func StoragePath(kbRoot string) string {
	return filepath.Join(kbRoot, ".index")
}
