// SPDX-License-Identifier: Apache-2.0

package strategies

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// DomainOption configures a DeclaredDomain at creation time.
type DomainOption func(*domainConfig)

type domainConfig struct {
	typeFieldName string
}

// WithTypeFieldName sets the YAML discriminator key for Level 1 (Strategy→Type).
// Defaults to "type".
func WithTypeFieldName(name string) DomainOption {
	return func(c *domainConfig) { c.typeFieldName = name }
}

// DeclaredDomain holds the three extension points for a strategy domain
// and provides accessors, registries, and name computation helpers.
//
// Type parameter TType is what strategies produce (Level 1 output, e.g., PluginFactory).
// Stored wrapped in TypeDef[TType] in the Types point.
//
// Level 2 (Type → Instance) is domain-specific and not managed here.
// Each domain defines its own instantiation pattern on TType.
type DeclaredDomain[TType any] struct {
	// Name is the domain identifier (e.g., "agent.plugin", "task").
	Name string

	// Strategies holds contributed StrategyDef entries, keyed by Name.
	Strategies *em.IndexedPoint[StrategyDef[TType]]

	// Types holds contributed TypeDef[TType] entries, keyed by Name.
	Types *em.IndexedPoint[TypeDef[TType]]

	// Providers holds contributed ProviderDefs for this domain.
	Providers *em.ListPoint[ProviderDef[TType]]

	typeFieldName string
}

// NewDomain creates a DeclaredDomain with its three extension points.
// The domain is not yet registered with any EM — call Register() for that.
func NewDomain[TType any](
	name string,
	opts ...DomainOption,
) *DeclaredDomain[TType] {
	cfg := &domainConfig{
		typeFieldName: "type",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &DeclaredDomain[TType]{
		Name: name,
		Strategies: em.NewIndexedPoint[StrategyDef[TType]](
			func(sd StrategyDef[TType]) string { return sd.Name },
		),
		Types: em.NewIndexedPoint[TypeDef[TType]](
			func(td TypeDef[TType]) string { return td.Name },
		),
		Providers:     em.NewListPoint[ProviderDef[TType]](),
		typeFieldName: cfg.typeFieldName,
	}
}

// --- Strategy access ---

// GetStrategyDef returns the full StrategyDef by name. Zero value if not found.
func (d *DeclaredDomain[TType]) GetStrategyDef(name string) StrategyDef[TType] {
	return d.Strategies.Get(name)
}

// GetStrategy returns the Strategy by name. Nil if not found.
func (d *DeclaredDomain[TType]) GetStrategy(name string) Strategy[TType] {
	return d.Strategies.Get(name).Strategy
}

// --- Type access ---

// GetTypeDef returns the full TypeDef stored under the given key. Zero value if not found.
func (d *DeclaredDomain[TType]) GetTypeDef(name string) TypeDef[TType] {
	return d.Types.Get(name)
}

// GetType returns the raw TType for the given key. Zero value if not found.
func (d *DeclaredDomain[TType]) GetType(name string) TType {
	return d.Types.Get(name).Impl
}

// HasType returns true if the given key exists in the types point.
func (d *DeclaredDomain[TType]) HasType(name string) bool {
	return d.Types.Has(name)
}

// --- Creation ---

// CreateType looks up a strategy by typeFieldName in the config map,
// decodes the config (env var expansion + ProcessConfig), and calls
// createFn(ctx) — deps resolved via UseService[T](ctx).
func (d *DeclaredDomain[TType]) CreateType(ctx context.Context, data map[string]any) (TType, error) {
	result, err := services.CreateFromMap(ctx, d.AsTypeFactoryLookup(), data, d.typeFieldName)
	if err != nil {
		var zero TType
		return zero, err
	}
	return result.Value, nil
}

// AsTypeFactoryLookup returns the domain's strategies as a services.TypeFactoryLookup.
func (d *DeclaredDomain[TType]) AsTypeFactoryLookup() services.TypeFactoryLookup[TType] {
	return services.AsTypeFactoryLookup(d.Strategies, strategyExtractor[TType])
}

// --- Lifecycle methods ---

// Register registers a domain's three extension points with the Application.
// Must be called during Phase 1 (RegisterPoints).
func (d *DeclaredDomain[TType]) register(app *application.Application) error {
	if err := application.RegisterPoint(app, d.Strategies); err != nil {
		return err
	}
	if err := application.RegisterPoint(app, d.Types); err != nil {
		return err
	}
	return application.RegisterPoint(app, d.Providers)
}

// registerDirectTypes creates a 1:1 type for each contributed strategy that
// doesn't already have a matching type (i.e., not created by a provider).
// Call after registerByProvider to fill in remaining strategies.
func (d *DeclaredDomain[TType]) registerDirectTypes(ctx context.Context, app *application.Application) error {
	for _, sd := range d.Strategies.Entries() {
		if d.HasType(sd.Name) {
			continue
		}
		_, create := sd.Strategy.NewTypeConfig()
		raw, err := create(ctx)
		if err != nil {
			return fmt.Errorf("strategies: create direct type %q in domain %q: %w",
				sd.Name, d.Name, err)
		}
		if isNil(raw) {
			continue // Strategy requires config — skip direct type creation.
		}
		td := sd.Strategy.CreateTypeDef(sd.Name, raw)
		if err := application.Contribute(app, td); err != nil {
			return fmt.Errorf("strategies: contribute direct type %q to domain %q: %w",
				sd.Name, d.Name, err)
		}
	}
	return nil
}

// registerByProvider reads all ProviderDefs contributed to the domain,
// collects ConfigItems, creates types via services.CreateFromMap (config decoding),
// wraps results in TypeDefs, and contributes them.
func (d *DeclaredDomain[TType]) registerByProvider(ctx context.Context, app *application.Application) error {
	domainProviders := d.Providers.Entries()
	if len(domainProviders) == 0 {
		return nil
	}

	lookup := d.AsTypeFactoryLookup()

	for _, dp := range domainProviders {
		items, err := dp.Provider.ProvideConfigFor(d.Name)
		if err != nil {
			return fmt.Errorf("strategies: provider error for domain %q: %w", d.Name, err)
		}

		for _, item := range items {
			result, err := services.CreateFromMap(ctx, lookup, item.Data, d.typeFieldName)
			if err != nil {
				name := item.FindValue("name")
				// Skip unknown types with a warning — the strategy may not be
				// registered in this binary (e.g., notify from ktw-notify sidecar).
				slog.Warn("strategies: skipping unregistered type",
					"type", name, "domain", d.Name, "source", item.Source, "error", err)
				continue
			}

			strategy := result.Factory.(Strategy[TType])

			typeName := item.FindValue("name")
			if typeName == "" {
				typeName = result.TypeName
			}

			td := strategy.CreateTypeDef(typeName, result.Value)

			if err := application.Contribute(app, td); err != nil {
				return fmt.Errorf("strategies: contribute type %q to domain %q: %w",
					typeName, d.Name, err)
			}
		}
	}

	return nil
}

// Extension creates an application.Extension with the standard domain lifecycle:
// RegisterPoints (registers 3 extension points) + AppInit (providers then direct types).
// Dependencies are resolved via UseService[T](ctx) inside strategy createFn.
func (d *DeclaredDomain[TType]) Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints(d.Name, "", func(app *application.Application) error {
			return d.register(app)
		})
		init.AddAppInit(d.Name, "", func(app *application.Application) error {
			ctx := app.Context()
			if err := d.registerByProvider(ctx, app); err != nil {
				return err
			}
			return d.registerDirectTypes(ctx, app)
		})
	}
}

// --- Typed access helpers ---

// GetStrategyAs returns the Strategy cast to a concrete type, or zero+false.
func GetStrategyAs[
	S Strategy[TType],
	TType any,
](d *DeclaredDomain[TType], name string) (S, bool) {
	st := d.GetStrategy(name)
	if st == nil {
		var zero S
		return zero, false
	}
	typed, ok := st.(S)
	return typed, ok
}

// GetTypeAs returns the TypeDef's Type cast to a concrete type, or zero+false.
func GetTypeAs[
	T any,
	TType any,
](d *DeclaredDomain[TType], name string) (T, bool) {
	if !d.Types.Has(name) {
		var zero T
		return zero, false
	}
	td := d.Types.Get(name)
	typed, ok := any(td.Impl).(T)
	return typed, ok
}

// --- Error types ---

// ErrStrategyNotFound is returned when a strategy name is not found in the domain.
type ErrStrategyNotFound struct {
	Domain string
	Name   string
}

func (e ErrStrategyNotFound) Error() string {
	return fmt.Sprintf("strategy %q not found in domain %q", e.Name, e.Domain)
}

// ErrTypeNotFound is returned when a type name is not found in the domain.
type ErrTypeNotFound struct {
	Domain string
	Name   string
}

func (e ErrTypeNotFound) Error() string {
	return fmt.Sprintf("type %q not found in domain %q", e.Name, e.Domain)
}

// isNil checks if an interface value is nil (handles typed nil interfaces).
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}
