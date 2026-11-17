// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/models/api"

	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

type modelType struct{ cfg Config }

func NewModelType(cfg Config) core.ModelType { return &modelType{cfg: cfg} }

func (t *modelType) Name() string         { return t.cfg.ModelID }
func (t *modelType) ProviderName() string { return "ollama" }

func (t *modelType) CreateModel(overrides map[string]any) (core.Model, error) {
	cfg := t.cfg
	if err := core.UnsetKeys(cfg); err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	if v, ok := overrides["server_url"].(string); ok && v != "" {
		cfg.ServerURL = v
	}

	opts := []Option{
		WithModel(cfg.ModelID),
	}
	if cfg.ServerURL != "" {
		opts = append(opts, WithServerURL(cfg.ServerURL))
	}

	llm, err := New(opts...)
	if err != nil {
		return nil, fmt.Errorf("ollama: create client: %w", err)
	}

	var defaults []api.CallOption
	if cfg.MaxTokens > 0 {
		defaults = append(defaults, api.WithMaxTokens(cfg.MaxTokens))
	}
	if cfg.Temperature != nil {
		defaults = append(defaults, api.WithTemperature(float64(*cfg.Temperature)))
	}

	return &core.ModelWrapper{
		LLM:      llm,
		Info_:    core.ModelInfo{Provider: core.ProviderOr(cfg.Provider, "ollama"), ModelID: cfg.ModelID, Reasoning: cfg.Reasoning, CostInputPerMTok: cfg.CostInputPerMTok, CostOutputPerMTok: cfg.CostOutputPerMTok, ContextWindow: cfg.ContextWindow},
		Defaults: defaults,
	}, nil
}

var _ core.ModelType = (*modelType)(nil)
