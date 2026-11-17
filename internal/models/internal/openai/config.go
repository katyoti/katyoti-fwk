// SPDX-License-Identifier: Apache-2.0

// Package openai implements the OpenAI (GPT) LLM provider strategy.
package openai

import (
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

// Config holds OpenAI-specific model configuration.
type Config struct {
	core.BaseModelConfig `yaml:",inline"`

	// ReasoningEffort controls reasoning depth for o1/o3 models.
	// Values: "low", "medium", "high".
	// Optional — only applies to reasoning models.
	ReasoningEffort string `yaml:"reasoning_effort,omitempty" desc:"Reasoning effort for o1/o3 (low/medium/high)"`

	// FrequencyPenalty penalizes tokens based on their existing frequency in the text.
	// Range: [-2.0, 2.0].
	// Optional.
	FrequencyPenalty *float32 `yaml:"frequency_penalty,omitempty" desc:"Frequency penalty (-2.0 to 2.0)"`

	// PresencePenalty penalizes tokens based on whether they appear in the text so far.
	// Range: [-2.0, 2.0].
	// Optional.
	PresencePenalty *float32 `yaml:"presence_penalty,omitempty" desc:"Presence penalty (-2.0 to 2.0)"`

	// APIKey is the API key. Default: OPENAI_API_KEY env var.
	// Can be overridden per model in YAML for OpenAI-compatible APIs
	// (e.g. Mistral, Together, Groq) that use a different key.
	APIKey string `yaml:"api_key,omitempty" envkey:"OPENAI_API_KEY"`

	// BaseURL overrides the API endpoint.
	// Leave empty for OpenAI default. Set for compatible APIs:
	//   Mistral: https://api.mistral.ai/v1
	//   Together: https://api.together.xyz/v1
	BaseURL string `yaml:"base_url,omitempty" desc:"API base URL"`
}
