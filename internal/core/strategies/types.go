// SPDX-License-Identifier: Apache-2.0

// Package strategies provides a toolkit for the three-level strategy pattern
// (Strategy → Type → Instance) using the Extension Manager (EM).
//
// # DefMeta
//
// DefMeta holds display metadata shared by all levels: name, label, description,
// icons (terminal + GUI), hashtags, category. Embedded by StrategyDef and TypeDef.
//
// # Level 1: Strategy
//
// A Strategy knows how to create Types from config + context-based DI.
//
//	StrategyDef  — registration wrapper (DefMeta + Strategy impl)
//	Strategy     — behavior interface: NewTypeConfig() + CreateTypeDef()
//
// # Level 2: Type
//
// A Type is a configured factory produced by a Strategy from config.
//
//	TypeDef  — registration wrapper (DefMeta + Type + Config + Impl)
//
// # Level 3: Instance
//
// An Instance is a runtime object produced from a Type. Level 3 is
// domain-specific and not defined in this package.
package strategies

import (
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// DefMeta holds display metadata common to all strategy/type definitions.
// Embedded by both StrategyDef and TypeDef for consistent GUI/CLI rendering.
type DefMeta struct {
	// Name is the unique identifier (e.g., "agent-loop", "hello-world").
	Name string

	// Label is a human-readable display name (e.g., "Agent Loop", "Hello World").
	Label string

	// Description is a short description for tooltips and list views.
	Description string

	// Icon is a Unicode/emoji icon for terminal rendering (e.g., "🤖", "👋").
	Icon string

	// IconName is a Tabler Icon name for GUI rendering (e.g., "robot", "hand-wave").
	// See https://tabler.io/icons
	IconName string

	// Hashtags are categorization tags for filtering and search.
	Hashtags []string

	// Category is the primary grouping category (e.g., "execution", "analysis").
	Category string
}

// Strategy defines Level 1 behavior: how to create Types from config + deps.
//
// Embeds services.TypeFactory for the tuple/closure pattern (NewTypeConfig).
// createFn receives context.Context for dependency resolution via UseService[T](ctx).
// Adds CreateTypeDef to wrap raw types with strategy-provided metadata.
type Strategy[TType any] interface {
	services.TypeFactory[TType]

	// CreateTypeDef wraps a raw TType with strategy-provided metadata
	// into a TypeDef.
	CreateTypeDef(name string, raw TType) TypeDef[TType]
}

// StrategyDef is the Level 1 registration unit: metadata + Strategy impl.
// Stored in an IndexedPoint keyed by Name.
type StrategyDef[TType any] struct {
	DefMeta

	// Strategy implements the Level 1 behavior for this strategy.
	Strategy Strategy[TType]
}

// TypeDef is the Level 2 registration unit stored in the Types point.
// Contains display metadata + strategy reference + config + the created domain type.
type TypeDef[TType any] struct {
	DefMeta

	// Strategy is the name of the strategy that creates this type (e.g., "agent-loop", "mcp").
	// Links to a StrategyDef by name.
	Strategy string

	// Config is the strategy-specific configuration as raw JSON.
	// Decoded by the strategy's TypeFactory when creating Impl.
	Config json.RawMessage

	// Impl is the created domain type (e.g., MachineFactory, PluginFactory).
	// Filled after the strategy's TypeFactory creates it from Config.
	Impl TType
}

// ProviderDef wraps a provider.ConfigProvider for EM routing.
type ProviderDef[TType any] struct {
	Provider provider.ConfigProvider
}
