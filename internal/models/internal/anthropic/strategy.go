// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

// Strategy is the "anthropic" provider strategy definition.
var Strategy = strategies.StrategyDef[core.ModelType]{
	DefMeta: strategies.DefMeta{Name: "anthropic",
		Label: "Anthropic (Claude)"},
	Strategy: &strategyImpl{},
}

type strategyImpl struct{}

func (s *strategyImpl) NewTypeConfig() (any, func(context.Context) (core.ModelType, error)) {
	cfg := &Config{}
	// APIKey filled by envkey:"ANTHROPIC_API_KEY" tag via ProcessConfig
	// BaseURL filled by default:"https://api.anthropic.com" tag via ProcessConfig
	return cfg, func(_ context.Context) (core.ModelType, error) {
		return NewModelType(*cfg), nil
	}
}

func (s *strategyImpl) CreateTypeDef(name string, raw core.ModelType) strategies.TypeDef[core.ModelType] {
	return strategies.TypeDef[core.ModelType]{
		DefMeta:  strategies.DefMeta{Name: name},
		Strategy: "anthropic",
		Impl:     raw,
	}
}
