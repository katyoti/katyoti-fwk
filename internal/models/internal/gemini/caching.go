// SPDX-License-Identifier: Apache-2.0

// Package googleai provides caching support for Google AI models.

package gemini

import (
	"context"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// CachingHelper provides utilities for working with Google AI's cached content feature.
// Unlike Anthropic which supports inline cache control, Google AI requires
// pre-creating cached content through the API.
type CachingHelper struct {
	client *genai.Client
}

// NewCachingHelper creates a helper for managing cached content.
func NewCachingHelper(ctx context.Context, opts ...Option) (*CachingHelper, error) {
	// Create a GoogleAI client to get access to the underlying genai client
	gai, err := New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &CachingHelper{
		client: gai.client,
	}, nil
}

// CreateCachedContent creates cached content that can be reused across multiple requests.
// This is useful for caching large system prompts, context documents, or frequently used instructions.
//
// Example usage:
//
//	helper, _ := NewCachingHelper(ctx, WithAPIKey(apiKey))
//	cached, _ := helper.CreateCachedContent(ctx, "gemini-2.0-flash", []api.MessageContent{
//	    {
//	        Role: api.ChatMessageTypeSystem,
//	        Parts: []api.ContentPart{
//	            api.TextPart("You are an expert assistant with deep knowledge..."),
//	        },
//	    },
//	}, 1*time.Hour)
//
//	// Use the cached content in requests
//	model, _ := New(ctx, WithAPIKey(apiKey))
//	resp, _ := model.GenerateContent(ctx, messages, WithCachedContent(cached.Name))
func (ch *CachingHelper) CreateCachedContent(
	ctx context.Context,
	modelName string,
	messages []api.MessageContent,
	ttl time.Duration,
) (*genai.CachedContent, error) {
	// Convert messages to genai content
	contents := make([]*genai.Content, 0, len(messages))
	var systemInstruction *genai.Content

	for _, msg := range messages {
		parts := make([]genai.Part, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case api.TextContent:
				parts = append(parts, genai.Text(p.Text))
			case api.CachedContent:
				// Extract the underlying content if it's wrapped with cache control
				// (though Google AI doesn't use inline cache control like Anthropic)
				if textPart, ok := p.ContentPart.(api.TextContent); ok {
					parts = append(parts, genai.Text(textPart.Text))
				}
			}
		}

		content := &genai.Content{
			Parts: parts,
		}

		// Set role
		switch msg.Role {
		case api.ChatMessageTypeSystem:
			content.Role = "system"
			systemInstruction = content
		case api.ChatMessageTypeHuman:
			content.Role = "user"
			contents = append(contents, content)
		case api.ChatMessageTypeAI:
			content.Role = "model"
			contents = append(contents, content)
		}
	}

	// Create the cached content
	cc := &genai.CachedContent{
		Model:             modelName,
		Contents:          contents,
		SystemInstruction: systemInstruction,
		Expiration: genai.ExpireTimeOrTTL{
			TTL: ttl,
		},
	}

	return ch.client.CreateCachedContent(ctx, cc)
}

// GetCachedContent retrieves existing cached content by name.
func (ch *CachingHelper) GetCachedContent(ctx context.Context, name string) (*genai.CachedContent, error) {
	return ch.client.GetCachedContent(ctx, name)
}

// DeleteCachedContent removes cached content.
func (ch *CachingHelper) DeleteCachedContent(ctx context.Context, name string) error {
	return ch.client.DeleteCachedContent(ctx, name)
}

// ListCachedContents returns an iterator for all cached content.
func (ch *CachingHelper) ListCachedContents(ctx context.Context) *genai.CachedContentIterator {
	return ch.client.ListCachedContents(ctx)
}
