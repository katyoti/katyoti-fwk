// SPDX-License-Identifier: Apache-2.0

package openai

import (
	"fmt"

	"github.com/openai/openai-go/v2/option"

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
func (t *modelType) ProviderName() string { return "openai" }

func (t *modelType) CreateModel(overrides map[string]any) (core.Model, error) {
	cfg := t.cfg
	applyOverrides(&cfg, overrides)
	if err := core.UnsetKeys(cfg); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: OPENAI_API_KEY is required")
	}

	var sdkOpts []option.RequestOption
	if cfg.BaseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(cfg.BaseURL))
	}

	llm, err := New(cfg.APIKey, cfg.ModelID, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("openai: create client: %w", err)
	}

	var defaults []api.CallOption
	defaults = append(defaults, api.WithMaxTokens(cfg.MaxTokens))

	// Non-OpenAI compatible APIs (Mistral, Together, etc.) don't support
	// max_completion_tokens — force legacy max_tokens field.
	if cfg.BaseURL != "" {
		defaults = append(defaults, api.WithMetadata(map[string]interface{}{
			"openai:use_legacy_max_tokens": true,
		}))
	}
	if cfg.Temperature != nil {
		defaults = append(defaults, api.WithTemperature(float64(*cfg.Temperature)))
	}
	if cfg.TopP != nil {
		defaults = append(defaults, api.WithTopP(float64(*cfg.TopP)))
	}
	if cfg.FrequencyPenalty != nil {
		defaults = append(defaults, api.WithFrequencyPenalty(float64(*cfg.FrequencyPenalty)))
	}
	if cfg.PresencePenalty != nil {
		defaults = append(defaults, api.WithPresencePenalty(float64(*cfg.PresencePenalty)))
	}

	return &core.ModelWrapper{
		LLM:      llm,
		Info_:    core.ModelInfo{Provider: core.ProviderOr(cfg.Provider, "openai"), ModelID: cfg.ModelID, Reasoning: cfg.Reasoning, CostInputPerMTok: cfg.CostInputPerMTok, CostOutputPerMTok: cfg.CostOutputPerMTok, ContextWindow: cfg.ContextWindow},
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
	if v, ok := overrides["temperature"]; ok {
		if f, ok := v.(float64); ok {
			f32 := float32(f)
			cfg.Temperature = &f32
		}
	}
}

var _ core.ModelType = (*modelType)(nil)
