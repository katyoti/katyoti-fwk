// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	mbsubscribe "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
	"github.com/spf13/cobra"
)

// Resolution contract for `<binary> sc ktsp nats`. We deliberately do NOT
// invent new env vars when an existing resolution chain already covers
// the same ground:
//
//   - **Universe** is resolved via the existing `cenv` chain (`KTW_WORKSPACE`
//     → Univers.yaml `name`). One env var (`KTW_WORKSPACE`) — not three —
//     and it's the same one every other command reads.
//   - **NATS URL** is auto-discovered from `<DataDir>/hub/hub.lock`
//     written by `ktw-hub serve` (the hubclient lockfile). The env var
//     `KTW_NATS_URL` is a manual-override fast path for ad-hoc dev use
//     without a hub on disk; production sidecars don't need it.
//   - **Tenant** defaults to `"local"` per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md)
//     (desktop = `local` tenant; cloud tenant comes from auth, not env).
//     The override `KTW_TENANT_ID` is kept for cloud-side bootstrap that
//     hasn't reached the auth layer yet.
const (
	envNATSURLOverride  = "KTW_NATS_URL"  // override only; auto-discovered from lockfile
	envTenantIDOverride = "KTW_TENANT_ID" // override only; default "local"
	defaultTenant       = "local"
)

// NewNATSSubCommand creates a `nats` subcommand under the ktsp group.
// Sibling to `sc ktsp stdio`; name communicates the transport.
//
// Resolution (in order):
//
//	NATS URL    : $KTW_NATS_URL → else <DataDir>/hub/hub.lock (auto)
//	universeID  : Univers.yaml `name` field (existing cenv chain)
//	tenantID    : $KTW_TENANT_ID → else "local"
//
// Production sidecars are exec'd by the bus host's connection manager
// with the same `KTW_WORKSPACE` (or `KTW_UNIVERSE`) the rest of the framework
// already uses to scope `application.Env()`. No new env contract.
func NewNATSSubCommand(container *ktsp.Endpoint) *cobra.Command {
	return &cobra.Command{
		Use:   "nats",
		Short: "Run as a NATS-backed KTSP server (used by ktw-hub)",
		Long: `Starts the sidecar in NATS server mode: connects to the bus host's
NATS server, installs a queue subscription per slot on its scoped subject
pattern, and dispatches incoming requests through the local KTSP container.

Resolution:
  - NATS URL: auto-discovered from <DataDir>/hub/hub.lock (written by
    'ktw-hub serve'). Override with $KTW_NATS_URL for ad-hoc dev.
  - Universe: read from Univers.yaml at the active KTW_WORKSPACE (same as
    every other command).
  - Tenant: defaults to "local" (desktop), the bus's rule for subjects, streams and buckets.
    Override with $KTW_TENANT_ID for cloud bootstrap until auth lands.

This is the transport the bus host uses when spawning sidecars after the
NATS-mode cutover. The legacy 'ktsp stdio' mode coexists for direct-spawn
fallbacks.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A sidecar serving the bus has no cause of its own: each call it
			// serves carries its caller's.
			cmd.SetContext(cause.Without(cmd.Context()))
			return runNATSSubcommand(cmd.Context(), cmd, container)
		},
	}
}

func runNATSSubcommand(ctx context.Context, cmd *cobra.Command, container *ktsp.Endpoint) error {
	env := application.Env()
	if env == nil {
		return fmt.Errorf("sc ktsp nats: no Env (KTW_WORKSPACE unresolved)")
	}

	// One serving process per universe and provider: its lock first — before
	// anything is connected —, held for this process's life (serving_lock.go).
	servingLock, err := acquireServingLock(ctx, env.DataDir(), cmd.Root().Use, servingLockWait)
	if err != nil {
		return fmt.Errorf("sc ktsp nats: %w", err)
	}
	defer func() { _ = servingLock.Unlock() }()

	// 1. NATS URL — explicit override, else lockfile auto-discovery.
	natsURL := os.Getenv(envNATSURLOverride)
	if natsURL == "" {
		lf, err := hubclient.ReadLockFile(env.DataDir())
		if err != nil {
			return fmt.Errorf("sc ktsp nats: read hub lockfile: %w", err)
		}
		if lf == nil {
			return fmt.Errorf("sc ktsp nats: no hub lockfile at %s and %s not set — start ktw-hub or set $%s",
				hubclient.LockFilePath(env.DataDir()), envNATSURLOverride, envNATSURLOverride)
		}
		natsURL = lf.NATSURL
	}

	// 2. Universe — from the active universe workspace.yaml (same chain
	// every other command reads).
	manifest, err := cenv.LoadUniverseManifest(env.UniverseDir())
	if err != nil {
		return fmt.Errorf("sc ktsp nats: resolve universe: %w", err)
	}
	universeID := manifest.Name

	// 3. Tenant — desktop default, cloud override.
	tenantID := os.Getenv(envTenantIDOverride)
	if tenantID == "" {
		tenantID = defaultTenant
	}

	// 4. Its bus key - the one the hub minted for it and named in its
	// environment; none on a hub that checks no keys.
	busKey, err := busauth.FromEnv()
	if err != nil {
		return fmt.Errorf("sc ktsp nats: %w", err)
	}

	handle, err := natsxport.Connect(ctx, natsURL, natsxport.ConnectOptions{
		Credential:     busKey,
		Name:           "ktsp-nats:" + cmd.Root().Use,
		MaxReconnects:  -1,
		AllowReconnect: true,
		// The hub's NATS port is chosen at each start: on every reconnect the
		// URL is read again from its lock file (as HubClient does) — a hub
		// restarted on a new port is found. An explicit URL stays as given.
		ResolveURL: func() (string, error) {
			if os.Getenv(envNATSURLOverride) != "" {
				return "", nil
			}
			lf, err := hubclient.ReadLockFile(env.DataDir())
			if err != nil || lf == nil {
				return "", err
			}
			return lf.NATSURL, nil
		},
	})
	if err != nil {
		return fmt.Errorf("sc ktsp nats: connect: %w", err)
	}
	defer handle.Close()

	// Publish the MessageBus + scope as application services so slot
	// handlers running inside this process (e.g. Terminal service) can
	// resolve them when they need to emit event notifications on
	// session-scoped subjects. Without this, handlers can only Notify on
	// slot-scoped subjects via the ktsp.Sender — fine for request/reply,
	// but no good for fan-out streaming where the subject path carries
	// (session, task) identity for multi-subscriber cowork.
	if app := application.AppFromContext(cmd.Context()); app != nil {
		application.PutService[mbapi.MessageBus](app, "default", handle.MessageBus())
	}

	// Self-provider is the binary name (cobra root's Use field). Also
	// recorded on the Application during NewAppExt's Contribute phase
	// — same value, read here directly to keep this command independent
	// of when AppFromContext is populated.
	selfProvider := cmd.Root().Use

	group, err := mbsubscribe.Subscribe(ctx, handle.MessageBus(), tenantID, universeID, selfProvider, container)
	if err != nil {
		return fmt.Errorf("sc ktsp nats: subscribe: %w", err)
	}
	defer func() { _ = group.Drain() }()

	fmt.Fprintf(cmd.ErrOrStderr(),
		"sc ktsp nats: subscribed to %d slot(s) at %s (tenant=%s universe=%s provider=%s)\n",
		len(group.Slots()), natsURL, tenantID, universeID, selfProvider)

	// The serving context ends at SIGINT / SIGTERM (or a parent ctx
	// cancellation) — or when the serving lock no longer holds its path.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	servingCtx, lostLock := context.WithCancel(sigCtx)
	defer lostLock()
	lockLost := make(chan struct{})
	go watchServingLock(servingCtx, servingLock, servingLockCheck, func() { close(lockLost); lostLock() })

	// Now serving: start what only the serving process does, with the serving
	// context, so a hook sees the process stop (a last export at hub stop).
	waitHooks := startServingHooks(servingCtx)

	<-servingCtx.Done()
	// The hooks finish before the subscriptions drain — bounded, so a stuck
	// hook cannot keep the process from exiting.
	if !waitHooks(servingHooksStopWait) {
		slog.Warn("serving: a serving hook did not return at stop: exiting anyway", "after", servingHooksStopWait)
	}
	select {
	case <-lockLost:
		return fmt.Errorf("sc ktsp nats: the serving lock %s no longer holds its path — stopped serving", servingLockPath(env.DataDir(), selfProvider))
	default:
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "sc ktsp nats: shutting down (draining subscriptions)")
	return nil
}

// recoverServingHook keeps a serving hook that panics from taking the serving
// process down with it: the panic is reported, the others go on.
func recoverServingHook(name string) {
	if r := recover(); r != nil {
		slog.Error("serving: a serving hook panicked — it stopped, the process goes on", "hook", name, "panic", r)
	}
}

// servingFirstWait bounds how long the other hooks wait for the First ones: a
// First hook that never says ready does not keep the rest from starting.
var servingFirstWait = 2 * time.Minute

// servingHooksStopWait bounds how long a stopping process waits for its
// serving hooks to return once their context ended.
var servingHooksStopWait = 10 * time.Second

// startServingHooks runs every contributed ServingHook on its own goroutine:
// the First ones at once, the others once those are ready (see ServingHook).
// The answer waits, at most the given time, for every hook to return; false
// when one still runs.
func startServingHooks(ctx context.Context) (wait func(time.Duration) bool) {
	hooks := application.UseExtensions[ktsapi.ServingHook](ctx)
	var running, ready sync.WaitGroup
	for _, h := range hooks {
		if h.Start == nil || !h.First {
			continue
		}
		ready.Add(1)
		running.Add(1)
		var once sync.Once
		done := func() { once.Do(ready.Done) }
		go func(h ktsapi.ServingHook) {
			defer running.Done()
			defer done()
			defer recoverServingHook(h.Name)
			h.Start(ktsapi.WithServingReady(ctx, done))
		}(h)
	}
	running.Add(1)
	go func() {
		defer running.Done()
		all := make(chan struct{})
		go func() { ready.Wait(); close(all) }()
		select {
		case <-all:
		case <-time.After(servingFirstWait):
			slog.Warn("serving: a first serving hook is not ready: the others start anyway", "after", servingFirstWait)
		case <-ctx.Done():
			return
		}
		for _, h := range hooks {
			if h.Start != nil && !h.First {
				running.Add(1)
				go func(h ktsapi.ServingHook) {
					defer running.Done()
					defer recoverServingHook(h.Name)
					h.Start(ctx)
				}(h)
			}
		}
	}()
	return func(limit time.Duration) bool {
		all := make(chan struct{})
		go func() { running.Wait(); close(all) }()
		select {
		case <-all:
			return true
		case <-time.After(limit):
			return false
		}
	}
}
