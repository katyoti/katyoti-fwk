// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// Collection metadata keys recording what an index was embedded with.
const (
	metaEmbeddingModel      = "embedding_model"
	metaEmbeddingDimensions = "embedding_dimensions"
	metaEmbeddingProfile    = "embedding_profile"
)

// embedderFor returns the embedder of the profile the knowledge base names
// (empty: the EmbeddingProfiles default).
func (s *kbService) embedderFor(ctx context.Context, kbCfg *KnowledgeBaseConfig) (kbapi.Embedder, error) {
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("embedding profiles: %w", err)
	}
	return profiles.Embedder(kbCfg.Indexing.Embedding.Profile)
}

// embeddingMeta is what an index records about its embeddings.
func embeddingMeta(e kbapi.Embedder) map[string]string {
	return map[string]string{
		metaEmbeddingModel:      e.Model(),
		metaEmbeddingDimensions: strconv.Itoa(e.Dimension()),
		metaEmbeddingProfile:    e.Name(),
	}
}

// checkCompatible refuses an embedder whose model or dimensions differ from
// those the index was built with. The provider is not compared: the same
// model and dimensions from another endpoint (OpenAI, Foundry) give the same
// vectors. An index that recorded no dimensions (older indexes, which stored
// "openai/<model>") is compared on its model only; an index that recorded
// nothing is accepted.
func checkCompatible(meta map[string]string, e kbapi.Embedder) error {
	model, ok := meta[metaEmbeddingModel]
	if !ok || model == "" {
		return nil
	}
	dims, hasDims := meta[metaEmbeddingDimensions]
	if !hasDims {
		// Older index: "openai/<model>", the model empty when the KB named none.
		if i := strings.Index(model, "/"); i >= 0 {
			model = model[i+1:]
		}
		if model == "" || model == "%!s(<nil>)" {
			model = "text-embedding-3-small"
		}
	}
	mismatch := model != e.Model()
	if hasDims && dims != strconv.Itoa(e.Dimension()) {
		mismatch = true
	}
	if !mismatch {
		return nil
	}
	built := model
	if hasDims {
		built += "/" + dims
	}
	return fmt.Errorf("the index was built with %s and embedding profile %q gives %s/%d: re-index the knowledge base (index --force)",
		built, e.Name(), e.Model(), e.Dimension())
}

// dropper is a KnowledgeIndexer that can drop a collection (force re-index).
type dropper interface {
	Drop(ctx context.Context, collection string) error
}
