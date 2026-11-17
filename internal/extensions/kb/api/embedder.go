// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// Embedder generates vector embeddings from text chunks.
type Embedder interface {
	// Name is the embedding profile the embedder serves.
	Name() string
	// Model is the model the profile sends (the deployment name on Foundry
	// and Azure OpenAI).
	Model() string
	// Dimension is the profile's declared vector size.
	Dimension() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
