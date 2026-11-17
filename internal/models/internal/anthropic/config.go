// SPDX-License-Identifier: Apache-2.0

// Package anthropic implements the Anthropic (Claude) LLM provider strategy.
package anthropic

import (
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

// Config holds Anthropic-specific model configuration.
// Embeds BaseModelConfig for shared fields (model_id, max_tokens, temperature, etc.).
// Extends with Claude-specific features (thinking, auto-cache, top_k).
type Config struct {
	core.BaseModelConfig `yaml:",inline"`

	// TopK controls diversity by limiting the top K tokens to sample from.
	// Anthropic-specific (not available on all providers).
	// Optional.
	TopK *int32 `yaml:"top_k,omitempty" desc:"Top-K sampling (Anthropic-specific)"`

	// Thinking enables Claude's extended thinking feature.
	// When enabled, Claude can use a "thinking" step before responding.
	// Optional — nil disables thinking.
	Thinking *ThinkingConfig `yaml:"thinking,omitempty" desc:"Extended thinking (Claude-specific)"`

	// AutoCache enables automatic prompt caching for tools and system prompts.
	// Sets cache_control={"type":"ephemeral"} on tool definitions and system blocks.
	// Reduces cost on repeated tool calls within a session.
	AutoCache bool `yaml:"auto_cache" desc:"Auto-cache tools and system prompt" default:"true"`

	// DisableParallelToolUse disables parallel tool execution.
	// When true, Claude will call tools one at a time.
	// Optional.
	DisableParallelToolUse *bool `yaml:"disable_parallel_tool_use,omitempty" desc:"Disable parallel tool execution"`

	// APIKey is the Anthropic API key. Default: ANTHROPIC_API_KEY env var.
	// Can be overridden per model in YAML for Anthropic-compatible APIs
	// (e.g. Azure AI Foundry's /anthropic endpoint) that use a different
	// key — typically via variable expansion: `api_key: "${{env:MY_KEY}}"`.
	// YAML wins; the envkey only fills when no per-model value is set.
	APIKey string `yaml:"api_key,omitempty" envkey:"ANTHROPIC_API_KEY"`

	// BaseURL is the Anthropic API endpoint.
	// Leave empty for the SDK default (https://api.anthropic.com).
	BaseURL string `yaml:"base_url,omitempty" desc:"Anthropic API base URL"`

	// AdditionalHeaders are extra HTTP headers sent with every request.
	AdditionalHeaders map[string]string `yaml:"additional_headers,omitempty" desc:"Extra HTTP headers"`
}

// ThinkingConfig configures Claude's extended thinking feature.
type ThinkingConfig struct {
	// Enabled activates the thinking feature.
	Enabled bool `yaml:"enabled" desc:"Enable extended thinking" default:"false"`

	// BudgetTokens is the token budget allocated for the thinking step.
	BudgetTokens int `yaml:"budget_tokens" desc:"Token budget for thinking" default:"4096"`
}
