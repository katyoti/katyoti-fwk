// SPDX-License-Identifier: Apache-2.0

package cohere

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/models/api"

	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

type modelType struct{ cfg Config }

func NewModelType(cfg Config) core.ModelType { return &modelType{cfg: cfg} }

func (t *modelType) Name() string         { return t.cfg.ModelID }
func (t *modelType) ProviderName() string { return "cohere" }

func (t *modelType) CreateModel(overrides map[string]any) (core.Model, error) {
	cfg := t.cfg
	if err := core.UnsetKeys(cfg); err != nil {
		return nil, fmt.Errorf("cohere: %w", err)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("cohere: COHERE_API_KEY is required")
	}

	opts := []Option{
		WithToken(cfg.APIKey),
		WithModel(cfg.ModelID),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, WithBaseURL(cfg.BaseURL))
	}

	llm, err := New(opts...)
	if err != nil {
		return nil, fmt.Errorf("cohere: create client: %w", err)
	}

	var defaults []api.CallOption
	defaults = append(defaults, api.WithMaxTokens(cfg.MaxTokens))
	if cfg.Temperature != nil {
		defaults = append(defaults, api.WithTemperature(float64(*cfg.Temperature)))
	}
	if cfg.TopP != nil {
		defaults = append(defaults, api.WithTopP(float64(*cfg.TopP)))
	}

	return &core.ModelWrapper{
		LLM:      llm,
		Info_:    core.ModelInfo{Provider: core.ProviderOr(cfg.Provider, "cohere"), ModelID: cfg.ModelID, Reasoning: cfg.Reasoning, CostInputPerMTok: cfg.CostInputPerMTok, CostOutputPerMTok: cfg.CostOutputPerMTok, ContextWindow: cfg.ContextWindow},
		Defaults: defaults,
	}, nil
}

var _ core.ModelType = (*modelType)(nil)
