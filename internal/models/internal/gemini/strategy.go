// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

var Strategy = strategies.StrategyDef[core.ModelType]{
	DefMeta: strategies.DefMeta{Name: "gemini",
		Label: "Google Gemini"},
	Strategy: &strategyImpl{},
}

type strategyImpl struct{}

func (s *strategyImpl) NewTypeConfig() (any, func(context.Context) (core.ModelType, error)) {
	cfg := &Config{}
	return cfg, func(_ context.Context) (core.ModelType, error) {
		return NewModelType(*cfg), nil
	}
}

func (s *strategyImpl) CreateTypeDef(name string, raw core.ModelType) strategies.TypeDef[core.ModelType] {
	return strategies.TypeDef[core.ModelType]{DefMeta: strategies.DefMeta{Name: name}, Strategy: "gemini", Impl: raw}
}
