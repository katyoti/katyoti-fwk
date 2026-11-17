// SPDX-License-Identifier: Apache-2.0

package application

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

func newMockConfig() *config.ConfigService {
	return config.NewConfigService(loader.NewMapLoader(nil))
}

// --- Test helpers ---

func trackingExt(name string, calls *[]string) Extension {
	return func(init *ApplicationInit) {
		init.AddRegisterPoints(name, "", func(app *Application) error {
			*calls = append(*calls, name+":RegisterPoints")
			return nil
		})
		init.AddContribute(name, "", func(app *Application) error {
			*calls = append(*calls, name+":Contribute")
			return nil
		})
		init.AddAppInit(name, "", func(app *Application) error {
			*calls = append(*calls, name+":AppInit")
			return nil
		})
		init.AddBootstrap(name, "", func(app *Application) error {
			*calls = append(*calls, name+":Bootstrap")
			return nil
		})
	}
}

// --- Phase orchestration tests ---

func TestRun_CallsAllPhasesInOrder(t *testing.T) {
	var calls []string
	app := New(newMockConfig(), trackingExt("test", &calls))

	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	want := []string{"test:RegisterPoints", "test:Contribute", "test:AppInit", "test:Bootstrap"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, c, want[i])
		}
	}
}

func TestRun_MultipleExtensions_AllPhase1BeforePhase2(t *testing.T) {
	var globalOrder []string

	app := New(newMockConfig())
	for _, name := range []string{"a", "b", "c"} {
		trackingExt(name, &globalOrder)(app.init)
	}

	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(globalOrder, " ")
	want := "a:RegisterPoints b:RegisterPoints c:RegisterPoints a:Contribute b:Contribute c:Contribute a:AppInit b:AppInit c:AppInit a:Bootstrap b:Bootstrap c:Bootstrap"
	if got != want {
		t.Fatalf("order = %q\nwant  = %q", got, want)
	}
}

func TestRun_PhaseAfterCompletion(t *testing.T) {
	app := New(newMockConfig(), func(init *ApplicationInit) {})
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if app.phase != PhaseDone {
		t.Fatalf("phase = %v, want PhaseDone", app.phase)
	}
}

func TestRun_ErrorStopsExecution(t *testing.T) {
	tests := []struct {
		name      string
		phase     string
		notCalled string
	}{
		{"RegisterPointsError", "RegisterPoints", "Contribute"},
		{"ContributeError", "Contribute", "AppInit"},
		{"AppInitError", "AppInit", "Bootstrap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			ext := func(init *ApplicationInit) {
				addPhaseHook := map[string]func(string, string, HookFn) *ApplicationInit{
					"RegisterPoints": init.AddRegisterPoints,
					"Contribute":     init.AddContribute,
					"AppInit":        init.AddAppInit,
					"Bootstrap":      init.AddBootstrap,
				}
				for _, phase := range []string{"RegisterPoints", "Contribute", "AppInit", "Bootstrap"} {
					p := phase
					fn := func(app *Application) error {
						calls = append(calls, p)
						if p == tt.phase {
							return fmt.Errorf("%s failed", p)
						}
						return nil
					}
					addPhaseHook[p]("failing", "", fn)
				}
			}

			app := New(newMockConfig(), ext)
			err := app.Run()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.phase+" failed") {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.phase+" failed")
			}
			for _, c := range calls {
				if c == tt.notCalled {
					t.Fatalf("%s was called despite %s error", tt.notCalled, tt.phase)
				}
			}
		})
	}
}

func TestRun_BootstrapError_StopsSecondExtension(t *testing.T) {
	var calls []string
	ext1 := func(init *ApplicationInit) {
		init.AddBootstrap("first", "", func(app *Application) error {
			calls = append(calls, "first:Bootstrap")
			return fmt.Errorf("boot failed")
		})
	}
	ext2 := func(init *ApplicationInit) {
		init.AddBootstrap("second", "", func(app *Application) error {
			calls = append(calls, "second:Bootstrap")
			return nil
		})
	}

	app := New(newMockConfig(), ext1, ext2)
	err := app.Run()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, c := range calls {
		if c == "second:Bootstrap" {
			t.Fatal("second extension's Bootstrap was called despite first's error")
		}
	}
}

// --- Phase-gated point operations ---

func TestRegisterPoint_PhaseViolation(t *testing.T) {
	ext := func(init *ApplicationInit) {
		init.AddContribute("late-register", "", func(app *Application) error {
			point := em.NewListPoint[string]()
			err := RegisterPoint(app, point)
			if err == nil {
				t.Fatal("expected ErrPhaseViolation for RegisterPoint during Contribute phase")
			}
			var phaseErr ErrPhaseViolation
			if !errors.As(err, &phaseErr) {
				t.Fatalf("expected ErrPhaseViolation, got %T: %v", err, err)
			}
			return nil
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestContribute_PhaseViolation(t *testing.T) {
	ext := func(init *ApplicationInit) {
		init.AddRegisterPoints("early-contribute", "", func(app *Application) error {
			point := em.NewListPoint[string]()
			if err := RegisterPoint(app, point); err != nil {
				return err
			}
			err := Contribute(app, "too early")
			if err == nil {
				t.Fatal("expected ErrPhaseViolation for Contribute during RegisterPoints phase")
			}
			var phaseErr ErrPhaseViolation
			if !errors.As(err, &phaseErr) {
				t.Fatalf("expected ErrPhaseViolation, got %T: %v", err, err)
			}
			return nil
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestContribute_DuringAppInit_Allowed(t *testing.T) {
	ext := func(init *ApplicationInit) {
		init.AddRegisterPoints("appinit-contribute", "", func(app *Application) error {
			point := em.NewListPoint[string]()
			return RegisterPoint(app, point)
		})
		init.AddAppInit("appinit-contribute", "", func(app *Application) error {
			return Contribute(app, "factory-from-provider")
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	// Verify the contribution landed.
	entries := UseAppExtensions[string](app)
	if len(entries) != 1 || entries[0] != "factory-from-provider" {
		t.Fatalf("entries = %v, want [factory-from-provider]", entries)
	}
}

func TestContribute_PointNotFound(t *testing.T) {
	ext := func(init *ApplicationInit) {
		init.AddContribute("missing", "", func(app *Application) error {
			// Contributing a type with no registered point should fail.
			type unregisteredType struct{ V string }
			err := Contribute(app, unregisteredType{"value"})
			if err == nil {
				t.Fatal("expected ErrPointNotFound")
			}
			var notFound em.ErrPointNotFound
			if !errors.As(err, &notFound) {
				t.Fatalf("expected ErrPointNotFound, got %T: %v", err, err)
			}
			return nil
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
}

// --- ApplicationInit tests ---

func TestApplicationInit_AddExtension(t *testing.T) {
	var order []string
	child := func(init *ApplicationInit) {
		init.AddRegisterPoints("child", "", func(app *Application) error {
			order = append(order, "child:RegisterPoints")
			return nil
		})
		init.AddContribute("child", "", func(app *Application) error {
			order = append(order, "child:Contribute")
			return nil
		})
	}

	parent := func(init *ApplicationInit) {
		init.AddRegisterPoints("parent", "", func(app *Application) error {
			order = append(order, "parent:RegisterPoints")
			return nil
		})
		init.AddExtension(child)
		init.AddContribute("parent", "", func(app *Application) error {
			order = append(order, "parent:Contribute")
			return nil
		})
	}

	app := New(newMockConfig(), parent)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	// Phase 1: parent, then child. Phase 2: child, then parent.
	expected := []string{
		"parent:RegisterPoints", "child:RegisterPoints",
		"child:Contribute", "parent:Contribute",
	}
	if len(order) != len(expected) {
		t.Fatalf("got %d calls, want %d: %v", len(order), len(expected), order)
	}
	for i, got := range order {
		if got != expected[i] {
			t.Errorf("call[%d] = %q, want %q", i, got, expected[i])
		}
	}
}

func TestApplicationInit_Chaining(t *testing.T) {
	init := newApplicationInit()
	noop := func(app *Application) error { return nil }
	child := func(init *ApplicationInit) {}

	got := init.AddRegisterPoints("p", "n", noop)
	if got != init {
		t.Error("AddRegisterPoints should return same pointer")
	}
	got = init.AddContribute("p", "n", noop)
	if got != init {
		t.Error("AddContribute should return same pointer")
	}
	got = init.AddAppInit("p", "n", noop)
	if got != init {
		t.Error("AddAppInit should return same pointer")
	}
	got = init.AddBootstrap("p", "n", noop)
	if got != init {
		t.Error("AddBootstrap should return same pointer")
	}
	got = init.AddExtension(child)
	if got != init {
		t.Error("AddExtension should return same pointer")
	}
}

// --- Bootstrap service registration tests ---

func TestNew_RegistersBootstrapServices(t *testing.T) {
	cfg := newMockConfig()
	app := New(cfg)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	// SM should be resolvable.
	sm, err := services.UseService[*services.ServiceManager](app.sm, app.Context())
	if err != nil {
		t.Fatalf("UseService[*ServiceManager]: %v", err)
	}
	if sm != app.sm {
		t.Error("expected same ServiceManager instance")
	}

	// Application should be resolvable.
	gotApp, err := services.UseService[*Application](app.sm, app.Context())
	if err != nil {
		t.Fatalf("UseService[*Application]: %v", err)
	}
	if gotApp != app {
		t.Error("expected same Application instance")
	}

	// Config should be resolvable.
	gotCfg, err := services.UseService[*config.ConfigService](app.sm, app.Context())
	if err != nil {
		t.Fatalf("UseService[*ConfigService]: %v", err)
	}
	if gotCfg != cfg {
		t.Error("expected same ConfigService instance")
	}

	// EM registered after Run().
	gotEM, err := services.UseService[*em.EM](app.sm, app.Context())
	if err != nil {
		t.Fatalf("UseService[*EM]: %v", err)
	}
	if gotEM != app.em {
		t.Error("expected same EM instance")
	}
}

func TestRegister_BeforeRun(t *testing.T) {
	var calls []string
	app := New(newMockConfig())
	app.Register(trackingExt("late", &calls))

	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(calls), calls)
	}
}

func TestNoOp_Extension(t *testing.T) {
	app := New(newMockConfig(), func(init *ApplicationInit) {})
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
}

// --- EM ownership via Application ---

func TestEM_OwnershipTracking(t *testing.T) {
	ext := func(init *ApplicationInit) {
		init.AddRegisterPoints("my-ext", ".point", func(app *Application) error {
			return RegisterPoint(app, em.NewListPoint[string]())
		})
		init.AddContribute("my-ext", ".contrib", func(app *Application) error {
			return Contribute(app, "item")
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	info := app.em.PointInfo(em.PointID[string]())
	if info == nil {
		t.Fatal("PointInfo = nil")
	}
	if info.Owner != "my-ext.point" {
		t.Fatalf("Owner = %q, want %q", info.Owner, "my-ext.point")
	}
	if len(info.Contributors) != 1 || info.Contributors[0] != "my-ext.contrib" {
		t.Fatalf("Contributors = %v, want [my-ext.contrib]", info.Contributors)
	}
}

// --- IndexedPoint via Application ---

func TestIndexedPoint_ViaApplication(t *testing.T) {
	type Strategy struct {
		Name string
	}

	ext := func(init *ApplicationInit) {
		init.AddRegisterPoints("indexed", "", func(app *Application) error {
			point := em.NewIndexedPoint[Strategy](func(s Strategy) string { return s.Name })
			return RegisterPoint(app, point)
		})
		init.AddContribute("indexed", "", func(app *Application) error {
			if err := Contribute(app, Strategy{Name: "mcp"}); err != nil {
				return err
			}
			return Contribute(app, Strategy{Name: "xskill"})
		})
	}

	app := New(newMockConfig(), ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}

	raw, err := em.GetPoint[Strategy](app.em)
	if err != nil {
		t.Fatal(err)
	}
	indexed := raw.(*em.IndexedPoint[Strategy])
	if !indexed.Has("mcp") || !indexed.Has("xskill") {
		t.Fatal("missing expected strategies")
	}
}
