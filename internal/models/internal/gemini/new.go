// SPDX-License-Identifier: Apache-2.0

// package gemini implements a provider for Google AI (Gemini) LLMs.

package gemini

import (
	"context"

	"github.com/google/generative-ai-go/genai"
)

// GoogleAI is a type that represents a Google AI API client.
type GoogleAI struct {
	client *genai.Client
	opts   Options
}

// New creates a new GoogleAI client.
func New(ctx context.Context, opts ...Option) (*GoogleAI, error) {
	clientOptions := DefaultOptions()
	for _, opt := range opts {
		opt(&clientOptions)
	}
	clientOptions.EnsureAuthPresent()

	gi := &GoogleAI{
		opts: clientOptions,
	}

	client, err := genai.NewClient(ctx, clientOptions.ClientOptions...)
	if err != nil {
		return gi, err
	}

	gi.client = client
	return gi, nil
}

// Close closes the underlying genai client.
func (g *GoogleAI) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
