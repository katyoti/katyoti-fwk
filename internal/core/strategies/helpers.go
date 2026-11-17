// SPDX-License-Identifier: Apache-2.0

package strategies

import "context"

// --- Level 1 helpers: Strategy ---

// FuncStrategy creates a StrategyDef from a direct creation function.
// Use when the strategy doesn't need config or deps from YAML — the function
// IS the type creator, called directly with no arguments.
func FuncStrategy[TType any](name, label string, fn func() (TType, error)) StrategyDef[TType] {
	return StrategyDef[TType]{
		DefMeta:  DefMeta{Name: name, Label: label},
		Strategy: &funcStrategy[TType]{name: name, fn: fn},
	}
}

// funcStrategy implements Strategy using a direct function.
type funcStrategy[TType any] struct {
	name string
	fn   func() (TType, error)
}

func (s *funcStrategy[TType]) NewTypeConfig() (any, func(context.Context) (TType, error)) {
	return nil, func(_ context.Context) (TType, error) { return s.fn() }
}

func (s *funcStrategy[TType]) CreateTypeDef(name string, raw TType) TypeDef[TType] {
	return TypeDef[TType]{
		DefMeta:  DefMeta{Name: name},
		Strategy: s.name,
		Impl:     raw,
	}
}
