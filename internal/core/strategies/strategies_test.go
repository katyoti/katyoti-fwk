// SPDX-License-Identifier: Apache-2.0

package strategies_test

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// --- Domain-specific test types ---

// testType simulates a configured type (Level 1 output, stored in Types point via TypeDef).
type testType struct {
	name     string
	strategy string
	url      string
}

func (t *testType) Name() string { return t.name }

// mcpTypeConfig is a typed config struct for the "mcp" strategy.
// After unmarshal fills its fields, the create closure produces a testType.
type mcpTypeConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// mcpStrategy implements Strategy for the "mcp" strategy using the tuple pattern.
type mcpStrategy struct{}

func (s *mcpStrategy) NewTypeConfig() (any, func(context.Context) (*testType, error)) {
	cfg := &mcpTypeConfig{}
	return cfg, func(_ context.Context) (*testType, error) {
		return &testType{
			name:     cfg.Name,
			strategy: "mcp",
			url:      cfg.URL,
		}, nil
	}
}

func (s *mcpStrategy) CreateTypeDef(name string, raw *testType) strategies.TypeDef[*testType] {
	return strategies.TypeDef[*testType]{

		DefMeta:  strategies.DefMeta{Name: name, Label: "MCP: " + name},
		Strategy: "mcp",
		Impl:     raw,
	}
}

func newMockConfig() *config.ConfigService {
	return config.NewConfigService(loader.NewMapLoader(nil))
}

// --- StrategyDef tests ---

func TestStrategyDef_Fields(t *testing.T) {
	sd := strategies.StrategyDef[*testType]{
		DefMeta:  strategies.DefMeta{Name: "mcp", Label: "MCP Plugin", Description: "Model Context Protocol plugin", Icon: "P"},
		Strategy: &mcpStrategy{},
	}
	if sd.Name != "mcp" {
		t.Errorf("Name = %q, want %q", sd.Name, "mcp")
	}
	if sd.Strategy == nil {
		t.Error("Strategy is nil")
	}

	// Verify the Strategy produces a usable type via tuple pattern
	cfg, create := sd.Strategy.NewTypeConfig()
	if cfg == nil {
		t.Fatal("NewConfig returned nil config")
	}
	tt, err := create(context.Background())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tt.strategy != "mcp" {
		t.Errorf("type strategy = %q, want %q", tt.strategy, "mcp")
	}
}

// --- FuncStrategy tests ---

func TestFuncStrategy(t *testing.T) {
	sd := strategies.FuncStrategy("program", "Program", func() (*testType, error) {
		return &testType{name: "prog", strategy: "program"}, nil
	})

	if sd.Name != "program" {
		t.Errorf("Name = %q, want %q", sd.Name, "program")
	}
	if sd.Label != "Program" {
		t.Errorf("Label = %q, want %q", sd.Label, "Program")
	}
	if sd.Strategy == nil {
		t.Fatal("Strategy is nil")
	}

	// Verify FuncStrategy produces a working type via tuple pattern
	_, create := sd.Strategy.NewTypeConfig()
	tt, err := create(context.Background())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tt.name != "prog" {
		t.Errorf("name = %q, want %q", tt.name, "prog")
	}
}

func TestFuncStrategy_CreateTypeDef(t *testing.T) {
	sd := strategies.FuncStrategy("program", "Program", func() (*testType, error) {
		return &testType{name: "prog", strategy: "program"}, nil
	})

	raw := &testType{name: "my-prog", strategy: "program"}
	td := sd.Strategy.CreateTypeDef("my-prog", raw)

	if td.Name != "my-prog" {
		t.Errorf("Name = %q, want %q", td.Name, "my-prog")
	}
	if td.Strategy != "program" {
		t.Errorf("Type = %q, want %q", td.Strategy, "program")
	}
	if td.Impl != raw {
		t.Error("Impl does not match raw type")
	}
}

// --- TypeDef tests ---

func TestTypeDef_Fields(t *testing.T) {
	td := strategies.TypeDef[*testType]{

		DefMeta:  strategies.DefMeta{Name: "my-mcp", Label: "My MCP", Description: "test description", Icon: "M"},
		Strategy: "mcp",
		Impl:     &testType{name: "my-mcp"},
	}
	if td.Name != "my-mcp" {
		t.Errorf("Name = %q, want %q", td.Name, "my-mcp")
	}
	if td.Strategy != "mcp" {
		t.Errorf("StrategyName = %q, want %q", td.Strategy, "mcp")
	}
	if td.Impl.name != "my-mcp" {
		t.Errorf("Type.name = %q, want %q", td.Impl.name, "my-mcp")
	}
}

// --- LoadTypedConfig with StrategyRegistry tests ---

func TestLoadTypedConfig_WithStrategyRegistry(t *testing.T) {
	app, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"type": "mcp", "name": "my-mcp", "url": "ws://localhost:8080"}, Source: "test"},
	}, nil)
	_ = app

	// Verify TypeDef was created via strategy registry
	td := getExt().domain.GetTypeDef("my-mcp")
	if td.Impl == nil {
		t.Fatal("type 'my-mcp' not found")
	}
	if td.Impl.name != "my-mcp" {
		t.Errorf("name = %q, want %q", td.Impl.name, "my-mcp")
	}
	if td.Impl.url != "ws://localhost:8080" {
		t.Errorf("url = %q, want %q", td.Impl.url, "ws://localhost:8080")
	}
	if td.Strategy != "mcp" {
		t.Errorf("StrategyName = %q, want %q", td.Strategy, "mcp")
	}
}

func TestLoadTypedConfig_FuncStrategy(t *testing.T) {
	app, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"type": "program", "ignored": "data"}, Source: "test"},
	}, nil)
	_ = app

	// FuncStrategy config has no "name" field, so TypeDef.Name falls back to strategy name.
	td := getExt().domain.GetTypeDef("program")
	if td.Impl == nil {
		t.Fatal("type 'program' not found")
	}
	if td.Impl.strategy != "program" {
		t.Errorf("strategy = %q, want %q", td.Impl.strategy, "program")
	}
	if td.Strategy != "program" {
		t.Errorf("StrategyName = %q, want %q", td.Strategy, "program")
	}
}

// --- ConfigProvider tests ---

func TestFuncConfigProvider(t *testing.T) {
	p := &provider.FuncConfigProvider{
		Fn: func(domain string) ([]provider.ConfigItem, error) {
			if domain == "test" {
				return []provider.ConfigItem{
					{Source: "test", Data: map[string]any{"name": "item1", "type": "mcp"}},
				}, nil
			}
			return nil, nil
		},
	}
	items, err := p.ProvideConfigFor("test")
	if err != nil {
		t.Fatalf("ProvideConfigFor: %v", err)
	}
	if len(items) != 1 || items[0].FindValue("name") != "item1" {
		t.Errorf("unexpected items: %v", items)
	}

	items, err = p.ProvideConfigFor("other")
	if err != nil {
		t.Fatalf("ProvideConfigFor(other): %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for unknown domain, got %v", items)
	}
}

// --- CreateDomain tests ---

func TestCreateDomain_CreatesTwoPoints(t *testing.T) {
	app := application.New(newMockConfig(), newDeclareDomainExt())
	if err := app.Run(); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	// Strategies and Types are registered in the EM (providers are domain-local).
	strategiesID := em.PointID[strategies.StrategyDef[*testType]]()
	typesID := em.PointID[strategies.TypeDef[*testType]]()
	for _, id := range []string{strategiesID, typesID} {
		if pi := app.PointInfo(id); pi == nil {
			t.Errorf("point %q not registered", id)
		}
	}
}

// Phase enforcement for RegisterDomain is now the Application's responsibility
// via application.RegisterPoint (points.go), not raw em.RegisterPoint.
// Raw EM is deliberately phase-unaware, so late registration succeeds at the EM level.
// Application-level phase tests live in application/application_test.go.

// --- DeclaredDomain access method tests ---

func TestDeclaredDomain_GetStrategy(t *testing.T) {
	_, getExt := runFullLifecycleApp(t, nil, nil)

	st := getExt().domain.GetStrategy("mcp")
	if st == nil {
		t.Fatal("GetStrategy('mcp') returned nil")
	}

	// Unknown strategy
	st = getExt().domain.GetStrategy("nonexistent")
	if st != nil {
		t.Error("GetStrategy('nonexistent') should return nil")
	}
}

func TestDeclaredDomain_GetTypeDef(t *testing.T) {
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"type": "mcp", "name": "my-mcp", "url": "ws://localhost"}, Source: "test"},
	}, nil)

	td := getExt().domain.GetTypeDef("my-mcp")
	if td.Impl == nil {
		t.Fatal("GetTypeDef('my-mcp') returned nil Data")
	}
	if td.Strategy != "mcp" {
		t.Errorf("StrategyName = %q, want %q", td.Strategy, "mcp")
	}
	if td.Label != "MCP: my-mcp" {
		t.Errorf("Label = %q, want %q", td.Label, "MCP: my-mcp")
	}
}

func TestDeclaredDomain_GetTypeData(t *testing.T) {
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"type": "mcp", "name": "my-mcp", "url": "ws://localhost"}, Source: "test"},
	}, nil)

	tt := getExt().domain.GetType("my-mcp")
	if tt == nil {
		t.Fatal("GetTypeData('my-mcp') returned nil")
	}
	if tt.name != "my-mcp" {
		t.Errorf("name = %q, want %q", tt.name, "my-mcp")
	}
}

// --- RegisterByProvider tests ---

func TestRegisterByProvider_HappyPath(t *testing.T) {
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"name": "my-mcp", "type": "mcp", "url": "ws://localhost"}, Source: "test"},
		{Data: map[string]any{"name": "my-prog", "type": "program"}, Source: "test"},
	}, nil)

	// Verify types created
	if !getExt().domain.HasType("my-mcp") {
		t.Error("type 'my-mcp' not found")
	}
	if !getExt().domain.HasType("my-prog") {
		t.Error("type 'my-prog' not found")
	}

	// Verify TypeDef content
	td := getExt().domain.GetTypeDef("my-mcp")
	if td.Impl == nil {
		t.Fatal("GetTypeDef('my-mcp') returned nil Data")
	}
	if td.Impl.strategy != "mcp" {
		t.Errorf("type strategy = %q, want %q", td.Impl.strategy, "mcp")
	}
	if td.Impl.url != "ws://localhost" {
		t.Errorf("type url = %q, want %q", td.Impl.url, "ws://localhost")
	}
	if td.Strategy != "mcp" {
		t.Errorf("StrategyName = %q, want %q", td.Strategy, "mcp")
	}
}

func TestRegisterByProvider_UnknownStrategy(t *testing.T) {
	// Unknown strategy types are skipped with a warning (not fatal).
	// The app starts successfully; the bad type is simply not registered.
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"name": "bad", "type": "nonexistent"}, Source: "test"},
	}, nil)
	if getExt().domain.HasType("bad") {
		t.Error("unknown strategy type should not be registered")
	}
}

func TestRegisterByProvider_CreateError(t *testing.T) {
	// Data has valid type but url is a list — triggers decode error in mcpTypeConfig.
	// Decode errors are skipped with a warning (not fatal).
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"name": "fail", "type": "mcp", "url": []any{"not", "a", "string"}}, Source: "test"},
	}, nil)
	if getExt().domain.HasType("fail") {
		t.Error("decode-error type should not be registered")
	}
}

func TestRegisterByProvider_EmptyProviders(t *testing.T) {
	// No providers → no provider-created types.
	// But registerDirectTypes still creates 1:1 types for contributed strategies.
	_, getExt := runFullLifecycleApp(t, nil, nil)
	if len(getExt().domain.Types.Keys()) != 2 {
		t.Errorf("expected 2 types (direct from strategies), got %d", len(getExt().domain.Types.Keys()))
	}
}

func TestRegisterByProvider_ProviderError(t *testing.T) {
	app := application.New(newMockConfig(), newFullLifecycleExt(nil, errors.New("provider failed")))
	err := app.Run()
	if err == nil {
		t.Fatal("expected provider error")
	}
}

// --- Integration test: full lifecycle with ApplicationInit ---

func TestFullLifecycle_WithComposite(t *testing.T) {
	// Shared domain for the integration test
	var integrationDomain *strategies.DeclaredDomain[*testType]

	integrationDomain = strategies.NewDomain[*testType]("integration")
	wiringExt := func(init *application.ApplicationInit) {
		init.AddExtension(integrationDomain.Extension())
	}

	strategyExt := func(init *application.ApplicationInit) {
		init.AddContribute("strategy-contributor", "", func(app *application.Application) error {
			if err := strategies.ContributeStrategy(app, strategies.StrategyDef[*testType]{
				DefMeta: strategies.DefMeta{Name: "mcp",
					Label: "MCP Strategy"},
				Strategy: &mcpStrategy{},
			}); err != nil {
				return err
			}
			return strategies.ContributeStrategy(app, strategies.FuncStrategy("program", "Program Strategy",
				func() (*testType, error) {
					return &testType{name: "my-prog", strategy: "program"}, nil
				},
			))
		})
	}

	configExt := func(init *application.ApplicationInit) {
		init.AddContribute("config-contributor", "", func(app *application.Application) error {
			return strategies.ContributeProvider(app, strategies.ProviderDef[*testType]{Provider: &provider.FuncConfigProvider{
				Fn: func(domain string) ([]provider.ConfigItem, error) {
					return []provider.ConfigItem{
						{Data: map[string]any{"name": "my-mcp", "type": "mcp", "url": "ws://example.com"}, Source: "test"},
						{Data: map[string]any{"name": "my-prog", "type": "program"}, Source: "test"},
					}, nil
				},
			}})
		})
	}

	composite := func(init *application.ApplicationInit) {
		init.AddExtension(wiringExt)
		init.AddExtension(strategyExt)
		init.AddExtension(configExt)
	}

	app := application.New(newMockConfig(), composite)
	if err := app.Run(); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	domain := integrationDomain
	if domain == nil {
		t.Fatal("domain not created")
	}

	// Strategies contributed
	if !domain.Strategies.Has("mcp") {
		t.Error("strategy 'mcp' not found")
	}
	if !domain.Strategies.Has("program") {
		t.Error("strategy 'program' not found")
	}

	// Types created from providers
	if !domain.HasType("my-mcp") {
		t.Error("type 'my-mcp' not found")
	}
	if !domain.HasType("my-prog") {
		t.Error("type 'my-prog' not found")
	}

	// Verify TypeDef content
	td := domain.GetTypeDef("my-mcp")
	if td.Impl.strategy != "mcp" {
		t.Errorf("my-mcp strategy = %q, want 'mcp'", td.Impl.strategy)
	}
	if td.Impl.url != "ws://example.com" {
		t.Errorf("my-mcp url = %q, want 'ws://example.com'", td.Impl.url)
	}
	if td.Strategy != "mcp" {
		t.Errorf("my-mcp StrategyName = %q, want 'mcp'", td.Strategy)
	}

	// FuncStrategy type
	ptd := domain.GetTypeDef("my-prog")
	if ptd.Impl.strategy != "program" {
		t.Errorf("my-prog strategy = %q, want 'program'", ptd.Impl.strategy)
	}
	if ptd.Strategy != "program" {
		t.Errorf("my-prog StrategyName = %q, want 'program'", ptd.Strategy)
	}
}

// --- DomainOption tests ---

func TestWithTypeFieldName(t *testing.T) {
	testDomain := strategies.NewDomain[*testType](
		"custom",
		strategies.WithTypeFieldName("strategy"),
	)
	ext := func(init *application.ApplicationInit) {
		init.AddExtension(testDomain.Extension())
		init.AddContribute("custom-field-name", "", func(app *application.Application) error {
			return strategies.ContributeStrategy(app, strategies.StrategyDef[*testType]{
				DefMeta: strategies.DefMeta{Name: "mcp",
					Label: "MCP"},
				Strategy: &mcpStrategy{},
			})
		})
	}
	app := application.New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	// CreateType should use "strategy" as field name
	data := map[string]any{"strategy": "mcp", "name": "test-mcp", "url": "ws://test"}
	tt, err := testDomain.CreateType(context.Background(), data)
	if err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if tt.name != "test-mcp" {
		t.Errorf("name = %q, want %q", tt.name, "test-mcp")
	}
}

// --- GetTypeDataAs tests ---

func TestGetTypeDataAs(t *testing.T) {
	_, getExt := runFullLifecycleApp(t, []provider.ConfigItem{
		{Data: map[string]any{"type": "mcp", "name": "my-mcp", "url": "ws://localhost"}, Source: "test"},
	}, nil)

	tt, ok := strategies.GetTypeAs[*testType](getExt().domain, "my-mcp")
	if !ok {
		t.Fatal("GetTypeDataAs returned false")
	}
	if tt.name != "my-mcp" {
		t.Errorf("name = %q, want %q", tt.name, "my-mcp")
	}

	// Unknown type
	_, ok = strategies.GetTypeAs[*testType](getExt().domain, "nonexistent")
	if ok {
		t.Error("GetTypeDataAs('nonexistent') should return false")
	}
}

// --- Test extension helpers ---

// newDeclareDomainExt creates an extension that creates a domain during RegisterPoints.
func newDeclareDomainExt() application.Extension {
	d := strategies.NewDomain[*testType]("test.domain")
	return d.Extension()
}

// fullLifecycleState holds state that persists after Run().
type fullLifecycleState struct {
	domain *strategies.DeclaredDomain[*testType]
}

// newFullLifecycleExt does the complete domain lifecycle in one extension.
func newFullLifecycleExt(configs []provider.ConfigItem, providerErr error) application.Extension {
	return newFullLifecycleExtWithState(configs, providerErr, &fullLifecycleState{})
}

// newFullLifecycleExtWithState creates a full lifecycle extension with shared state.
func newFullLifecycleExtWithState(configs []provider.ConfigItem, providerErr error, state *fullLifecycleState) application.Extension {
	return func(init *application.ApplicationInit) {
		state.domain = strategies.NewDomain[*testType]("test")
		init.AddExtension(state.domain.Extension())
		init.AddContribute("full-lifecycle", "", func(app *application.Application) error {
			// MCP strategy: typed config (Strategy → NewConfig → unmarshal → create)
			if err := strategies.ContributeStrategy(app, strategies.StrategyDef[*testType]{
				DefMeta: strategies.DefMeta{Name: "mcp",
					Label: "MCP"},
				Strategy: &mcpStrategy{},
			}); err != nil {
				return err
			}

			// Program strategy: FuncStrategy (no config, direct creation)
			if err := strategies.ContributeStrategy(app, strategies.FuncStrategy("program", "Program",
				func() (*testType, error) {
					return &testType{name: "my-prog", strategy: "program"}, nil
				},
			)); err != nil {
				return err
			}

			// Register config provider
			if providerErr != nil {
				return strategies.ContributeProvider(app, strategies.ProviderDef[*testType]{Provider: &provider.FuncConfigProvider{
					Fn: func(string) ([]provider.ConfigItem, error) { return nil, providerErr },
				}})
			}
			if len(configs) > 0 {
				return strategies.ContributeProvider(app, strategies.ProviderDef[*testType]{Provider: &provider.FuncConfigProvider{
					Fn: func(string) ([]provider.ConfigItem, error) { return configs, nil },
				}})
			}
			return nil
		})
		// AppInit (registerByProvider + registerDirectTypes) handled by Extension(nil) above.
	}
}

// runFullLifecycleApp creates and runs an app with the full lifecycle extension, failing the test on error.
func runFullLifecycleApp(t *testing.T, configs []provider.ConfigItem, providerErr error) (*application.Application, func() *fullLifecycleState) {
	t.Helper()
	state := &fullLifecycleState{}
	app := application.New(newMockConfig(), newFullLifecycleExtWithState(configs, providerErr, state))
	if err := app.Run(); err != nil {
		t.Fatalf("Run(): %v", err)
	}
	return app, func() *fullLifecycleState { return state }
}
