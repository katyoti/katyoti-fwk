// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/bundling"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/chunking"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/indexing"
)

func (s *kbService) Index(ctx context.Context, in kbapi.IndexInput) (kbapi.IndexOutput, error) {
	if in.Path == "" {
		return kbapi.IndexOutput{}, fmt.Errorf("path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.IndexOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	kbCfg, err := LoadKnowledgeBase(absPath)
	if err != nil {
		return kbapi.IndexOutput{}, err
	}

	// Initialize chunker
	chunkerCfg := kbCfg.Indexing.Chunking
	chunker := resolveChunker(chunkerCfg)

	// The embedder of the profile the knowledge base names
	embedder, err := s.embedderFor(ctx, kbCfg)
	if err != nil {
		return kbapi.IndexOutput{}, fmt.Errorf("init embedder: %w", err)
	}

	// Initialize indexer from backend config
	indexCfg := kbCfg.Indexing.Config
	if indexCfg == nil {
		indexCfg = map[string]any{}
	}
	if _, ok := indexCfg["path"]; !ok {
		indexCfg["path"] = indexing.StoragePath(absPath)
	}
	indexerInst, err := indexing.ResolveIndexer(kbCfg.Indexing.Backend, indexCfg)
	if err != nil {
		return kbapi.IndexOutput{}, fmt.Errorf("init indexer: %w", err)
	}

	// Get collection name
	collection := kbCfg.Name
	if col, ok := kbCfg.Indexing.Config["collection"].(string); ok && col != "" {
		collection = col
	}

	// Force drops the collection; otherwise an existing collection must have
	// been embedded with the same model and dimensions.
	if in.Force {
		if d, ok := indexerInst.(dropper); ok {
			if err := d.Drop(ctx, collection); err != nil {
				return kbapi.IndexOutput{}, err
			}
		}
	} else {
		meta, err := indexerInst.CollectionMeta(ctx, collection)
		if err != nil {
			return kbapi.IndexOutput{}, fmt.Errorf("read collection metadata: %w", err)
		}
		if err := checkCompatible(meta, embedder); err != nil {
			return kbapi.IndexOutput{}, err
		}
	}

	// Init collection, recording the embedding model and dimensions
	collMeta := embeddingMeta(embedder)
	collMeta["kb_name"] = kbCfg.Name
	if err := indexerInst.Init(ctx, collection, embedder.Dimension(), collMeta); err != nil {
		return kbapi.IndexOutput{}, fmt.Errorf("init collection: %w", err)
	}

	// Scan bundles
	bundlesDir := filepath.Join(absPath, ".bundles")
	entries, err := os.ReadDir(bundlesDir)
	if err != nil {
		return kbapi.IndexOutput{}, fmt.Errorf("read bundles: %w", err)
	}

	var out kbapi.IndexOutput
	batchSize := 20 // embed N chunks at a time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		meta, err := bundling.LoadMetadata(absPath, slug)
		if err != nil {
			continue
		}
		if meta.Status == "stale" {
			// Its source no longer brings it: its chunks leave the index,
			// counted, and so does its bundle.
			n, err := indexerInst.Delete(ctx, kbapi.ChunkFilter{Slug: slug})
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("delete %s: %v", slug, err))
				continue
			}
			out.ChunksDeleted += n
			if err := os.RemoveAll(filepath.Join(bundlesDir, slug)); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("remove bundle %s: %v", slug, err))
			}
			continue
		}
		if meta.Status != "extracted" {
			continue
		}

		// Read all content files for this bundle
		var allText strings.Builder
		for _, cf := range meta.Content {
			cfPath := filepath.Join(bundlesDir, slug, cf.File)
			data, err := os.ReadFile(cfPath)
			if err != nil {
				continue
			}
			// Strip frontmatter
			content := stripFrontmatter(string(data))
			if allText.Len() > 0 {
				allText.WriteString("\n\n---\n\n")
			}
			allText.WriteString(content)
		}

		text := allText.String()
		if strings.TrimSpace(text) == "" {
			continue
		}

		// Chunk
		chunks, err := chunker.Chunk(text, kbapi.ChunkMeta{
			SourcePath: meta.Source.Path,
			Part:       "whole",
		})
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("chunk %s: %v", slug, err))
			continue
		}

		// Build knowledge chunks with metadata
		var kChunks []kbapi.KnowledgeChunk
		for _, ch := range chunks {
			chunkID := fmt.Sprintf("%x", sha256.Sum256([]byte(slug+fmt.Sprintf("-%d", ch.Index))))[:16]

			chMeta := map[string]string{
				"kb_name":            kbCfg.Name,
				"slug":               slug,
				"source_path":        meta.Source.Path,
				"source_hash":        meta.Source.Hash,
				"part":               "whole",
				"chunk_index":        fmt.Sprintf("%d", ch.Index),
				"extraction_profile": meta.Extraction.Profile,
				"extracted_at":       meta.Extraction.ExtractedAt.Format(time.RFC3339),
				"indexed_at":         time.Now().Format(time.RFC3339),
			}
			if meta.Source.Origin != nil {
				chMeta["sync_source"] = meta.Source.Origin.SyncSource
				chMeta["remote_path"] = meta.Source.Origin.RemotePath
			}

			kChunks = append(kChunks, kbapi.KnowledgeChunk{
				ID:       chunkID,
				Content:  ch.Content,
				Metadata: chMeta,
			})
		}

		// Embed in batches
		for i := 0; i < len(kChunks); i += batchSize {
			end := i + batchSize
			if end > len(kChunks) {
				end = len(kChunks)
			}
			batch := kChunks[i:end]

			texts := make([]string, len(batch))
			for j, ch := range batch {
				texts[j] = ch.Content
			}

			vectors, err := embedder.Embed(ctx, texts)
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("embed %s: %v", slug, err))
				break
			}

			for j := range batch {
				batch[j].Vector = vectors[j]
			}

			if err := indexerInst.Index(ctx, batch); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("index %s: %v", slug, err))
				break
			}

			out.ChunksIndexed += len(batch)
		}

		slog.Debug("indexed", "slug", slug, "chunks", len(kChunks))
	}

	return out, nil
}

func resolveChunker(cfg ChunkingConfig) kbapi.Chunker {
	minSize := 200
	maxSize := 1500
	if cfg.Config != nil {
		if v, ok := cfg.Config["min_size"].(int); ok {
			minSize = v
		}
		if v, ok := cfg.Config["max_size"].(int); ok {
			maxSize = v
		}
	}

	switch cfg.Strategy {
	case "fixed-size":
		overlap := 200
		if cfg.Config != nil {
			if v, ok := cfg.Config["overlap"].(int); ok {
				overlap = v
			}
		}
		return &chunking.FixedSizeChunker{MaxSize: maxSize, Overlap: overlap}
	default:
		return &chunking.HeadingChunker{MinSize: minSize, MaxSize: maxSize}
	}
}

// stripFrontmatter removes YAML frontmatter from markdown content.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return content
	}
	return strings.TrimSpace(content[end+8:]) // skip past closing ---\n
}
