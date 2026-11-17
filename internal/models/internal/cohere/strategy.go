// SPDX-License-Identifier: Apache-2.0

package cohere

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/core"
)

var Strategy = strategies.StrategyDef[core.ModelType]{
	DefMeta: strategies.DefMeta{Name: "cohere",
		Label: "Cohere"},
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
	return strategies.TypeDef[core.ModelType]{DefMeta: strategies.DefMeta{Name: name}, Strategy: "cohere", Impl: raw}
}
