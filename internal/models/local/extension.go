// SPDX-License-Identifier: Apache-2.0

// Package local provides the wiring extension that registers all built-in
// LLM provider strategies and the LLMProviders service.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/anthropic"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/cohere"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/gemini"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/ollama"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/openai"
)

// Extension registers the LLM model domain, built-in provider strategies,
// the config provider for LLMProviders.yaml, and the LLMProviders service.
func Extension(init *application.ApplicationInit) {
	// Register the model domain (3 extension points)
	init.AddExtension(core.ModelDomain.Extension())

	// Contribute built-in provider strategies
	init.AddContribute("llm.providers", ".strategies", func(app *application.Application) error {
		return strategies.ContributeStrategy(app,
			anthropic.Strategy,
			openai.Strategy,
			ollama.Strategy,
			gemini.Strategy,
			cohere.Strategy,
		)
	})

	// Contribute model types from LLMProviders.yaml "models" section
	init.AddContribute("llm.providers", ".config", func(app *application.Application) error {
		cfg, _ := application.AppUseConfig[core.LLMProvidersConfig](app)
		if len(cfg.Models) == 0 {
			return nil
		}

		var items []provider.ConfigItem
		for name, m := range cfg.Models {
			m["name"] = name
			items = append(items, provider.ConfigItem{Data: m, Source: "LLMProviders.yaml"})
		}

		return strategies.ContributeProvider(app, strategies.ProviderDef[core.ModelType]{
			Provider: &staticConfigProvider{domain: "llm.model", items: items},
		})
	})

	// Register LLM providers DI service
	init.AddExtension(core.LLMProvidersExtension)
}

// staticConfigProvider provides pre-loaded config items to the domain.
type staticConfigProvider struct {
	domain string
	items  []provider.ConfigItem
}

func (p *staticConfigProvider) ProvideConfigFor(domain string) ([]provider.ConfigItem, error) {
	if domain != p.domain {
		return nil, nil
	}
	return p.items, nil
}
