// SPDX-License-Identifier: Apache-2.0

// Package core provides base interfaces and shared configuration for LLM providers.
package core

// ProviderOr returns the configured display-name provider when set, else the
// adapter's protocol fallback. Adapters use it to honor a model's explicit
// `provider:` (e.g. foundry-anthropic) without hardcoding "anthropic".
func ProviderOr(configured, fallback string) string {
	if configured != "" {
		return configured
	}
	return fallback
}

// BaseModelConfig holds configuration fields common to all LLM providers.
// Provider-specific configs embed this and add their own fields.
type BaseModelConfig struct {
	// ModelID is the model identifier sent to the provider API.
	// e.g., "claude-opus-4-6", "gpt-5.4", "llama3.3"
	// Required.
	ModelID string `yaml:"model_id" desc:"Model identifier" required:"true"`

	// Provider is the DISPLAY name of who actually serves the model — distinct
	// from `type` (the protocol/adapter class). e.g. a model with
	// `type: anthropic` served through MS Foundry sets `provider: foundry-anthropic`
	// so the UI doesn't misleadingly label it plain "anthropic". When empty,
	// adapters fall back to their protocol name.
	Provider string `yaml:"provider,omitempty" desc:"Display name of the serving provider (e.g. foundry-anthropic)"`

	// MaxTokens limits the maximum number of tokens in the response.
	MaxTokens int `yaml:"max_tokens" desc:"Max output tokens per call" default:"4096"`

	// ContextWindow is the total context size (input + output).
	// Informational — used by profile selection and docs, not sent to API.
	ContextWindow int `yaml:"context_window,omitempty" desc:"Total context window (tokens)"`

	// MaxOutput is the model's hard max output limit.
	// Informational — MaxTokens is capped to this.
	MaxOutput int `yaml:"max_output,omitempty" desc:"Model's max output tokens"`

	// CostInputPerMTok is the cost per million input tokens (USD).
	// Informational — used by routing and cost reports.
	CostInputPerMTok float64 `yaml:"cost_input,omitempty" desc:"$/MTok input"`

	// CostOutputPerMTok is the cost per million output tokens (USD).
	CostOutputPerMTok float64 `yaml:"cost_output,omitempty" desc:"$/MTok output"`

	// CostCacheReadPerMTok / CostCacheWritePerMTok are the cache pricing
	// components ($/MTok). Providers price cache distinctly: Anthropic charges
	// 0.10× input for reads and 1.25×/2× input for writes (5-min/1-hr TTL);
	// OpenAI discounts reads (~0.25–0.5×) and charges nothing to write. When
	// left 0, ModelInfo.ComputeCost derives defaults from input cost
	// (read = 0.10× input, write = 1.25× input — the Anthropic convention).
	CostCacheReadPerMTok  float64 `yaml:"cost_cache_read,omitempty" desc:"$/MTok cache read"`
	CostCacheWritePerMTok float64 `yaml:"cost_cache_write,omitempty" desc:"$/MTok cache write"`

	// Temperature controls randomness in responses.
	// Range: [0.0, 1.0], where 0.0 is more focused and 1.0 is more creative.
	Temperature *float32 `yaml:"temperature,omitempty" desc:"Sampling temperature (0.0-1.0)"`

	// TopP controls diversity via nucleus sampling.
	TopP *float32 `yaml:"top_p,omitempty" desc:"Top-p / nucleus sampling (0.0-1.0)"`

	// StopSequences specifies custom stop sequences.
	StopSequences []string `yaml:"stop_sequences,omitempty" desc:"Custom stop sequences"`

	// Reasoning indicates the model supports extended thinking/reasoning.
	// Set in LLMProviders.yaml — no hardcoded model name detection needed.
	Reasoning bool `yaml:"reasoning,omitempty" desc:"Model supports reasoning/thinking tokens"`
}
