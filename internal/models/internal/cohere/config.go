// SPDX-License-Identifier: Apache-2.0

// Package cohere implements the Cohere LLM provider strategy.
package cohere

import "github.com/katyoti/katyoti-fwk/internal/models/internal/core"

// Config holds Cohere-specific model configuration.
type Config struct {
	core.BaseModelConfig `yaml:",inline"`

	// APIKey is the Cohere API key.
	APIKey string `yaml:"-" envkey:"COHERE_API_KEY"`

	// BaseURL is the Cohere API endpoint.
	BaseURL string `yaml:"base_url,omitempty" desc:"Cohere API base URL"`
}
