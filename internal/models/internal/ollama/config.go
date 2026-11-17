// SPDX-License-Identifier: Apache-2.0

// Package ollama implements the Ollama (local) LLM provider strategy.
package ollama

import "github.com/katyoti/katyoti-fwk/internal/models/internal/core"

// Config holds Ollama-specific model configuration.
type Config struct {
	core.BaseModelConfig `yaml:",inline"`

	// ServerURL is the Ollama server endpoint.
	// Default: http://localhost:11434
	ServerURL string `yaml:"server_url" default:"http://localhost:11434" desc:"Ollama server URL"`
}
