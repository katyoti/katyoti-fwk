// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/models/api"

	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

type modelType struct{ cfg Config }

func NewModelType(cfg Config) core.ModelType { return &modelType{cfg: cfg} }

func (t *modelType) Name() string         { return t.cfg.ModelID }
func (t *modelType) ProviderName() string { return "gemini" }

func (t *modelType) CreateModel(overrides map[string]any) (core.Model, error) {
	cfg := t.cfg
	if err := core.UnsetKeys(cfg); err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: GOOGLE_API_KEY is required")
	}

	opts := []Option{
		WithAPIKey(cfg.APIKey),
		WithDefaultModel(cfg.ModelID),
	}
	if cfg.MaxTokens > 0 {
		opts = append(opts, WithDefaultMaxTokens(cfg.MaxTokens))
	}
	if cfg.CloudProject != "" {
		opts = append(opts, WithCloudProject(cfg.CloudProject))
	}
	if cfg.CloudLocation != "" {
		opts = append(opts, WithCloudLocation(cfg.CloudLocation))
	}

	// googleai.New requires a context (for credential resolution)
	llm, err := New(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	var defaults []api.CallOption
	if cfg.MaxTokens > 0 {
		defaults = append(defaults, api.WithMaxTokens(cfg.MaxTokens))
	}
	if cfg.Temperature != nil {
		defaults = append(defaults, api.WithTemperature(float64(*cfg.Temperature)))
	}
	if cfg.TopP != nil {
		defaults = append(defaults, api.WithTopP(float64(*cfg.TopP)))
	}

	return &core.ModelWrapper{
		LLM:      llm,
		Info_:    core.ModelInfo{Provider: core.ProviderOr(cfg.Provider, "gemini"), ModelID: cfg.ModelID, Reasoning: cfg.Reasoning, CostInputPerMTok: cfg.CostInputPerMTok, CostOutputPerMTok: cfg.CostOutputPerMTok, ContextWindow: cfg.ContextWindow},
		Defaults: defaults,
	}, nil
}

var _ core.ModelType = (*modelType)(nil)
