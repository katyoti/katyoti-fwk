// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"gopkg.in/yaml.v3"
)

// --- Test service types ---

type Greeter interface {
	Greet() string
}

type greeterImpl struct{ prefix string }

func (g *greeterImpl) Greet() string { return g.prefix + " world" }

type Logger interface {
	Log(msg string)
}

type loggerImpl struct{ messages []string }

func (l *loggerImpl) Log(msg string) { l.messages = append(l.messages, msg) }

// --- Test config structs ---

type greeterConfig struct {
	Prefix string `yaml:"prefix"`
}

// --- Test "NewService" functions (ctx-based pattern) ---

// NewGreeterFor returns a configured-greeter factory bound to a
// specific ServiceManager — the closure resolves the Logger dependency
// via `Get[Logger](sm, ctx, "default")` re-entrantly. After the Phase E
// §5a refactor (per-entry sync.Once in ScopeContainer, registry RWMutex
// only) recursive `Get` calls work without a context-carried resolver.
func NewGreeterFor(sm *ServiceManager) func(ctx context.Context, cfg *greeterConfig) (Greeter, error) {
	return func(ctx context.Context, cfg *greeterConfig) (Greeter, error) {
		log, _ := Get[Logger](sm, ctx, "default")
		if log != nil {
			log.Log("greeter created")
		}
		return &greeterImpl{prefix: cfg.Prefix}, nil
	}
}

func NewConfiguredGreeter(_ context.Context, cfg *greeterConfig) (Greeter, error) {
	return &greeterImpl{prefix: cfg.Prefix}, nil
}

func NewSimpleGreeter(_ context.Context) (Greeter, error) {
	return &greeterImpl{prefix: "hello"}, nil
}

func NewLogger(_ context.Context) (Logger, error) {
	return &loggerImpl{}, nil
}

// testCtx returns a context with ApplicationScope available (scopes need it).
func testCtx() context.Context {
	return context.Background()
}

// --- Test ConfigService helpers ---

func newMockConfigService() *config.ConfigService {
	return config.NewConfigService(loader.NewMapLoader(nil))
}

func newTestConfigService(entries map[string]map[string]any) *config.ConfigService {
	data := make(map[string]string)
	for key, m := range entries {
		b, _ := yaml.Marshal(m)
		data[key+".yaml"] = string(b)
	}
	return config.NewConfigService(loader.NewMapLoader(data))
}

// --- Tests ---

func TestSimpleService(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	ctx := testCtx()
	sm.RegisterService(SimpleService(NewSimpleGreeter))

	g, err := UseService[Greeter](sm, ctx)
	if err != nil {
		t.Fatalf("UseService: %v", err)
	}
	if got := g.Greet(); got != "hello world" {
		t.Errorf("Greet() = %q, want %q", got, "hello world")
	}
}

func TestConfiguredService(t *testing.T) {
	sm := NewServiceManager(newTestConfigService(map[string]map[string]any{
		config.ServiceName("Greeter", "default"): {"prefix": "bonjour"},
	}))
	ctx := testCtx()
	sm.RegisterService(ConfiguredService(NewConfiguredGreeter))

	g, err := UseService[Greeter](sm, ctx)
	if err != nil {
		t.Fatalf("UseService: %v", err)
	}
	if got := g.Greet(); got != "bonjour world" {
		t.Errorf("Greet() = %q, want %q", got, "bonjour world")
	}
}

func TestServiceWithDeps(t *testing.T) {
	sm := NewServiceManager(newTestConfigService(map[string]map[string]any{
		config.ServiceName("Greeter", "default"): {"prefix": "hello"},
	}))
	ctx := testCtx()

	sm.RegisterService(SimpleService(NewLogger))
	sm.RegisterService(ConfiguredService(NewGreeterFor(sm)))

	g, err := UseService[Greeter](sm, ctx)
	if err != nil {
		t.Fatalf("UseService: %v", err)
	}
	if got := g.Greet(); got != "hello world" {
		t.Errorf("Greet() = %q, want %q", got, "hello world")
	}

	// Verify Logger was injected and used.
	l, _ := UseService[Logger](sm, ctx)
	if msgs := l.(*loggerImpl).messages; len(msgs) != 1 || msgs[0] != "greeter created" {
		t.Errorf("Logger messages = %v", msgs)
	}
}

func TestMultipleInstances(t *testing.T) {
	sm := NewServiceManager(newTestConfigService(map[string]map[string]any{
		config.ServiceName("Greeter", "english"): {"prefix": "hello"},
		config.ServiceName("Greeter", "french"):  {"prefix": "bonjour"},
	}))
	ctx := testCtx()
	sm.RegisterService(ConfiguredService(NewConfiguredGreeter))

	en, err := UseNamedService[Greeter](sm, ctx, "english")
	if err != nil {
		t.Fatalf("Get english: %v", err)
	}
	fr, err := UseNamedService[Greeter](sm, ctx, "french")
	if err != nil {
		t.Fatalf("Get french: %v", err)
	}

	if en.Greet() != "hello world" {
		t.Errorf("english = %q", en.Greet())
	}
	if fr.Greet() != "bonjour world" {
		t.Errorf("french = %q", fr.Greet())
	}
}

func TestCaching(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	ctx := testCtx()
	callCount := 0
	sm.RegisterService(&simpleService[Greeter]{name: "Greeter", scope: ApplicationScope, create: func(_ context.Context) (Greeter, error) {
		callCount++
		return &greeterImpl{prefix: "hello"}, nil
	}})

	g1, _ := UseService[Greeter](sm, ctx)
	g2, _ := UseService[Greeter](sm, ctx)

	if g1 != g2 {
		t.Error("expected same instance (caching)")
	}
	if callCount != 1 {
		t.Errorf("create called %d times, want 1", callCount)
	}
}

func TestServiceNotFound(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	ctx := testCtx()

	_, err := Get[Greeter](sm, ctx, "default")
	if err == nil {
		t.Fatal("expected error")
	}
	var notFound ErrServiceNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("expected ErrServiceNotFound, got %T: %v", err, err)
	}
}

func TestPutBootstrapService(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	ctx := testCtx()

	logger := &loggerImpl{}
	Put[Logger](sm, "default", Logger(logger))

	got, err := UseService[Logger](sm, ctx)
	if err != nil {
		t.Fatalf("UseService: %v", err)
	}
	if got != Logger(logger) {
		t.Error("expected same instance from Put")
	}
}

func TestTypeName(t *testing.T) {
	if n := config.TypeName[Greeter](); n != "Greeter" {
		t.Errorf("config.TypeName[Greeter]() = %q", n)
	}
	if n := config.TypeName[greeterImpl](); n != "greeterImpl" {
		t.Errorf("config.TypeName[greeterImpl]() = %q", n)
	}
}

func TestIsEmptyStruct(t *testing.T) {
	if !isEmptyStruct(nil) {
		t.Error("nil should be empty")
	}
	if !isEmptyStruct(&struct{}{}) {
		t.Error("*struct{} should be empty")
	}
	if isEmptyStruct(&greeterConfig{}) {
		t.Error("*greeterConfig should not be empty")
	}
}

// --- Scope tests ---

func TestTransientScope_RunInScope(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	scope := NewTransientScope("test")

	sm.RegisterService(ScopedSimpleService[Greeter](scope, NewSimpleGreeter))

	// Outside scope — should fail.
	ctx := testCtx()
	_, err := Get[Greeter](sm, ctx, "default")
	if err == nil {
		t.Fatal("expected error outside scope")
	}

	// Inside scope — should succeed.
	err = scope.RunInScope(ctx, func(scopedCtx context.Context) error {
		g, err := Get[Greeter](sm, scopedCtx, "default")
		if err != nil {
			return err
		}
		if g.Greet() != "hello world" {
			t.Errorf("Greet() = %q", g.Greet())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInScope: %v", err)
	}
}

func TestTransientScope_NestedCreatesNewContainer(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	scope := NewTransientScope("test")
	callCount := 0

	sm.RegisterService(&simpleService[Greeter]{name: "Greeter", scope: scope, create: func(_ context.Context) (Greeter, error) {
		callCount++
		return &greeterImpl{prefix: "hello"}, nil
	}})

	ctx := testCtx()
	_ = scope.RunInScope(ctx, func(outerCtx context.Context) error {
		g1, _ := Get[Greeter](sm, outerCtx, "default")

		// Nested scope — new container, new instance.
		return scope.RunInScope(outerCtx, func(innerCtx context.Context) error {
			g2, _ := Get[Greeter](sm, innerCtx, "default")
			if g1 == g2 {
				t.Error("nested scope should create a different instance")
			}
			return nil
		})
	})

	if callCount != 2 {
		t.Errorf("create called %d times, want 2 (one per scope entry)", callCount)
	}
}

func TestScopeContainer_Close_DisposesCloser(t *testing.T) {
	closed := false
	svc := &closableService{onClose: func() { closed = true }}

	sc := NewScopeContainer(ApplicationScope)
	sc.Put("test", svc)
	_ = sc.Close()

	if !closed {
		t.Error("Close should have called Close on io.Closer service")
	}
}

type closableService struct {
	onClose func()
}

func (c *closableService) Close() error {
	c.onClose()
	return nil
}

// Verify closableService implements io.Closer.
var _ io.Closer = (*closableService)(nil)

func TestScopeContainer_GetOrCreate_Concurrent(t *testing.T) {
	sc := NewScopeContainer(ApplicationScope)
	createCount := 0
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sc.GetOrCreate("svc", func() (any, error) {
				mu.Lock()
				createCount++
				mu.Unlock()
				return "instance", nil
			})
		}()
	}
	wg.Wait()

	if createCount != 1 {
		t.Errorf("factory called %d times, want 1", createCount)
	}
}

func TestCrossScope_Dependencies(t *testing.T) {
	sm := NewServiceManager(newMockConfigService())
	taskScope := NewTransientScope("task")

	// Logger is app-scoped, Greeter is task-scoped but depends on Logger.
	sm.RegisterService(SimpleService(NewLogger))
	sm.RegisterService(ScopedService[Greeter, greeterConfig](taskScope, func(ctx context.Context, cfg *greeterConfig) (Greeter, error) {
		// Resolve app-scoped Logger from within task-scoped creation —
		// works re-entrantly post Phase E §5a refactor.
		log, _ := Get[Logger](sm, ctx, "default")
		if log != nil {
			log.Log("task greeter created")
		}
		return &greeterImpl{prefix: "task"}, nil
	}))

	ctx := testCtx()
	err := taskScope.RunInScope(ctx, func(scopedCtx context.Context) error {
		g, err := Get[Greeter](sm, scopedCtx, "default")
		if err != nil {
			return err
		}
		if g.Greet() != "task world" {
			t.Errorf("Greet() = %q", g.Greet())
		}

		// Logger should have been created and logged.
		l, err := Get[Logger](sm, scopedCtx, "default")
		if err != nil {
			return err
		}
		msgs := l.(*loggerImpl).messages
		if len(msgs) != 1 || msgs[0] != "task greeter created" {
			t.Errorf("Logger messages = %v", msgs)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInScope: %v", err)
	}
}
