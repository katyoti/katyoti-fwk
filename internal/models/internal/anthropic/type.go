// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

type modelType struct {
	cfg Config
}

func NewModelType(cfg Config) core.ModelType {
	return &modelType{cfg: cfg}
}

func (t *modelType) Name() string         { return t.cfg.ModelID }
func (t *modelType) ProviderName() string { return "anthropic" }

func (t *modelType) CreateModel(overrides map[string]any) (core.Model, error) {
	cfg := t.cfg
	applyOverrides(&cfg, overrides)
	if err := core.UnsetKeys(cfg); err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: API key is required (set api_key in the model config or the ANTHROPIC_API_KEY env var)")
	}

	var sdkOpts []option.RequestOption
	if cfg.BaseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(cfg.BaseURL))
	}

	llm, err := New(cfg.APIKey, cfg.ModelID, cfg.Reasoning, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("anthropic: create client: %w", err)
	}

	var defaults []api.CallOption
	defaults = append(defaults, api.WithMaxTokens(cfg.MaxTokens))
	if cfg.Temperature != nil {
		defaults = append(defaults, api.WithTemperature(float64(*cfg.Temperature)))
	}
	if cfg.TopP != nil {
		defaults = append(defaults, api.WithTopP(float64(*cfg.TopP)))
	}
	if cfg.AutoCache {
		defaults = append(defaults, WithPromptCaching())
	}
	if cfg.Thinking != nil && cfg.Thinking.Enabled {
		defaults = append(defaults, api.WithInterleaveThinking(true))
		if cfg.Thinking.BudgetTokens > 0 {
			defaults = append(defaults, api.WithThinkingBudget(cfg.Thinking.BudgetTokens))
		}
	}

	return &core.ModelWrapper{
		LLM:      llm,
		Info_:    core.ModelInfo{Provider: core.ProviderOr(cfg.Provider, "anthropic"), ModelID: cfg.ModelID, Reasoning: cfg.Reasoning, CostInputPerMTok: cfg.CostInputPerMTok, CostOutputPerMTok: cfg.CostOutputPerMTok, CostCacheReadPerMTok: cfg.CostCacheReadPerMTok, CostCacheWritePerMTok: cfg.CostCacheWritePerMTok, ContextWindow: cfg.ContextWindow},
		Defaults: defaults,
	}, nil
}

func applyOverrides(cfg *Config, overrides map[string]any) {
	if overrides == nil {
		return
	}
	if v, ok := overrides["max_tokens"]; ok {
		if n, ok := v.(float64); ok {
			cfg.MaxTokens = int(n)
		}
	}
	if v, ok := overrides["thinking"]; ok {
		if m, ok := v.(map[string]any); ok {
			if cfg.Thinking == nil {
				cfg.Thinking = &ThinkingConfig{}
			}
			if enabled, ok := m["enabled"].(bool); ok {
				cfg.Thinking.Enabled = enabled
			}
			if budget, ok := m["budget_tokens"].(float64); ok {
				cfg.Thinking.BudgetTokens = int(budget)
			}
		}
	}
	if v, ok := overrides["auto_cache"].(bool); ok {
		cfg.AutoCache = v
	}
}

var _ core.ModelType = (*modelType)(nil)
