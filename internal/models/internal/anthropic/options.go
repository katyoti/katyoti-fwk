// SPDX-License-Identifier: Apache-2.0

package anthropic

import "github.com/katyoti/katyoti-fwk/internal/models/api"

// WithPromptCaching enables Anthropic's prompt caching feature.
// This allows frequently-used prompts and system messages to be cached for improved
// performance and reduced costs.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    anthropic.WithPromptCaching(),
//	)
func WithPromptCaching() api.CallOption {
	return func(opts *api.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		opts.Metadata["anthropic:beta_headers"] = []string{"prompt-caching-2024-07-31"}
	}
}

// WithExtendedOutput enables 128K token output for Claude 3.7+.
// Standard models are limited to 8K tokens, but this beta feature allows
// generating much longer responses.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    api.WithMaxTokens(50000),
//	    anthropic.WithExtendedOutput(),
//	)
func WithExtendedOutput() api.CallOption {
	return func(opts *api.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		// Add to existing headers if present
		if existing, ok := opts.Metadata["anthropic:beta_headers"].([]string); ok {
			opts.Metadata["anthropic:beta_headers"] = append(existing, "output-128k-2025-02-19")
		} else {
			opts.Metadata["anthropic:beta_headers"] = []string{"output-128k-2025-02-19"}
		}
	}
}

// WithInterleavedThinking enables thinking between tool calls for Claude 3.7+.
// This allows the model to use reasoning tokens to plan tool usage and interpret results.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    api.WithTools(tools),
//	    api.WithThinkingMode(api.ThinkingModeMedium),
//	    anthropic.WithInterleavedThinking(),
//	)
func WithInterleavedThinking() api.CallOption {
	return func(opts *api.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		// Add to existing headers if present
		if existing, ok := opts.Metadata["anthropic:beta_headers"].([]string); ok {
			opts.Metadata["anthropic:beta_headers"] = append(existing, "interleaved-thinking-2025-05-14")
		} else {
			opts.Metadata["anthropic:beta_headers"] = []string{"interleaved-thinking-2025-05-14"}
		}
	}
}

// WithBetaHeader adds a custom beta header for accessing Anthropic's experimental features.
// This is useful for testing new features before dedicated support is added.
//
// Usage:
//
//	llm.GenerateContent(ctx, messages,
//	    anthropic.WithBetaHeader("new-feature-2025-01-01"),
//	)
func WithBetaHeader(header string) api.CallOption {
	return func(opts *api.CallOptions) {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		// Add to existing headers if present
		if existing, ok := opts.Metadata["anthropic:beta_headers"].([]string); ok {
			opts.Metadata["anthropic:beta_headers"] = append(existing, header)
		} else {
			opts.Metadata["anthropic:beta_headers"] = []string{header}
		}
	}
}

// EphemeralCache creates a standard ephemeral cache control for Anthropic.
func EphemeralCache() *api.CacheControl {
	return &api.CacheControl{
		Type: "ephemeral",
	}
}
