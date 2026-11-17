// SPDX-License-Identifier: Apache-2.0

package local

import (
	stdctx "context"
	"log/slog"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
)

// bootGate holds the `bootstrap` point until the application has booted.
//
// The CWM service is usually built DURING boot: the KTSP container resolves
// every published service at AppInit and publishes this binary's endpoint only
// once it has them all. A bootstrap hook names a slot — most often one this
// very binary serves (inbox/ensure, requests/ensure) — and the action service
// runs a slot in-process only when the endpoint holds it. Run before that, the
// hook went to the bus: the hub spawned another member of this binary, which
// took the workspace lock and locked this process's own commands out; with no
// hub, the hook failed and a fresh project got no system modules.
//
// So a service built during boot queues its bootstrap, and the gate runs it at
// the Bootstrap phase, synchronously: every slot is published, and the project
// has its system modules before any command runs on it. A service built after
// boot (resolved lazily at runtime) runs its bootstrap at once, in a goroutine
// — its factory must return before a hook can call CWM back — and closing the
// service waits it out (wait): it writes into the workspace (the index's
// catch-up cursor), never after the workspace is released.
type bootGate struct {
	mu      sync.Mutex
	booted  bool
	pending []func()
	running sync.WaitGroup
}

// afterBoot runs fn once the application has booted: queued until open when
// called during boot, in a goroutine when called after.
func (g *bootGate) afterBoot(fn func()) {
	g.mu.Lock()
	if !g.booted {
		g.pending = append(g.pending, fn)
		g.mu.Unlock()
		return
	}
	g.running.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.running.Done()
		fn()
	}()
}

// wait returns once what afterBoot started in the background has ended.
func (g *bootGate) wait() { g.running.Wait() }

// open marks the application booted and runs what was queued, in order.
func (g *bootGate) open() {
	g.mu.Lock()
	g.booted = true
	pending := g.pending
	g.pending = nil
	g.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

// runOthersBootstrap runs, once the process serves its universe, the
// bootstrap hooks another sidecar serves (core.BootstrapOthers). Run during
// boot, each waited out its request timeout — its action calls CWM back, and
// this process did not answer on the bus yet — and a project with several of
// them kept the process past the hub's readiness deadline: the hub killed it
// and spawned the next, forever.
func runOthersBootstrap(ctx stdctx.Context) {
	cwm, err := application.UseService[api.CWMService](ctx)
	if err != nil {
		slog.Warn("cwm: bootstrap hooks of other sidecars: no CWM service", "err", err)
		return
	}
	svc, ok := cwm.(interface {
		RunBootstrapHooks(stdctx.Context, core.BootstrapPart) []error
	})
	if !ok {
		return
	}
	for _, e := range svc.RunBootstrapHooks(ctx, core.BootstrapOthers) {
		slog.Warn("cwm: bootstrap hook failed", "error", e)
	}
}
