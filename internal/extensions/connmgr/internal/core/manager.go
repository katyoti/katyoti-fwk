// SPDX-License-Identifier: Apache-2.0

// Package core holds the in-process ConnectionManager implementation.
//
// First impl pass: a thin, FSM-tracking wrapper over the existing
// stdio-spawn path (appcore.SidecarClient). Every (name) entry is
// allocated once at construction (from Univers.yaml membership) in
// DECLARED state and transitions through SPAWNING / READY / FAILED
// driven by Acquire calls.
//
// Cutover (replacing SidecarClient call sites with ConnectionManager.Acquire)
// + idle-timer + restart policy + ExternalProcess unification land in
// sibling todos.
package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// streamSidecarStderr forwards the spawned sidecar's stderr to the parent's
// stderr, prefixing each line with the sidecar name. Used by spawnNATS so
// readiness-probe failures (the sidecar prints an actionable error and
// exits, but the ping just times out) aren't silently swallowed.
//
// Runs in a goroutine until the pipe closes (cmd exits or pipe is reset).
// All errors are logged + ignored — this is a diagnostic shim, not a
// correctness path.
func streamSidecarStderr(r io.Reader, name string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		log.Printf("[%s] %s", name, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("[%s] stderr scan error: %v", name, err)
	}
}

// stdioConnection holds the live transport for one DECLARED member.
// nil sender + DECLARED state = never spawned. Non-nil sender + READY
// state = spawned and connected.
//
// Despite the name (legacy from the stdio-only first impl pass), this
// struct now backs both `transport: stdio` (owns `*ktransport.Session`)
// AND `transport: nats` (owns `*exec.Cmd` for the spawned subprocess
// + a stateless messagebus.Sender). The `transport` field is non-nil
// for stdio mode; `cmd` is non-nil for nats mode.
type stdioConnection struct {
	mu        sync.Mutex
	spec      cmapi.SpawnSpec
	state     cmapi.ConnState
	refCount  int
	transport *ktransport.Session // stdio mode only
	cmd       *exec.Cmd           // nats mode only — sidecar subprocess
	// exited is closed when cmd's process has exited (nats mode; see
	// watchExit, the process's one Wait).
	exited    chan struct{}
	sender    ktsp.Sender
	spawnedAt time.Time
	lastCall  time.Time
	lastErr   string

	// idleTimer is non-nil when the connection is armed for idle
	// disconnect — i.e. refCount has dropped to 0 with IdleTimeout > 0
	// and no Pin / Lifecycle:stateful exemption. Cancelled on Acquire.
	idleTimer *time.Timer
	// epoch is bumped every time the timer is armed so a fired-but-
	// queued callback from a previous arm cycle can detect that the
	// connection has since been re-acquired or re-armed and bail out.
	epoch uint64

	// restartCount counts spawn failures within the active rolling
	// window. Reset to 0 on a successful spawn AND on window-expiry.
	restartCount int
	// restartWindowStart marks when the current rolling window opened
	// (timestamp of the first failure in the window). Window expiry
	// is `now - restartWindowStart > spec.MaxRestartWindow`.
	restartWindowStart time.Time

	// spawning is non-nil while a spawn is in flight; it is closed when that
	// spawn settles (READY or FAILED). A concurrent Acquire waits on it and
	// shares the outcome instead of starting a second process: at a GUI's
	// first paint several panels miss the same cold sidecar at once, and each
	// used to spawn its own copy — the copies raced for what a member holds
	// alone (a CWM workspace lock admits one) and the losers died at boot.
	spawning chan struct{}
	// spawnCancel cancels the start in flight (Close); nil when none runs.
	spawnCancel context.CancelFunc

	// startBackoff is how long, from failedAt, a member whose last start
	// failed (a crash, a readiness timeout) refuses a new start. Each failed
	// start doubles it (startBackoffBase … startBackoffCap); a successful
	// start clears it. Without it every caller that missed the member
	// started it again at once, and a member that could not come up was
	// started and killed in a loop, each start paying the whole readiness
	// wait. backoffLogged says this window's refusal was logged already.
	failedAt      time.Time
	startBackoff  time.Duration
	backoffLogged bool
}

// The back-off a failed start opens before the member may be started again:
// doubled per consecutive failed start, capped, cleared by a success.
// Variables, not constants, so a test can shrink them.
var (
	startBackoffBase = 2 * time.Second
	startBackoffCap  = 60 * time.Second
)

// nextStartBackoff is the back-off after one more failed start.
func nextStartBackoff(prev time.Duration) time.Duration {
	if prev <= 0 {
		return startBackoffBase
	}
	next := prev * 2
	if next > startBackoffCap {
		next = startBackoffCap
	}
	return next
}

// now is the manager's clock, behind a seam so the back-off test can move it.
var now = time.Now

// ErrStartBackoff is returned by Acquire while a member whose last start
// failed is in its back-off window: the start is refused, not attempted.
var ErrStartBackoff = errors.New("connmgr: start refused, the last start failed (back-off)")

// BusScope captures the (bus, tenant, universe) triple needed to spawn
// sidecars in `transport: nats` mode. NewManagerWithBus stores it; nil
// means "stdio-only manager" (existing behaviour for tests + ktw-hub
// pre-NATS-cutover).
type BusScope struct {
	Bus        mbapi.MessageBus
	TenantID   string
	UniverseID string
	// KeyFor, when set, is the path of the bus key a member connects with
	// (busauth), minted when it has none: spawnNATS hands it to the member
	// as busauth.KeyEnv. Unset on a bus that checks no keys.
	KeyFor func(member string) (string, error)
	// ServingHolder, when set, names the process holding member's serving
	// lock for the universe ("" when none does): a member that does not become
	// ready because another process serves its provider is told so, that
	// process named, in the readiness error.
	ServingHolder func(member string) string
}

// ManagerImpl is the first-impl-pass ConnectionManager. Maintains one
// stdioConnection per declared member; routes Acquire to the right one.
// Concurrency: the outer map is RWMutex-protected for membership
// changes; per-connection state mutates under the inner stdioConnection.mu.
type ManagerImpl struct {
	mu      sync.RWMutex
	conns   map[string]*stdioConnection
	members []string  // declared order, for All()
	bus     *BusScope // nil = stdio-only (no nats spawn capability)
}

// NewManager builds a stdio-only ConnectionManager populated with one
// DECLARED entry per spec in members. The order of members is preserved
// by All(). Sidecars with `transport: nats` will fail at Acquire — use
// NewManagerWithBus to enable nats-mode spawning.
//
// The names in members must match the SpawnSpec.Name values; a duplicate
// or empty name returns an error.
func NewManager(members []cmapi.SpawnSpec) (*ManagerImpl, error) {
	return newManager(members, nil)
}

// NewManagerWithBus builds a ConnectionManager that can spawn sidecars
// in either `transport: stdio` (legacy) or `transport: nats` (NATS bus).
// The bus must already be connected; tenant + universe identify the
// scope for SlotSubject construction.
func NewManagerWithBus(members []cmapi.SpawnSpec, scope BusScope) (*ManagerImpl, error) {
	if scope.Bus == nil {
		return nil, fmt.Errorf("connmgr: NewManagerWithBus: nil Bus")
	}
	if scope.TenantID == "" {
		return nil, fmt.Errorf("connmgr: NewManagerWithBus: empty TenantID")
	}
	if scope.UniverseID == "" {
		return nil, fmt.Errorf("connmgr: NewManagerWithBus: empty UniverseID")
	}
	return newManager(members, &scope)
}

func newManager(members []cmapi.SpawnSpec, scope *BusScope) (*ManagerImpl, error) {
	m := &ManagerImpl{
		conns:   make(map[string]*stdioConnection, len(members)),
		members: make([]string, 0, len(members)),
		bus:     scope,
	}
	for _, spec := range members {
		if spec.Name == "" {
			return nil, fmt.Errorf("connmgr: spec with empty Name")
		}
		if _, dup := m.conns[spec.Name]; dup {
			return nil, fmt.Errorf("connmgr: duplicate member %q", spec.Name)
		}
		m.conns[spec.Name] = &stdioConnection{
			spec:  spec,
			state: cmapi.ConnDeclared,
		}
		m.members = append(m.members, spec.Name)
	}
	return m, nil
}

// SetMembers replaces the membership list, preserving live entries
// whose name remains in the new list. Removed entries have their
// transports closed. A running member whose spawn changed - another
// binary (a pinned version moved), its args, its env, a refusal - is
// started again with it: it would run the old program
// until it idled out.
func (m *ManagerImpl) SetMembers(specs []cmapi.SpawnSpec) error {
	changed, err := m.setMembers(specs)
	if err != nil {
		return err
	}
	for _, name := range changed {
		ctx, cancel := context.WithTimeout(context.Background(), memberRestartWait)
		_, _ = m.Restart(ctx, name) // a refused or failing start is its state, said by Status
		cancel()
	}
	return nil
}

// memberRestartWait bounds the restart of a member whose spawn changed.
const memberRestartWait = 3 * time.Minute

// spawnChanged is whether b starts another process than a.
func spawnChanged(a, b cmapi.SpawnSpec) bool {
	return a.Binary != b.Binary || a.Transport != b.Transport || a.Folder != b.Folder ||
		a.Refused != b.Refused || !slices.Equal(a.Args, b.Args) || !maps.Equal(a.Env, b.Env)
}

// setMembers swaps the membership and answers the running members whose
// spawn changed.
func (m *ManagerImpl) setMembers(specs []cmapi.SpawnSpec) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var changed []string
	newConns := make(map[string]*stdioConnection, len(specs))
	newMembers := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == "" {
			return nil, fmt.Errorf("connmgr: spec with empty Name")
		}
		if existing, ok := m.conns[spec.Name]; ok {
			// Preserve live state — just update spec.
			existing.mu.Lock()
			if existing.state == cmapi.ConnReady && existing.sender != nil && spawnChanged(existing.spec, spec) {
				changed = append(changed, spec.Name)
			}
			existing.spec = spec
			existing.mu.Unlock()
			newConns[spec.Name] = existing
		} else {
			newConns[spec.Name] = &stdioConnection{spec: spec, state: cmapi.ConnDeclared}
		}
		newMembers = append(newMembers, spec.Name)
	}
	// Close removed entries.
	for name, c := range m.conns {
		if _, kept := newConns[name]; !kept {
			closeConn(c)
		}
	}
	m.conns = newConns
	m.members = newMembers
	return changed, nil
}

// Acquire implements ConnectionManager.
//
// Spawn flow:
//
//	DECLARED|FAILED → SPAWNING → (retry loop)
//	                              ├─ success → READY (return)
//	                              └─ failure → check restart policy
//	                                          ├─ retry → backoff → loop
//	                                          └─ exhausted → FAILED
//
// Retry policy is per-spec via SpawnSpec.MaxRestarts +
// MaxRestartWindow. Default 0 disables retries (failure is immediate).
// `transport: remote` ignores the policy — there's nothing to "respawn"
// on the remote sidecar (its lifecycle is owned elsewhere); a remote
// probe failure is just a one-shot reachability check.
func (m *ManagerImpl) Acquire(ctx context.Context, name string) (ktsp.Sender, error) {
	m.mu.RLock()
	c, ok := m.conns[name]
	bus := m.bus
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", cmapi.ErrSidecarNotInUniverse, name)
	}

	c.mu.Lock()
	if c.spec.Folder != "" && c.spec.Binary == "" {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", cmapi.ErrFolderMember, name)
	}
	if c.spec.Refused != "" {
		reason := c.spec.Refused
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %s: %s", cmapi.ErrMemberRefused, name, reason)
	}
	// Cancel any pending idle disconnect — we're back to busy.
	c.cancelIdleTimerLocked()

	for {
		// Already live → bump refcount + return cached sender.
		if sender, ok := c.takeReadyLocked(); ok {
			c.mu.Unlock()
			return sender, nil
		}

		// A spawn already in flight → wait for it and share its outcome
		// (single-flight per member): one process per member, however many
		// callers missed it at once.
		if wait := c.spawning; wait != nil {
			c.mu.Unlock()
			return c.awaitSpawn(ctx, name, wait, nil)
		}

		// A member whose last start failed is not started again before its
		// back-off has elapsed. A short window left is waited out - the
		// call then starts the member (or joins the start another caller
		// made) instead of failing on a start that may well succeed now; a
		// long one, or one past the caller's deadline, is refused at once.
		// Logged once per window; every refused caller gets the last
		// start's error.
		if c.state == cmapi.ConnFailed && c.startBackoff > 0 {
			if until := c.failedAt.Add(c.startBackoff); now().Before(until) {
				left := until.Sub(now())
				if waitsOutBackoff(ctx, left) {
					c.mu.Unlock()
					select {
					case <-ctx.Done():
						return nil, fmt.Errorf("connmgr: waiting for %s's start back-off: %w", name, ctx.Err())
					case <-time.After(left):
					}
					c.mu.Lock()
					continue
				}
				lastErr, backoff, logged := c.lastErr, c.startBackoff, c.backoffLogged
				c.backoffLogged = true
				c.mu.Unlock()
				if !logged {
					slog.Warn("connmgr: start refused during back-off", "sidecar", name,
						"backoff", backoff, "retry_after", until.Format(time.RFC3339), "last_error", lastErr)
				}
				return nil, fmt.Errorf("%w: %s, retry after %s (%s): %s",
					ErrStartBackoff, name, left.Round(time.Millisecond), backoff, lastErr)
			}
		}
		break
	}

	// Transition DECLARED|FAILED → SPAWNING. The start runs apart from the
	// caller: a caller that gives up (its request timed out, it was
	// cancelled) no longer kills a start that is about to succeed - the
	// start goes on, bounded by the member's readiness limit, and the next
	// caller finds the member READY or joins the start. A start under load
	// used to outlast the caller's wait and be killed by it, so the member
	// never came up. Close cancels a start in flight.
	prev := c.state
	c.state = cmapi.ConnSpawning
	c.lastErr = ""
	settled := make(chan struct{})
	c.spawning = settled
	spawnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.spawnCancel = cancel
	c.mu.Unlock()
	m.emitStateChange(name, prev, cmapi.ConnSpawning, cmapi.ReasonSpawning)

	var spawnErr error
	go func() {
		spawnErr = m.runSpawn(spawnCtx, c, name, bus)
		cancel()
		c.mu.Lock()
		if c.spawning == settled {
			c.spawning = nil
			c.spawnCancel = nil
		}
		c.mu.Unlock()
		// READY or FAILED is written before its waiters are released.
		close(settled)
	}()
	return c.awaitSpawn(ctx, name, settled, &spawnErr)
}

// takeReadyLocked bumps the refcount of a READY member and answers its
// sender; ok is false when the member is not READY. Caller holds c.mu.
func (c *stdioConnection) takeReadyLocked() (ktsp.Sender, bool) {
	if c.sender == nil || c.state != cmapi.ConnReady {
		return nil, false
	}
	c.cancelIdleTimerLocked()
	c.refCount++
	c.lastCall = time.Now()
	return c.sender, true
}

// awaitSpawn waits, within the caller's ctx, for the start settled closes
// and takes the member when it came up. spawnErr, when given, is the
// start's own error (the caller who started it gets it wrapped); a caller
// that joined gets the member's last error.
func (c *stdioConnection) awaitSpawn(ctx context.Context, name string, settled <-chan struct{}, spawnErr *error) (ktsp.Sender, error) {
	select {
	case <-settled:
	case <-ctx.Done():
		return nil, fmt.Errorf("connmgr: waiting for %s to spawn: %w", name, ctx.Err())
	}
	c.mu.Lock()
	if sender, ok := c.takeReadyLocked(); ok {
		c.mu.Unlock()
		return sender, nil
	}
	lastErr := c.lastErr
	c.mu.Unlock()
	if spawnErr != nil && *spawnErr != nil {
		return nil, fmt.Errorf("connmgr: spawn %s: %w", name, *spawnErr)
	}
	return nil, fmt.Errorf("connmgr: spawn %s: %s", name, lastErr)
}

// maxBackoffWait is the longest back-off window a call waits out rather
// than being refused. A variable so a test can change it.
var maxBackoffWait = 10 * time.Second

// waitsOutBackoff says whether a caller waits the back-off window left
// before starting the member: a short window, within the caller's deadline.
func waitsOutBackoff(ctx context.Context, left time.Duration) bool {
	if left > maxBackoffWait {
		return false
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= left {
		return false
	}
	return true
}

// runSpawn runs one start of the member (with the restart policy's
// retries) and writes its outcome: READY with no reference held (each
// waiter takes its own; the idle timer is armed in case none does), or
// FAILED with the back-off opened. ctx is the start's own, not a caller's.
func (m *ManagerImpl) runSpawn(ctx context.Context, c *stdioConnection, name string, bus *BusScope) error {
	// Retry loop. attemptSpawn is mutex-free on the connection — it
	// only touches the spec value (read-only) and the bus.
	for attempt := 0; ; attempt++ {
		sender, transport, cmd, err := spawnAttempt(ctx, c.spec, bus)
		if err == nil {
			c.mu.Lock()
			c.transport = transport
			c.cmd = cmd
			if cmd != nil {
				c.exited = m.watchExit(c, cmd)
			}
			c.sender = sender
			c.state = cmapi.ConnReady
			c.refCount = 0
			c.spawnedAt = time.Now()
			c.lastCall = c.spawnedAt
			c.restartCount = 0 // reset on success
			c.restartWindowStart = time.Time{}
			c.startBackoff = 0 // a start that succeeded clears the back-off
			c.failedAt = time.Time{}
			if c.shouldArmIdleTimerLocked() {
				// Every caller may have given up: the member is reaped as
				// idle unless a waiter takes it first.
				c.armIdleTimerLocked(func() { m.idleDisconnect(c) })
			}
			c.mu.Unlock()
			m.emitStateChange(name, cmapi.ConnSpawning, cmapi.ConnReady, cmapi.ReasonReady)
			return nil
		}

		// Failure path. Decide retry vs. final-fail.
		c.mu.Lock()
		retry, backoff := decideRetryLocked(c, time.Now())
		if !retry {
			c.state = cmapi.ConnFailed
			c.lastErr = err.Error()
			// The member failed to start, not its start cancelled (the
			// manager closed): open (or widen) the back-off. A remote
			// member is not started here.
			if ctx.Err() == nil && c.spec.Transport != "remote" {
				c.startBackoff = nextStartBackoff(c.startBackoff)
				c.failedAt = now()
				c.backoffLogged = false
			}
			c.mu.Unlock()
			m.emitStateChange(name, cmapi.ConnSpawning, cmapi.ConnFailed, cmapi.ReasonSpawnFail+": "+err.Error())
			return err
		}
		c.mu.Unlock()

		// Sleep with cancellation awareness.
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.state = cmapi.ConnFailed
			c.lastErr = ctx.Err().Error()
			c.mu.Unlock()
			m.emitStateChange(name, cmapi.ConnSpawning, cmapi.ConnFailed, cmapi.ReasonSpawnFail+": "+ctx.Err().Error())
			return fmt.Errorf("cancelled (attempt %d): %w", attempt+1, ctx.Err())
		case <-time.After(backoff):
		}
	}
}

// spawnAttempt is the spawn Acquire runs — attemptSpawn, behind a seam so
// the single-flight test can count and hold spawns without starting processes.
var spawnAttempt = attemptSpawn

// attemptSpawn runs a single spawn attempt for the given spec and bus
// scope. Pure-ish — no connection state mutation. Returns the live
// (sender, transport, cmd) on success or err on failure.
//
// Transport semantics:
//   - "" or "stdio" → exec via stdio pipes; transport is the live session,
//     cmd is nil (the session owns the *exec.Cmd internally).
//   - "nats" → exec `<binary> sc ktsp nats`, poll readiness; cmd tracks the
//     subprocess so closeConn can SIGTERM.
//   - "remote" → no spawn; build messagebus.Sender + ping probe.
//   - anything else → unsupported-transport error.
func attemptSpawn(
	ctx context.Context,
	spec cmapi.SpawnSpec,
	bus *BusScope,
) (ktsp.Sender, *ktransport.Session, *exec.Cmd, error) {
	switch spec.Transport {
	case "", "stdio":
		sender, transport, err := spawnStdio(spec)
		if err != nil {
			return nil, nil, nil, err
		}
		return sender, transport, nil, nil
	case "nats":
		if bus == nil {
			return nil, nil, nil, fmt.Errorf("transport=nats but manager has no bus scope (use NewManagerWithBus)")
		}
		sender, cmd, err := spawnNATS(ctx, spec, *bus)
		if err != nil {
			return nil, nil, nil, err
		}
		// The messagebus.Sender requires a `<provider>:` prefix; bind it so
		// the connection honours the local ktsp.Sender path contract.
		return newLocalScopedSender(sender, busProvider(spec)), nil, cmd, nil
	case "remote":
		if bus == nil {
			return nil, nil, nil, fmt.Errorf("transport=remote but manager has no bus scope")
		}
		if spec.Provider == "" {
			return nil, nil, nil, fmt.Errorf("transport=remote requires SpawnSpec.Provider (the sidecar's self-declared bus identity; %q is a connmgr-local label only)", spec.Name)
		}
		sender, err := connectRemote(ctx, *bus, spec.Provider, spec.ReadyTimeout)
		if err != nil {
			return nil, nil, nil, err
		}
		return newLocalScopedSender(sender, spec.Provider), nil, nil, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported transport %q", spec.Transport)
	}
}

// decideRetryLocked applies the restart policy and returns (retry,
// backoff). Caller must hold c.mu. Updates c.restartCount and
// c.restartWindowStart as a side effect.
//
// Policy (per analysis §3.5):
//   - MaxRestarts == 0 → never retry (default; fail-fast).
//   - transport == "remote" → never retry (sidecar lifecycle owned elsewhere;
//     reachability failure is one-shot).
//   - Window expiry: if the oldest failure in the window is older than
//     MaxRestartWindow, reset the counter and start a new window.
//   - Backoff: exponential with base 200ms, capped at 2s. Stochastic
//     jitter is deferred — fixed schedule is fine for low-frequency
//     restarts in a single host.
func decideRetryLocked(c *stdioConnection, now time.Time) (retry bool, backoff time.Duration) {
	if c.spec.Transport == "remote" {
		return false, 0
	}
	if c.spec.MaxRestarts <= 0 {
		return false, 0
	}

	window := c.spec.MaxRestartWindow
	if window <= 0 {
		window = 5 * time.Minute // analysis §3.5 default
	}

	// Window expiry: reset the counter if the oldest failure is stale.
	if !c.restartWindowStart.IsZero() && now.Sub(c.restartWindowStart) > window {
		c.restartCount = 0
		c.restartWindowStart = time.Time{}
	}

	// Window opens on the first failure.
	if c.restartWindowStart.IsZero() {
		c.restartWindowStart = now
	}

	if c.restartCount >= c.spec.MaxRestarts {
		return false, 0
	}
	attempt := c.restartCount
	c.restartCount++

	// Exponential backoff: 200ms, 400ms, 800ms, 1.6s, capped at 2s.
	const base = 200 * time.Millisecond
	const cap = 2 * time.Second
	d := base << attempt //nolint:gosec — small attempt count
	if d > cap {
		d = cap
	}
	return true, d
}

// Release implements ConnectionManager.
func (m *ManagerImpl) Release(name string) {
	m.mu.RLock()
	c, ok := m.conns[name]
	m.mu.RUnlock()
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refCount > 0 {
		c.refCount--
	}
	if c.refCount == 0 && c.state == cmapi.ConnReady && c.shouldArmIdleTimerLocked() {
		c.armIdleTimerLocked(func() { m.idleDisconnect(c) })
	}
}

// shouldArmIdleTimerLocked returns true when the connection is eligible
// for idle disconnect — IdleTimeout > 0 AND not pinned AND not stateful.
// Caller must hold c.mu.
func (c *stdioConnection) shouldArmIdleTimerLocked() bool {
	if c.spec.IdleTimeout <= 0 {
		return false
	}
	if c.spec.Pin {
		return false
	}
	if c.spec.Lifecycle == cmapi.LifecycleStateful {
		return false
	}
	return true
}

// armIdleTimerLocked starts (or restarts) the idle disconnect timer.
// epoch is bumped so any in-flight callback from a previous arm cycle
// can detect that the timer has been re-armed and bail out cleanly.
// Caller must hold c.mu.
func (c *stdioConnection) armIdleTimerLocked(fire func()) {
	c.cancelIdleTimerLocked()
	c.epoch++
	thisEpoch := c.epoch
	c.idleTimer = time.AfterFunc(c.spec.IdleTimeout, func() {
		c.mu.Lock()
		stale := c.epoch != thisEpoch
		c.mu.Unlock()
		if stale {
			return
		}
		fire()
	})
}

// cancelIdleTimerLocked stops a pending idle timer if one is armed.
// Caller must hold c.mu.
func (c *stdioConnection) cancelIdleTimerLocked() {
	if c.idleTimer == nil {
		return
	}
	c.idleTimer.Stop()
	c.idleTimer = nil
	c.epoch++
}

// idleDisconnect is the timer callback. Verifies the connection is
// still eligible (refcount may have grown back; another arm may have
// cancelled and re-armed) before transitioning IDLE_DRAINING → DECLARED
// via closeConn (which terminates the transport).
func (m *ManagerImpl) idleDisconnect(c *stdioConnection) {
	c.mu.Lock()
	if c.refCount > 0 || c.state != cmapi.ConnReady || !c.shouldArmIdleTimerLocked() {
		c.mu.Unlock()
		return
	}
	c.state = cmapi.ConnIdleDraining
	name := c.spec.Name
	c.mu.Unlock()
	m.emitStateChange(name, cmapi.ConnReady, cmapi.ConnIdleDraining, cmapi.ReasonIdle)

	// closeConn re-acquires c.mu and resets transport + state to DECLARED.
	_ = closeConn(c)
	m.emitStateChange(name, cmapi.ConnIdleDraining, cmapi.ConnDeclared, cmapi.ReasonDrained)
}

// Status implements ConnectionManager.
func (m *ManagerImpl) Status(name string) (cmapi.ConnStatus, bool) {
	m.mu.RLock()
	c, ok := m.conns[name]
	m.mu.RUnlock()
	if !ok {
		return cmapi.ConnStatus{}, false
	}
	return c.snapshot(), true
}

// All implements ConnectionManager.
func (m *ManagerImpl) All() []cmapi.ConnStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]cmapi.ConnStatus, 0, len(m.conns))
	for _, name := range m.members {
		if c, ok := m.conns[name]; ok {
			out = append(out, c.snapshot())
		}
	}
	// Members are already in declared order, but sort for stability if
	// someone bypasses the declared-order path.
	sort.SliceStable(out, func(i, j int) bool {
		// Preserve declared order — stable sort with no swaps when
		// everything's already in member order.
		return false
	})
	return out
}

// ErrRemoteRestart refuses to restart a remote member: its process is owned
// elsewhere.
var ErrRemoteRestart = errors.New("connmgr: a remote member is not started by this host")

// Restart implements ConnectionManager: a READY member is stopped (closeConn,
// as Close does) and started again through Acquire, its holders' count kept;
// any other state is left alone and answers false.
func (m *ManagerImpl) Restart(ctx context.Context, name string) (bool, error) {
	m.mu.RLock()
	c, ok := m.conns[name]
	m.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("%w: %s", cmapi.ErrSidecarNotInUniverse, name)
	}
	c.mu.Lock()
	if c.state != cmapi.ConnReady || c.sender == nil {
		c.mu.Unlock()
		return false, nil
	}
	if c.spec.Transport == "remote" {
		c.mu.Unlock()
		return false, fmt.Errorf("%w: %s", ErrRemoteRestart, name)
	}
	holders := c.refCount
	c.mu.Unlock()

	_ = closeConn(c)
	m.emitStateChange(name, cmapi.ConnReady, cmapi.ConnDeclared, cmapi.ReasonRestart)
	if _, err := m.Acquire(ctx, name); err != nil {
		return false, err
	}
	if holders > 0 {
		// Who held the member before holds it still: the count is theirs.
		c.mu.Lock()
		c.refCount = holders
		c.mu.Unlock()
	} else {
		m.Release(name)
	}
	return true, nil
}

// Close implements ConnectionManager.
func (m *ManagerImpl) Close() error {
	m.mu.Lock()
	// Snapshot the live entries that need a transition event so we can
	// emit AFTER releasing the manager lock.
	type pending struct {
		name string
		from cmapi.ConnState
	}
	var toEmit []pending
	conns := make([]*stdioConnection, 0, len(m.conns))
	for _, c := range m.conns {
		c.mu.Lock()
		if c.spawnCancel != nil {
			c.spawnCancel() // a start in flight is given up
		}
		if c.state != cmapi.ConnDeclared {
			toEmit = append(toEmit, pending{name: c.spec.Name, from: c.state})
		}
		c.mu.Unlock()
		conns = append(conns, c)
	}
	m.mu.Unlock()

	var firstErr error
	for _, c := range conns {
		if err := closeConn(c); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Best-effort emit (no manager lock held).
	for _, p := range toEmit {
		m.emitStateChange(p.name, p.from, cmapi.ConnDeclared, cmapi.ReasonClose)
	}
	return firstErr
}

func closeConn(c *stdioConnection) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelIdleTimerLocked()
	var firstErr error
	// stdio mode: SIGTERM the child, then race transport.Close (which
	// closes stdin + waits for the process) against the graceful_shutdown
	// timer. On expiry, SIGKILL to force cmd.Wait to return. Per analysis
	// §3.5 step 2-3 — explicit signal + bounded grace + escalation, vs the
	// older "close stdin and hope" behaviour. Cooperative children honour
	// SIGTERM and exit promptly; misbehaving ones get force-killed.
	if c.transport != nil {
		grace := c.spec.GracefulShutdown
		if grace <= 0 {
			grace = natsShutdownGrace
		}
		// SIGTERM the child first if the transport exposes its cmd.
		// `signalProcess` is the cross-platform helper (SIGTERM on Unix,
		// Kill on Windows where SIGTERM isn't natively supported for
		// child processes outside their own process group).
		cmd := c.transport.Cmd()
		if cmd != nil && cmd.Process != nil {
			_ = signalProcess(cmd.Process)
		}
		done := make(chan error, 1)
		go func() { done <- c.transport.Close() }()
		select {
		case err := <-done:
			if err != nil && firstErr == nil {
				firstErr = err
			}
		case <-time.After(grace):
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done // drain so the goroutine doesn't leak
		}
	}
	// nats mode: SIGTERM the spawned subprocess and wait for exit. The
	// sidecar's `sc ktsp nats` command listens for SIGTERM and drains its
	// subscriptions before exiting.
	if c.cmd != nil && c.cmd.Process != nil {
		_ = signalProcess(c.cmd.Process)
		// Wait with a bounded timeout so a stuck sidecar doesn't hang
		// universe deactivation. exec.Cmd.Wait blocks indefinitely; we
		// race it against a timer and force-kill on expiry. Per-spec
		// override via SpawnSpec.GracefulShutdown; default 5s matches
		// analysis §3.5.
		grace := c.spec.GracefulShutdown
		if grace <= 0 {
			grace = natsShutdownGrace
		}
		if c.exited != nil {
			// The exit watcher holds the process's one Wait: wait for it
			// to see the exit (it closes exited before taking c.mu).
			select {
			case <-c.exited:
			case <-time.After(grace):
				_ = c.cmd.Process.Kill()
				<-c.exited
			}
		} else {
			done := make(chan error, 1)
			go func() { done <- c.cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(grace):
				_ = c.cmd.Process.Kill()
				<-done // drain so the goroutine doesn't leak
			}
		}
	}
	c.transport = nil
	c.cmd = nil
	c.exited = nil
	c.sender = nil
	c.refCount = 0
	c.state = cmapi.ConnDeclared
	c.spawnedAt = time.Time{}
	c.lastCall = time.Time{}
	c.lastErr = ""
	return firstErr
}

// natsShutdownGrace is the time we wait for a SIGTERM'd sidecar to exit
// before sending SIGKILL. 5 seconds matches the analysis §3.5 default
// for `graceful_shutdown`.
const natsShutdownGrace = 5 * time.Second

// emitStateChange publishes a ConnStateChangedEvent on the universe's
// connstate-changed subject. Best-effort — no error path: a dead bus
// must not block lifecycle work, and a publish failure here is far
// less important than the actual state transition that just happened.
//
// Skipped entirely when the manager has no bus scope (stdio-only
// mode); the event surface is NATS-only by design.
//
// Caller must NOT hold any connection mutex when calling this — the
// publish path can block briefly on the bus, and a stuck publish
// shouldn't cascade into manager-wide deadlock.
func (m *ManagerImpl) emitStateChange(name string, from, to cmapi.ConnState, reason string) {
	scope := m.bus
	if scope == nil {
		return
	}
	evt := cmapi.ConnStateChangedEvent{
		Name:   name,
		From:   from,
		To:     to,
		Reason: reason,
		At:     time.Now().UTC(),
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	subject := subjects.ConnStateChangedSubject(scope.TenantID, scope.UniverseID)
	// Use Background — the lifecycle event isn't tied to any caller's
	// context, and we don't want a cancelled Acquire ctx to also drop
	// the "FAILED" event that explains why it was cancelled.
	_ = scope.Bus.Publish(context.Background(), subject, payload)
}

func (c *stdioConnection) snapshot() cmapi.ConnStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cmapi.ConnStatus{
		Name:               c.spec.Name,
		State:              c.state,
		RefCount:           c.refCount,
		SpawnedAt:          c.spawnedAt,
		LastCallAt:         c.lastCall,
		Error:              c.lastErr,
		RestartCount:       c.restartCount,
		RestartWindowStart: c.restartWindowStart,
		RetryAfter:         c.retryAfterLocked(),
	}
}

// retryAfterLocked is when a member in its start back-off may be started
// again; zero outside a back-off. Caller holds c.mu.
func (c *stdioConnection) retryAfterLocked() time.Time {
	if c.state != cmapi.ConnFailed || c.startBackoff <= 0 {
		return time.Time{}
	}
	return c.failedAt.Add(c.startBackoff)
}

// ktspArgs is the child's argv for a KTSP session: what the spec declares, or
// the plain invocation when it declares nothing.
//
// `SpawnSpec.Args` is the WHOLE argv, and every production writer treats it
// that way — including the content-pack path, which writes
// `["--folder", <path>, "sc", "ktsp", "nats"]`: the host's flags AND the
// invocation, in one list (marketplace/internal/wire).
//
// This briefly sniffed for a "ktsp" element and appended the invocation when it
// found none, to accommodate a TEST that declared only `--folder <path>`. That
// was the wrong direction — loosening a contract five writers already keep, to
// fit the one caller that did not. The test was fixed instead and the sniff is
// gone.
func ktspArgs(declared []string, transport string) []string {
	if len(declared) == 0 {
		return []string{"sc", "ktsp", transport}
	}
	return declared
}

// spawnStdio handles `transport: stdio` (or empty, the legacy default).
// The spawned subprocess runs `<binary> sc ktsp stdio` and the parent owns
// the JSON-RPC session over the child's stdin/stdout pipes.
func spawnStdio(spec cmapi.SpawnSpec) (ktsp.Sender, *ktransport.Session, error) {
	if spec.Binary == "" {
		return nil, nil, fmt.Errorf("connmgr: empty Binary for %s", spec.Name)
	}
	args := ktspArgs(spec.Args, "stdio")
	transport, err := ktransport.StartStdio(spec.Binary, args...)
	if err != nil {
		return nil, nil, err
	}
	return ktsp.NewSender(transport), transport, nil
}

// spawnNATS handles `transport: nats`. Execs `<binary> sc ktsp nats` with
// the parent's environment (so KTW_WORKSPACE / KTW_NATS_URL etc. propagate)
// and waits for the sidecar to subscribe by polling `ktsp/ping` over
// the bus until it returns or the readiness timeout expires. Returns a
// messagebus.Sender that routes future calls via NATS.
//
// On readiness-poll failure, the spawned subprocess is killed before
// the error returns so we don't leak a child.
func spawnNATS(ctx context.Context, spec cmapi.SpawnSpec, scope BusScope) (ktsp.Sender, *exec.Cmd, error) {
	if spec.Binary == "" {
		return nil, nil, fmt.Errorf("connmgr: empty Binary for %s", spec.Name)
	}
	args := ktspArgs(spec.Args, "nats")

	cmd := exec.Command(spec.Binary, args...)
	publish.NoConsoleWindow(cmd) // a sidecar started by the hub opens no terminal window
	// Inherit parent env so KTW_WORKSPACE / KTW_NATS_URL / KTW_TENANT_ID flow
	// through — without the keys the hub (or what started it) took from the
	// universe's or the global .env: the member reads those files itself, so a
	// key saved since the hub started reaches it (cenv.ChildEnviron). Per-spec
	// overrides go on top via spec.Env.
	cmd.Env = cenv.ChildEnviron(os.Environ())
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Its own bus key, on a bus that checks keys.
	if scope.KeyFor != nil {
		path, err := scope.KeyFor(spec.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("the bus key of %s: %w", spec.Name, err)
		}
		cmd.Env = append(cmd.Env, busauth.KeyEnv+"="+path)
	}
	cmd.Stdout = nil
	// Capture stderr through a pipe and forward it line-by-line to the
	// parent's stderr (prefixed by sidecar name). Without this, a spawned
	// sidecar that prints an actionable error and exits is indistinguishable
	// from one that's still booting — the readiness probe just times out
	// without any clue what went wrong.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe for %s: %w", spec.Name, err)
	}
	go streamSidecarStderr(stderr, spec.Name)

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("exec %s: %w", spec.Binary, err)
	}

	// Attach to the parent's Windows Job Object so the sidecar dies with
	// us even on parent crash / force-kill. No-op on POSIX (process group
	// + ctx cancellation already cascade). Best-effort: log + continue if
	// the attach fails — refusing to spawn would be worse.
	if err := publish.AttachToParentJob(cmd.Process); err != nil {
		log.Printf("[connmgr] failed to attach %s (pid=%d) to parent job: %v — orphan possible on parent crash",
			spec.Name, cmd.Process.Pid, err)
	}

	// Build the messagebus.Sender for the active scope. We can construct
	// it before the sidecar is ready — it's just a struct around the bus.
	sender := messagebus.NewSender(scope.Bus, scope.TenantID, scope.UniverseID)

	// Poll for readiness via the built-in `ktsp/ping` method. Every
	// `ktsp.Endpoint` registers this slot automatically (see
	// container.go::registerKTSPSlot), so it's the cheapest "is the
	// sidecar subscribed yet?" signal we have.
	//
	// Path uses the named-slot URI form post bus-named-slot-addressing:
	// `<provider>:/ktsp/ping`. The 2-token legacy form ("ktsp/ping")
	// no longer parses.
	probePath := spec.Name + ":ktsp/ping"
	if err := waitForNATSReady(ctx, sender, probePath, spec.ReadyTimeout); err != nil {
		err = withServingHolder(err, scope, spec.Name, cmd)
		// Kill the orphan and report a clean error.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
		return nil, nil, fmt.Errorf("ready-probe %s: %w", spec.Name, err)
	}

	return sender, cmd, nil
}

// connectRemote handles `transport: remote`. No subprocess is spawned —
// the sidecar is assumed to already be running on the same NATS bus
// (cloud cluster member, sibling host, persistent service). We build a
// messagebus.Sender for the active scope and probe `ktsp/ping` for
// reachability before returning READY.
//
// Failure modes:
//   - Sidecar not subscribed → ErrNoResponders → mapped via the
//     readiness probe to a clean failure with FAILED state.
//   - Sidecar subscribed but unhealthy → probe times out, FAILED.
//
// Close is a no-op for remote — the sidecar's lifecycle is owned
// elsewhere.
func connectRemote(ctx context.Context, scope BusScope, providerName string, readyTimeout time.Duration) (ktsp.Sender, error) {
	sender := messagebus.NewSender(scope.Bus, scope.TenantID, scope.UniverseID)
	probePath := providerName + ":ktsp/ping"
	if err := waitForNATSReady(ctx, sender, probePath, readyTimeout); err != nil {
		return nil, fmt.Errorf("remote sidecar unreachable: %w", err)
	}
	return sender, nil
}

// readyProbeTimeout is how long one readiness ping may take.
const readyProbeTimeout = 2 * time.Second

// waitForNATSReady polls `ktsp/ping` over the messagebus.Sender until
// the sidecar responds or the deadline expires. The poll interval is
// short (50ms) because the spawn is already in progress and we want to
// minimise the perceived first-call latency. Total budget is bounded
// by the parent ctx + the local hard deadline below.
// waitForNATSReady polls `<probePath>` until it returns success or the
// deadline elapses. The path MUST be a fully-qualified URI form per
// bus-named-slot-addressing (e.g., `ktw-agents:/ktsp/ping`) — the
// legacy 2-token form no longer parses.
//
// readyTimeout is the member's declared limit (SpawnSpec.ReadyTimeout); zero
// keeps the default, DefaultReadyTimeout.
func waitForNATSReady(parent context.Context, sender ktsp.Sender, probePath string, readyTimeout time.Duration) error {
	// A ping is given up to readyProbeTimeout: a member busy serving others
	// (several agents' loops) answers late but is alive, and a start that
	// counts it as dead kills it and restarts it - a longer start under the
	// same load.
	const pollInterval = 50 * time.Millisecond
	probeTimeout := readyProbeTimeout
	if readyTimeout <= 0 {
		readyTimeout = cmapi.DefaultReadyTimeout
	}
	ctx, cancel := context.WithTimeout(parent, readyTimeout)
	defer cancel()

	var lastErr error
	for {
		probeCtx, probeCancel := context.WithTimeout(ctx, probeTimeout)
		_, err := sender.Call(probeCtx, probePath, nil)
		probeCancel()
		if err == nil {
			return nil
		}
		lastErr = err

		// Stop polling on terminal errors that won't go away on retry —
		// e.g. the bus is closed. ErrSlotNotFound from the wire is the
		// "not subscribed yet" signal — keep polling.
		if errors.Is(err, context.Canceled) {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("readiness deadline elapsed (last err: %w)", lastErr)
		case <-time.After(pollInterval):
		}
	}
}

// Compile-time interface assertion.
var _ cmapi.ConnectionManager = (*ManagerImpl)(nil)

// withServingHolder adds to a member's readiness error the process holding
// its serving lock, when one other than the member just spawned holds it: a
// member that never became ready because another serves its provider - a hub
// running unregistered beside this one, a hand-started `sc ktsp nats` - is
// told apart from one that is only slow.
func withServingHolder(err error, scope BusScope, member string, cmd *exec.Cmd) error {
	if scope.ServingHolder == nil {
		return err
	}
	holder := scope.ServingHolder(member)
	if holder == "" || (cmd != nil && cmd.Process != nil && holderIsPid(holder, cmd.Process.Pid)) {
		return err
	}
	return fmt.Errorf("%w - the serving lock of %s in this universe is held by %s: stop that process (or the hub that started it) and retry", err, member, holder)
}

// holderIsPid reports a holder description that names pid ("pid N…").
func holderIsPid(holder string, pid int) bool {
	rest, ok := strings.CutPrefix(holder, fmt.Sprintf("pid %d", pid))
	return ok && (rest == "" || !strings.ContainsAny(rest[:1], "0123456789"))
}
