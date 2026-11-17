// SPDX-License-Identifier: Apache-2.0

// Package api defines the ConnectionManager contract — the runtime peer
// of the descriptor/catalog track.
//
// Catalog answers "what's there?" (cached descriptors). ConnectionManager
// answers "how do I reach a live instance?" (spawn-aware Sender provider).
//
// Per ADR (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md) and §3.1 + §3.3 of the analysis, the manager
// owns the full DECLARED → SPAWNING → READY → IDLE_DRAINING → DECLARED
// lifecycle. This first impl pass exposes the API and the FSM; idle-timer
// + restart policy + per-sidecar config knobs land in sibling todos.
package api

import (
	"context"
	"errors"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// ConnState is the lifecycle state of one (universe, sidecar-name) entry.
type ConnState string

// Lifecycle states. The FSM is:
//
//	DECLARED ──spawn request──▶ SPAWNING ──ready──▶ READY
//	   ▲                          │                  │
//	   │                          │ error            │ idle (idle-timer todo)
//	   │                          ▼                  ▼
//	   └──────FAILED ◀───max-restart                 IDLE_DRAINING
//	                                                  │
//	                                                  │ SIGTERM (graceful)
//	                                                  ▼
//	                                                DECLARED
//
// First impl pass implements DECLARED → SPAWNING → READY → DECLARED on
// Close(). FAILED is emitted on spawn errors. IDLE_DRAINING is reserved
// for the idle-timer todo.
const (
	ConnDeclared     ConnState = "DECLARED"
	ConnSpawning     ConnState = "SPAWNING"
	ConnReady        ConnState = "READY"
	ConnIdleDraining ConnState = "IDLE_DRAINING"
	ConnFailed       ConnState = "FAILED"
)

// ConnStatus is the read-only snapshot of one connection. Returned by
// ConnectionManager.Status / All — for GUI Inspector and the CLI.
type ConnStatus struct {
	Name       string    `json:"name"`
	State      ConnState `json:"state"`
	RefCount   int       `json:"ref_count"`
	SpawnedAt  time.Time `json:"spawned_at,omitempty"`
	LastCallAt time.Time `json:"last_call_at,omitempty"`
	// Error is populated on FAILED state with the most recent spawn error.
	Error string `json:"error,omitempty"`

	// RestartCount is the number of spawn retries within the active
	// rolling window (non-zero only when the spec declares
	// MaxRestarts > 0 and the connection has hit at least one failure).
	// Reset to 0 on successful spawn and on window expiry. Useful for
	// dashboards: a non-zero value while State==READY means the
	// connection recovered after a partial fault.
	RestartCount int `json:"restart_count,omitempty"`

	// RestartWindowStart is the timestamp of the first failure in the
	// active rolling window. Zero when no window is open. Operators
	// can compute "time until window expiry" as
	// `RestartWindowStart + spec.MaxRestartWindow - now`.
	RestartWindowStart time.Time `json:"restart_window_start,omitempty"`

	// RetryAfter is set while the member is in its start back-off: its
	// last start failed, and Acquire refuses a new one until then (the
	// back-off doubles per consecutive failed start, 2 s to 60 s, and a
	// successful start clears it). Zero otherwise.
	RetryAfter time.Time `json:"retry_after,omitempty"`
}

// ConnectionManager owns the lifecycle of every connection from the bus
// host to a sidecar within the active universe. There is exactly one
// instance per (universe, host) — published as a service in the bus host
// (today: ktw-hub) and consumed by the bus dispatch path.
//
// All methods are safe for concurrent use.
type ConnectionManager interface {
	// Acquire returns a Sender for the named sidecar, lazily spawning
	// the underlying transport on first call. Subsequent calls return
	// the same Sender (idempotent; refcount tracked). Caller MUST call
	// Release(name) when done so the idle clock can advance.
	//
	// Returns ErrSidecarNotInUniverse if name isn't listed in
	// Univers.yaml (no spawn policy declared).
	Acquire(ctx context.Context, name string) (ktsp.Sender, error)

	// Release decrements the refcount for name. Releasing an unknown or
	// unacquired name is a no-op. When the refcount reaches zero AND no
	// live subscriptions exist AND idle_timeout has elapsed (idle-timer
	// todo), the manager initiates IDLE_DRAINING.
	Release(name string)

	// Status returns a snapshot of one connection's state. Returns
	// (zero, false) if name isn't a known member of this universe.
	Status(name string) (ConnStatus, bool)

	// All returns a snapshot of every member's status. Members never
	// acquired are present in DECLARED state with RefCount=0.
	All() []ConnStatus

	// Restart stops a member that runs and starts it again, so it reads its
	// keys and config anew (keys saved while it ran:). A member
	// that does not run is left alone: it answers false, and its next Acquire
	// starts it with what is saved. ErrSidecarNotInUniverse for an unknown
	// name; a remote member, whose process this host does not own, is refused.
	Restart(ctx context.Context, name string) (bool, error)

	// Close terminates every live connection (universe deactivation).
	// Sends SIGTERM with the configured grace period; force-kills on
	// timeout. After Close, every member transitions back to DECLARED.
	Close() error
}

// ErrSidecarNotInUniverse is returned by Acquire when the requested
// name isn't a declared member of this universe (i.e. not in
// Univers.yaml). Distinct from a spawn failure — this is "we don't
// know how to reach this name" rather than "we tried and it failed."
var ErrSidecarNotInUniverse = errors.New("connmgr: sidecar not declared in universe")

// ErrMemberRefused is returned by Acquire for a member the hub refuses to
// start (SpawnSpec.Refused): a binary whose version is not the one its
// universe pins.
var ErrMemberRefused = errors.New("connmgr: member refused")

// ErrFolderMember is returned by Acquire for a FOLDER member: a content root
// the hub reads itself, with no process to start or reach. Distinct from
// ErrSidecarNotInUniverse — the member is declared; it is not a process.
var ErrFolderMember = errors.New("connmgr: a folder member runs no process (the hub reads its folder)")

// SpawnSpec captures everything ConnectionManager needs to spawn a
// sidecar. Mirrors the relevant fields of `appcore.SidecarSpec` (the
// Univers.yaml row) without taking that import — the connmgr API
// stays free of appcore dependencies so it can move into a NATS-only
// world later without churn.
//
// Restart policy knobs (max_restarts / max_restart_window / graceful_shutdown)
// land in the supervisor-wiring follow-up; the idle-disconnect knobs
// (IdleTimeout, Pin, Lifecycle) are honoured by this manager.
type SpawnSpec struct {
	Name      string `json:"name"`
	Transport string `json:"transport"` // "stdio" / "ws" / "http+sse" / "remote"
	Binary    string `json:"binary,omitempty"`

	// Args is the child's WHOLE argv, not extra flags: it must END with the
	// KTSP invocation (`sc ktsp stdio` / `sc ktsp nats`), because that is what
	// tells the binary to speak the protocol rather than run a CLI verb.
	//
	// A content pack states both — `["--folder", <path>, "sc", "ktsp", "nats"]`
	// — since the host needs the folder AND has to be told to speak KTSP.
	// Empty means the plain invocation for the transport.
	//
	// Declaring only flags produces a child that prints its help and exits,
	// which surfaces as `transport: closed` several layers away.
	Args []string          `json:"args,omitempty"`
	Env  map[string]string `json:"env,omitempty"`

	// Folder is a FOLDER member's content root: no binary, nothing to spawn —
	// the hub reads the folder itself (appcore.SidecarSpec.IsFolder).
	Folder string `json:"folder,omitempty"`

	// Refused says why the hub will not start this member (a version skew,
	// ); Acquire answers ErrMemberRefused with it, and nothing
	// is spawned.
	Refused string `json:"refused,omitempty"`

	// Provider is the sidecar's self-declared bus identity — the
	// `<provider>` segment in `<provider>:/<slot>/<method>` URIs.
	//
	// REQUIRED when `Transport == "remote"`: the sidecar is already
	// running on the bus under its own name, which may differ from
	// `Name` (Name is a connmgr-local label, useful when two remote
	// sidecars of the same kind need distinct local handles).
	//
	// IGNORED for `transport: stdio` / `transport: nats`: the parent
	// spawns the binary and dictates the bus name via `Name`.
	Provider string `json:"provider,omitempty"`

	// IdleTimeout is the duration of idleness (refcount==0, no live
	// subscriptions, no busy flag) before the manager initiates
	// IDLE_DRAINING. Zero (the default) disables idle disconnect — the
	// connection stays live until Close() or universe deactivation.
	// Per analysis §3.5, this is the headline knob this milestone adds.
	IdleTimeout time.Duration `json:"idle_timeout,omitempty"`

	// Pin disables idle disconnect entirely, regardless of IdleTimeout.
	// Use for bridge-style sidecars with long-lived upstream connections
	// (per analysis §3.5: "pin: true").
	Pin bool `json:"pin,omitempty"`

	// Lifecycle declares whether the sidecar holds session-scoped
	// state. Stateful sidecars refuse idle disconnect for the lifetime
	// of the universe-level session (today: equivalent to Pin since
	// session tracking isn't wired in the manager yet). Empty == "stateless".
	Lifecycle string `json:"lifecycle,omitempty"` // "stateless" | "stateful"

	// MaxRestarts is the upper bound on automatic spawn retries within
	// `MaxRestartWindow`. Zero (the default) disables auto-retry — a
	// spawn failure parks the connection in FAILED until manual
	// intervention. Used for `transport: stdio` and `transport: nats`;
	// `transport: remote` ignores it (no spawn to retry).
	MaxRestarts int `json:"max_restarts,omitempty"`

	// MaxRestartWindow is the rolling window over which MaxRestarts
	// applies. After this duration without an in-window failure, the
	// restart counter resets so a long-running sidecar that occasionally
	// crashes doesn't accumulate state forever. Default 5 minutes.
	MaxRestartWindow time.Duration `json:"max_restart_window,omitempty"`

	// GracefulShutdown is the time the manager waits for a SIGTERM'd
	// (or stdin-closed) subprocess to exit before sending SIGKILL.
	// Default 5 seconds — matches analysis §3.5.
	GracefulShutdown time.Duration `json:"graceful_shutdown,omitempty"`

	// ReadyTimeout is how long a started `transport: nats` member (or a
	// `transport: remote` one) has to answer its readiness ping before the
	// start counts as failed and the process is killed. Zero keeps
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration `json:"ready_timeout,omitempty"`
}

// DefaultReadyTimeout is the readiness limit of a member that declares none:
// long enough for ktw-agents to read a large project while other agents keep
// it busy - a start past it is killed and restarted, longer.
const DefaultReadyTimeout = 30 * time.Second

// LifecycleStateful is the SpawnSpec.Lifecycle value that opts a sidecar
// out of idle disconnect for the duration of an active session.
const LifecycleStateful = "stateful"

// ConnStateChangedEvent is the payload published on
// `ktw.<tenant>.universe.<u>.event.connstate-changed` by ManagerImpl
// every time a sidecar's lifecycle state changes. Subscribers (GUI
// Inspector, observability dashboards, MCP refresh path, …) listen
// here for FSM transitions — including the "sidecar disconnected"
// case from analysis §3.5 (`From=READY, To=DECLARED, Reason="idle"`).
//
// Emission is best-effort: the manager publishes via Bus.Publish and
// ignores failures (a dead bus shouldn't block lifecycle work).
type ConnStateChangedEvent struct {
	// Name is the sidecar name (matches SpawnSpec.Name).
	Name string `json:"name"`

	// From is the previous state. Empty on the first emit (e.g.
	// the initial DECLARED entry has no "from").
	From ConnState `json:"from,omitempty"`

	// To is the new state.
	To ConnState `json:"to"`

	// Reason is a human-readable explanation. Examples:
	//   - "spawn: ..."        (during SPAWNING → FAILED)
	//   - "ready"             (SPAWNING → READY)
	//   - "idle"              (READY → IDLE_DRAINING)
	//   - "drained"           (IDLE_DRAINING → DECLARED)
	//   - "close"             (manager Close())
	Reason string `json:"reason,omitempty"`

	// At is the wall-clock timestamp of the transition (UTC).
	At time.Time `json:"at"`
}

// Event reasons — exported as constants so callers can switch on them
// without string literal drift.
const (
	ReasonSpawning   = "spawning"
	ReasonReady      = "ready"
	ReasonIdle       = "idle"
	ReasonDrained    = "drained"
	ReasonClose      = "close"
	ReasonSpawnFail  = "spawn-failed"
	ReasonRefAcquire = "acquire"
	// ReasonExited: the member's process ended without being asked to
	// (crashed, killed); it is started again at its next Acquire.
	ReasonExited = "exited"
	// ReasonRestart: the member was stopped to be started again (Restart).
	ReasonRestart = "restart"
)
