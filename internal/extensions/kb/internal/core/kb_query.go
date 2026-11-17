// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/indexing"
)

func (s *kbService) Query(ctx context.Context, in kbapi.QueryInput) (kbapi.QueryOutput, error) {
	if in.Path == "" {
		return kbapi.QueryOutput{}, fmt.Errorf("path is required")
	}
	if in.Query == "" {
		return kbapi.QueryOutput{}, fmt.Errorf("query is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	kbCfg, err := LoadKnowledgeBase(absPath)
	if err != nil {
		return kbapi.QueryOutput{}, err
	}

	// The embedder of the profile the knowledge base names (to embed the query)
	embedder, err := s.embedderFor(ctx, kbCfg)
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("init embedder: %w", err)
	}

	// Initialize indexer
	indexCfg := kbCfg.Indexing.Config
	if indexCfg == nil {
		indexCfg = map[string]any{}
	}
	if _, ok := indexCfg["path"]; !ok {
		indexCfg["path"] = indexing.StoragePath(absPath)
	}
	indexerInst, err := indexing.ResolveIndexer(kbCfg.Indexing.Backend, indexCfg)
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("init indexer: %w", err)
	}

	collection := kbCfg.Name
	if col, ok := kbCfg.Indexing.Config["collection"].(string); ok && col != "" {
		collection = col
	}

	// The index must have been built with the profile's model and dimensions
	meta, err := indexerInst.CollectionMeta(ctx, collection)
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("read collection metadata: %w", err)
	}
	if err := checkCompatible(meta, embedder); err != nil {
		return kbapi.QueryOutput{}, err
	}

	// Init collection (read-only — just resolves the ID)
	if err := indexerInst.Init(ctx, collection, embedder.Dimension(), nil); err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("init collection: %w", err)
	}

	// Embed the query
	vectors, err := embedder.Embed(ctx, []string{in.Query})
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("embed query: %w", err)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 5
	}

	// Search
	results, err := indexerInst.Query(ctx, kbapi.QueryRequest{
		Vector: vectors[0],
		Limit:  limit,
		Filter: in.Filter,
	})
	if err != nil {
		return kbapi.QueryOutput{}, fmt.Errorf("query: %w", err)
	}

	return kbapi.QueryOutput{Results: results}, nil
}
