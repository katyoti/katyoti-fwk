// SPDX-License-Identifier: Apache-2.0

// Package gemini implements the Google Gemini LLM provider strategy.
// Uses google/generative-ai-go under the hood.
package gemini

import "github.com/katyoti/katyoti-fwk/internal/models/internal/core"

// Config holds Gemini-specific model configuration.
type Config struct {
	core.BaseModelConfig `yaml:",inline"`

	// APIKey is the Google AI API key.
	APIKey string `yaml:"-" envkey:"GOOGLE_API_KEY"`

	// CloudProject is the GCP project ID (for Vertex AI).
	CloudProject string `yaml:"cloud_project,omitempty" desc:"GCP project ID (for Vertex AI)"`

	// CloudLocation is the GCP region (for Vertex AI).
	CloudLocation string `yaml:"cloud_location,omitempty" desc:"GCP region (for Vertex AI)"`
}
