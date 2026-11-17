// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// modelManager implements LLMProviders using the strategy domain.
type modelManager struct {
	domain *strategies.DeclaredDomain[ModelType]
	cfg    *LLMProvidersConfig
}

// LLMProvidersExtension registers LLMProviders as a DI service.
var LLMProvidersExtension = application.DeclareService[LLMProviders, LLMProvidersConfig](
	func(_ context.Context, cfg *LLMProvidersConfig) (LLMProviders, error) {
		return &modelManager{domain: ModelDomain, cfg: cfg}, nil
	},
)

func (m *modelManager) GetModelType(name string) (ModelType, error) {
	td := m.domain.GetTypeDef(name)
	if td.Impl == nil {
		return nil, fmt.Errorf("model type %q not found. Available: %v", name, m.domain.Types.Keys())
	}
	return td.Impl, nil
}

func (m *modelManager) CreateModel(name string, overrides map[string]any) (Model, error) {
	mt, err := m.GetModelType(name)
	if err != nil {
		return nil, err
	}
	return mt.CreateModel(overrides)
}

func (m *modelManager) DefaultModelName() string {
	return m.cfg.DefaultModel
}

func (m *modelManager) ListModelTypes() []string {
	return m.domain.Types.Keys()
}
