// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// A process that starts serving runs every contributed ServingHook, once,
// with the serving context.
func TestServingStartsTheContributedHooks(t *testing.T) {
	started := make(chan string, 2)
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hook", func(app *application.Application) error {
			return application.Contribute(app, ktsapi.ServingHook{Name: "h", Start: func(context.Context) { started <- "h" }})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}

	startServingHooks(app.Context())
	select {
	case got := <-started:
		if got != "h" {
			t.Errorf("started %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the serving hook did not start")
	}
	select {
	case <-started:
		t.Error("the hook started twice")
	case <-time.After(50 * time.Millisecond):
	}
}

// The others start once the First hooks are ready — the task restore decides
// a previous process's runs before the planner looks at them — and a First
// hook that never says so does not keep them from starting.
func TestFirstServingHooksStartBeforeTheOthers(t *testing.T) {
	events := make(chan string, 8)
	release := make(chan struct{})
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hooks", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "restore", First: true, Start: func(ctx context.Context) {
				<-release
				events <- "restore ready"
				ktsapi.ServingReady(ctx)
				<-ctx.Done()
			}}); err != nil {
				return err
			}
			return application.Contribute(app, ktsapi.ServingHook{Name: "planner", Start: func(context.Context) { events <- "planner" }})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx, cancel := context.WithCancel(app.Context())
	defer cancel()
	startServingHooks(ctx)
	select {
	case e := <-events:
		t.Fatalf("%q before the first hook was ready", e)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for _, want := range []string{"restore ready", "planner"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%q never came", want)
		}
	}
}

func TestAFirstHookNeverReadyDoesNotBlockTheOthers(t *testing.T) {
	old := servingFirstWait
	servingFirstWait = 50 * time.Millisecond
	t.Cleanup(func() { servingFirstWait = old })
	started := make(chan struct{}, 1)
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hooks", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "stuck", First: true, Start: func(ctx context.Context) { <-ctx.Done() }}); err != nil {
				return err
			}
			return application.Contribute(app, ktsapi.ServingHook{Name: "other", Start: func(context.Context) { started <- struct{}{} }})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(app.Context())
	defer cancel()
	startServingHooks(ctx)
	// The bound is 50 ms; the 10 s is a loaded machine's margin, not the rule.
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the other hook never started")
	}
}

// A serving hook that panics does not take the serving process down: the
// others still start.
func TestAPanickingServingHookDoesNotStopTheOthers(t *testing.T) {
	started := make(chan struct{}, 1)
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hooks", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "bad", First: true, Start: func(context.Context) { panic("boom") }}); err != nil {
				return err
			}
			return application.Contribute(app, ktsapi.ServingHook{Name: "good", Start: func(context.Context) { started <- struct{}{} }})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(app.Context())
	defer cancel()
	startServingHooks(ctx)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the other hook never started")
	}
}

// At stop the serving context ends: a hook sees ctx.Done(), does its last work
// (the scheduler's last export) and returns before the process exits; one that
// never returns does not keep it from exiting past the bound.
func TestServingHooksFinishAtStopWithinTheBound(t *testing.T) {
	var finished atomic.Bool
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hooks", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "restore", First: true, Start: func(ctx context.Context) {
				ktsapi.ServingReady(ctx)
				<-ctx.Done()
			}}); err != nil {
				return err
			}
			return application.Contribute(app, ktsapi.ServingHook{Name: "export", Start: func(ctx context.Context) {
				<-ctx.Done()
				time.Sleep(100 * time.Millisecond) // the last export
				finished.Store(true)
			}})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(app.Context())
	wait := startServingHooks(ctx)
	time.Sleep(100 * time.Millisecond) // the others have started
	cancel()
	if !wait(10 * time.Second) {
		t.Fatal("the serving hooks did not return at stop")
	}
	if !finished.Load() {
		t.Fatal("the wait returned before the hook finished its last work")
	}
}

func TestAStuckServingHookDoesNotBlockTheStop(t *testing.T) {
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	hookExt := func(init *application.ApplicationInit) {
		init.AddContribute("test", ".hooks", func(app *application.Application) error {
			return application.Contribute(app, ktsapi.ServingHook{Name: "stuck", First: true, Start: func(context.Context) { <-hold }})
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), AutoKTSPContainer("test", "serving hooks", "0.0.1"), hookExt)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(app.Context())
	wait := startServingHooks(ctx)
	cancel()
	if wait(50 * time.Millisecond) {
		t.Fatal("the wait says done while a hook still runs")
	}
}
