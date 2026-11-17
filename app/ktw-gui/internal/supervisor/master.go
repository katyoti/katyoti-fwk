// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// activeTenantID is the canonical tenant the desktop GUI assigns to every
// per-universe NATS scope. Per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md), desktop deployments
// always run under `local`; cloud installs override via auth, not via env.
const activeTenantID = TenantID

// TenantID is the desktop GUI's tenant, the one constant every GUI subject
// is scoped under (the display panels' too).
const TenantID = "local"

// ActiveCore is one entry in the active-universes map. It owns the
// connection to a ktw-hub running for a single activated universe and
// the `messagebus.Sender` that routes slot calls into that hub.
//
// One ActiveCore per universe per ktw-gui process. Multiple ActiveCores
// can run side-by-side (each universe = its own hub + sidecar tree);
// deactivating one does not affect the others.
//
// Lifetime: when ktw-gui spawned the hub (`ModeFindOrSpawn` resulted in
// a spawn), the hubclient owns the hub subprocess and `Close()` SIGTERMs
// it with grace. When ktw-gui connected to an already-running hub
// (lockfile present, healthy PID), `Close()` only drops the NATS
// connection — the hub keeps running for other clients.
type ActiveCore struct {
	Name      string
	HomePath  string
	StartedAt time.Time
	// HubFound: this GUI found the universe's hub running and did not start
	// it - a hub started for good (`katyoti universe start`, `--hub keep`) or
	// by another program - so closing the GUI leaves it running.
	HubFound bool

	mu         sync.RWMutex
	hub        *hubclient.HubClient
	sender     ktsp.Sender
	routerOnce sync.Once            // exactly-once router init; outside c.mu so newTerminalRouter can call c.Sender() without re-entrant RLock
	router     *terminalRouter      // demuxes session-scoped event subjects; lazy-initialized on first chat run
	wsSub      mbapi.Subscription   // workspace lifecycle-event watch; nil if the subscribe failed
	catalogSub mbapi.Subscription   // contributions-changed watch; nil if the subscribe failed
	runsSub    mbapi.Subscription   // run-lifecycle watch; nil if the subscribe failed
	adminSubs  []mbapi.Subscription // control-plane change watches (admin core only): registry, profile
	exited     chan struct{}
	exitErr    error
	closed     bool
}

// TerminalRouter returns the core's terminalRouter, lazily creating it on
// first call. Used by App.runViaUniverse to start a chat run.
//
// Uses sync.Once instead of c.mu.Lock so newTerminalRouter is free to call
// c.Sender() (which takes c.mu.RLock). Holding the write lock during
// init would deadlock — Go's sync.RWMutex does not support Lock→RLock
// re-entry by the same goroutine.
func (c *ActiveCore) TerminalRouter() *terminalRouter {
	c.routerOnce.Do(func() {
		c.router = newTerminalRouter(c)
	})
	return c.router
}

// Master holds the active-universes map for this ktw-gui process.
// Thread-safe.
type Master struct {
	mu               sync.RWMutex
	active           map[string]*ActiveCore
	wsHandler        WorkspaceEventHandler // forwards workspace events to the App layer; nil until SetWorkspaceEventHandler
	universesHandler UniversesEventHandler // forwards registry changes; nil until SetUniversesEventHandler
	profileHandler   ProfileEventHandler   // forwards profile changes; nil until SetProfileEventHandler
	catalogHandler   CatalogEventHandler   // forwards catalog changes; nil until SetCatalogEventHandler
	runsHandler      RunsEventHandler      // forwards run transitions; nil until SetRunsEventHandler

	// activating holds, per universe, the activation in progress: closed when
	// it is through. A second Activate of the same universe waits for it
	// instead of connecting (or spawning) on its own.
	activating map[string]chan struct{}
	// start connects to (or spawns) a universe's hub: startUniverseCore; a
	// test stands a fake in.
	start func(ctx context.Context, name string) (*ActiveCore, error)
	// stopHub ends the hub serving the universe at home and the members it
	// spawned, whoever started it: stopUniverseHub; a test stands a fake in.
	stopHub func(ctx context.Context, home string) error
}

// NewMaster returns an empty Master ready to accept activations.
func NewMaster() *Master {
	return &Master{active: map[string]*ActiveCore{}, activating: map[string]chan struct{}{}, start: startUniverseCore, stopHub: stopUniverseHub}
}

// IsActive reports whether the named universe has a running ActiveCore.
func (m *Master) IsActive(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.active[name]
	return ok
}

// ActiveNames returns the sorted list of currently-active universe names.
func (m *Master) ActiveNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.active))
	for n := range m.active {
		names = append(names, n)
	}
	return names
}

// ActiveUniverseInfo is one entry in App.UniverseListActive's output.
type ActiveUniverseInfo struct {
	Name          string `json:"name"`
	HomePath      string `json:"homePath"`
	StartedAtUnix int64  `json:"startedAtUnix"`
	// HubKeepsRunning: the GUI found this universe's hub running rather than
	// starting it, so the hub may keep running once the GUI closes.
	HubKeepsRunning bool `json:"hubKeepsRunning"`
}

// ListActive returns runtime info for every active universe (used by the
// frontend to update the IconRibbon's running indicators).
func (m *Master) ListActive() []ActiveUniverseInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ActiveUniverseInfo, 0, len(m.active))
	for _, c := range m.active {
		out = append(out, ActiveUniverseInfo{
			Name:            c.Name,
			HomePath:        c.HomePath,
			StartedAtUnix:   c.StartedAt.Unix(),
			HubKeepsRunning: c.HubFound,
		})
	}
	return out
}

// Activate connects ktw-gui to the ktw-hub for the named universe,
// spawning the hub if it isn't already running.
//
// Idempotent: returns nil + the existing ActiveCore if the universe is
// already active. Otherwise:
//  1. Look up the universe's home path in ~/.config/ktw-agents/universes.yaml
//  2. Resolve the per-universe DataDir (where ktw-hub writes its lockfile)
//  3. ConnectToHub(ModeFindOrSpawn) — spawns ktw-hub with KTW_WORKSPACE=<homePath>
//     when no live hub is found, else reuses
//  4. Build a messagebus.Sender bound to (tenant="local", universe=name)
//  5. Launch a watcher that signals via core.exited when the hub
//     connection terminates
//
// SINGLE-FLIGHT per universe: at a restart the frontend activates the same
// universe (the admin one above all) from several calls at once. One of them
// connects; the others wait for it and get its core. Each connecting on its
// own spawned or attached once per call, and the losers' cores were closed —
// a closed core that owned the hub took the hub down with it.
func (m *Master) Activate(ctx context.Context, name string) (*ActiveCore, error) {
	var done chan struct{}
	for {
		m.mu.Lock()
		if existing, ok := m.active[name]; ok {
			m.mu.Unlock()
			return existing, nil
		}
		wait, busy := m.activating[name]
		if !busy {
			done = make(chan struct{})
			m.activating[name] = done
			m.mu.Unlock()
			break
		}
		m.mu.Unlock()
		select {
		case <-wait: // through: it is active now, or it failed and we lead the retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() {
		m.mu.Lock()
		delete(m.activating, name)
		m.mu.Unlock()
		close(done)
	}()

	// The activation's wall time is logged: "the GUI is slow to open a
	// universe" is first a question of where the seconds went, and this is
	// the GUI's side of the answer (the hub's boot is in its own log).
	began := time.Now()
	core, err := m.start(ctx, name)
	if err != nil {
		log.Printf("[supervisor] universe %q: activation failed after %s: %v", name, time.Since(began).Round(time.Millisecond), err)
		return nil, err
	}
	log.Printf("[supervisor] universe %q: hub connected in %s", name, time.Since(began).Round(time.Millisecond))

	m.mu.Lock()
	// Race check: Activate is single-flight, but a core may have been put in
	// place another way meanwhile.
	if existing, ok := m.active[name]; ok {
		m.mu.Unlock()
		_ = core.Close()
		return existing, nil
	}
	m.active[name] = core
	m.mu.Unlock()

	// Watch for unexpected hub exit; remove from map when it signals.
	go m.watch(name, core)

	// A core on no bus (a test's stand-in) has nothing to watch.
	if core.MessageBus() != nil {
		// Live workspace refresh: forward CWM lifecycle events to the App.
		m.installWorkspaceWatch(core)
		// Live catalog refresh: what the frontend derives from the catalog (the
		// translation bundle) is re-read when it reloads.
		m.installCatalogWatch(core)
		m.installRunsWatch(core)

		// Live control-plane refresh: a universe created or a profile set from a
		// CLI (or another GUI) must show up here without a restart.
		m.installAdminWatches(core)
	}

	return core, nil
}

// startUniverseCore resolves the universe's home from the registry and
// connects to its hub, spawning it when none runs.
func startUniverseCore(ctx context.Context, name string) (*ActiveCore, error) {
	// Resolve home path from registry.
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		return nil, fmt.Errorf("resolve registry: %w", err)
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	entry, err := reg.Find(name)
	if err != nil {
		return nil, fmt.Errorf("universe %q not in registry: %w", name, err)
	}

	return spawnCore(ctx, name, entry.HomePath)
}

// Deactivate disconnects the named universe and (if ktw-gui owns the
// hub) SIGTERMs the hub subprocess. Idempotent — no-ops if not active.
func (m *Master) Deactivate(name string) error {
	m.mu.Lock()
	core, ok := m.active[name]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.active, name)
	m.mu.Unlock()
	return core.Close()
}

// Restart restarts the named universe in one action: this GUI's
// core closed, the universe's hub and every member it spawned ended - also a
// hub the GUI found running and does not own, which a deactivation leaves
// running -, then the universe activated again on a new hub, each program
// reading its keys, config and pinned version anew. A universe this GUI does
// not run is activated.
func (m *Master) Restart(ctx context.Context, name string) (*ActiveCore, error) {
	m.mu.RLock()
	core, ok := m.active[name]
	m.mu.RUnlock()
	if !ok {
		return m.Activate(ctx, name)
	}
	if err := m.Deactivate(name); err != nil {
		return nil, fmt.Errorf("restart %q: close: %w", name, err)
	}
	if err := m.stopHub(ctx, core.HomePath); err != nil {
		return nil, fmt.Errorf("restart %q: the hub did not stop: %w", name, err)
	}
	return m.Activate(ctx, name)
}

// stopUniverseHub ends the hub the universe at home runs on, found by its
// lockfile, and the members it spawned; nothing when none runs.
func stopUniverseHub(ctx context.Context, home string) error {
	paths, err := resolveUniversePaths(home)
	if err != nil {
		return err
	}
	_, err = hubclient.Stop(ctx, paths.DataDir)
	return err
}

// Close tears down all active cores. Called on ktw-gui app shutdown so
// hub processes spawned by us don't outlive the GUI.
func (m *Master) Close() {
	m.mu.Lock()
	cores := make([]*ActiveCore, 0, len(m.active))
	for _, c := range m.active {
		cores = append(cores, c)
	}
	m.active = map[string]*ActiveCore{}
	m.mu.Unlock()
	for _, c := range cores {
		_ = c.Close()
	}
}

// watch removes the core from the map when its hub connection signals
// exit. v1 does NOT auto-restart — manual re-activate is the recovery
// path. Auto-restart with backoff is a v2 enhancement.
func (m *Master) watch(name string, core *ActiveCore) {
	<-core.exited
	m.mu.Lock()
	defer m.mu.Unlock()
	// Remove only if it's still THIS core (a race with explicit Deactivate
	// could have already cleared it).
	if cur, ok := m.active[name]; ok && cur == core {
		delete(m.active, name)
		log.Printf("[supervisor] universe %q hub connection exited unexpectedly: %v", name, core.exitErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// ActiveCore lifecycle
// ─────────────────────────────────────────────────────────────────────────

// spawnCore opens a connection to the ktw-hub serving this universe
// (spawning a hub when none is running), then builds the messagebus
// Sender that routes slot calls into the hub's NATS bus.
//
// The universe's DataDir (where ktw-hub writes its lockfile) is resolved
// from the universe's KTW_WORKSPACE via Settings.yaml + Univers.yaml + XDG
// defaults — mirroring what the spawned hub itself computes.
//
// The spawned hub inherits ktw-gui's environment with KTW_WORKSPACE set to
// the universe folder, so all downstream resolution (sidecar specs,
// embedded NATS dir, lockfile path, .katyoti/config/Univers.yaml) sees the
// right universe scope.
//
// No code-side migrator: workspaces in the pre-cutover layout (with
// config/Univers.yaml at the universe root) must be migrated manually
// before activation.
// ClientVersion is this GUI's own version (set by main): a universe's hub found
// running at another version is reported (hubclient.HubVersionSkew).
var ClientVersion string

func spawnCore(ctx context.Context, name, homePath string) (*ActiveCore, error) {
	paths, err := resolveUniversePaths(homePath)
	if err != nil {
		return nil, fmt.Errorf("resolve universe paths for %q: %w", name, err)
	}

	hub, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir:       paths.DataDir,
		Mode:          hubclient.ModeFindOrSpawn,
		SpawnEnv:      hubclient.SpawnEnvForWorkspace(homePath),
		ExpectVersion: ClientVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to hub for universe %q (dataDir=%s): %w", name, paths.DataDir, err)
	}

	// Wrap the bus sender with A-RETRY lazy-spawn: on the first
	// ErrSlotNotFound, the wrapper calls
	// `ktw-hub:/conn-mgr/ensure-spawned({name})` and retries the
	// original Send once. Cold path: 2 round-trips. Warm path:
	// direct. Without this wrapper, ktw-gui's first slot call against
	// a not-yet-spawned sidecar would just fail.
	innerSender := messagebus.NewSender(hub.MessageBus(), activeTenantID, name)
	sender := messagebus.NewRetrySender(innerSender, "ktw-hub")

	core := &ActiveCore{
		Name:      name,
		HomePath:  homePath,
		StartedAt: time.Now(),
		HubFound:  !hub.Spawned(),
		hub:       hub,
		sender:    sender,
		exited:    make(chan struct{}),
	}

	// We don't currently get a "hub process died" signal from hubclient
	// — the NATS adapter reconnects automatically. The `exited` channel
	// is closed only by `Close()` for now; a future hubclient extension
	// can wire genuine death detection (e.g. via lockfile watch). Until
	// then, watch() effectively only fires on explicit Deactivate /
	// Close, which is the correct semantic for v1.
	return core, nil
}

// Close closes the hub connection (and, if ktw-gui owns the hub
// subprocess, SIGTERMs it with grace). Idempotent.
func (c *ActiveCore) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	hub := c.hub
	router := c.router
	wsSub := c.wsSub
	catalogSub := c.catalogSub
	runsSub := c.runsSub
	adminSubs := c.adminSubs
	c.mu.Unlock()

	if router != nil {
		router.Close()
	}
	for _, sub := range adminSubs {
		_ = sub.Unsubscribe()
	}
	if wsSub != nil {
		_ = wsSub.Unsubscribe()
	}
	if catalogSub != nil {
		_ = catalogSub.Unsubscribe()
	}
	if runsSub != nil {
		_ = runsSub.Unsubscribe()
	}

	var err error
	if hub != nil {
		err = hub.Close()
	}
	c.exitErr = err
	close(c.exited)
	return err
}

// Sender returns the bus-backed KTSP sender for this core. Slot calls
// (Terminal/start-task, resources/list, …) route through it onto the
// `ktw.local.universe.<name>.slot.<slot>.<method>` subject family.
func (c *ActiveCore) Sender() ktsp.Sender {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sender
}

// MessageBus exposes the raw bus underlying this core. The terminal
// router uses it directly to install per-session subscriptions on
// streaming event subjects — slot req/rep goes through Sender(), but
// fan-out subscriptions need bus-level access.
func (c *ActiveCore) MessageBus() mbapi.MessageBus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.hub == nil {
		return nil
	}
	return c.hub.MessageBus()
}
