// SPDX-License-Identifier: Apache-2.0

package core

import "github.com/katyoti/katyoti-fwk/internal/core/strategies"

// ModelDomain is the strategy domain for LLM model types.
// Strategy discriminator field: "type" (e.g., type: anthropic)
var ModelDomain = strategies.NewDomain[ModelType]("llm.model")

// LLMProvidersConfig is the top-level config for the model domain.
// Config file: LLMProviders.yaml
type LLMProvidersConfig struct {
	DefaultModel string                    `yaml:"default_model" default:"claude-sonnet" desc:"Default model type name"`
	Models       map[string]map[string]any `yaml:"models" desc:"Model type definitions (name → provider-specific config)"`
}
