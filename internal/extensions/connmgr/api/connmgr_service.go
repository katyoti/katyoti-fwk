// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ConnMgrService is the bus-exposed control surface for the in-process
// ConnectionManager. ConnectionManager's main method (Acquire) returns
// a Go-only Sender that doesn't serialize across the wire, so this
// thin service exists alongside it to give remote clients a way to
// trigger the SAME spawn lifecycle over NATS.
//
// Primary use case (A-RETRY lazy spawn): when a client's slot call hits
// `ErrNoResponders`, the messagebus sender's retry wrapper calls
// `ktw-hub:/conn-mgr/ensure-spawned({name})`, then retries the original
// request once. The hub spawns the named sidecar (transport=nats) via
// ConnectionManager.Acquire + immediate Release (bump-and-release the
// refcount so idle reaping isn't blocked). The sidecar subscribes to
// its slot subjects; the retry then succeeds.
//
// This service is intentionally minimal — Acquire returns a Sender
// which is not serializable, so EnsureSpawned is the only useful
// bus-side operation we can expose for the spawn lifecycle. Status /
// All / Close stay in-process on the hub.
type ConnMgrService interface {
	EnsureSpawned(ctx context.Context, in EnsureSpawnedInput) (EnsureSpawnedOutput, error)
	// Members is every member's state: which run (READY), which do not.
	Members(ctx context.Context, in MembersInput) (MembersOutput, error)
	// Restart stops a member that runs and starts it again (it reads its keys
	// anew:); one that does not run is left alone.
	Restart(ctx context.Context, in RestartInput) (RestartOutput, error)
}

// The paths a client of another universe's hub calls (the marketplace, from
// the control plane, after keys are saved).
const (
	MembersPath = "ktw-hub:ConnMgrService/Members"
	RestartPath = "ktw-hub:ConnMgrService/Restart"
)

// ConnMgrServiceDef declares the bus surface for hub-side ConnMgrService.
// BusName auto-derives to "conn-mgr-service" (mechanical kebab of Name)
// so the wire form matches what clients construct from the PascalCase
// type name in their URIs (e.g. `ktw-hub:/ConnMgrService/EnsureSpawned`).
var ConnMgrServiceDef = meta.ServiceDef{
	Name:  "ConnMgrService",
	Short: "Hub-side spawn lifecycle control over the bus",
	Long: `Bus-exposed control surface for ktw-hub's ConnectionManager. The
in-process ConnectionManager.Acquire returns a Go Sender (not
serializable); EnsureSpawned is the bus-friendly trigger that gives
remote clients a way to drive the same spawn lifecycle.

Primary consumer: messagebus.Sender's retry wrapper. On the cold path
where a slot call hits ErrNoResponders, the wrapper calls
` + "`ensure-spawned({name: \"<provider>\"})`" + ` and retries the
original request. This is the A-RETRY pattern from bus-hub-lazy-spawn-retry.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Members": meta.MethodLong(
			"Every member of the universe and its state",
			`Each member the hub knows from Univers.yaml, with its connection state
(READY: its process runs).`,
		),
		"Restart": meta.MethodLong(
			"Stop a running member and start it again",
			`A member that runs is stopped and started again, so it reads its keys and
config anew (keys saved while it ran). A member that does not run is left
alone: restarted is false, and it starts with what is saved at its next call.`,
		),
		"EnsureSpawned": meta.MethodLong(
			"Spawn the named sidecar if not already running",
			`Acquires the named sidecar via ConnectionManager (transport=nats),
waits for its ktsp/ping to succeed, then releases the refcount. Idempotent
— a sidecar already running is a no-op (the refcount briefly bumps and
drops). Errors only when the sidecar isn't declared in the Univers.yaml sidecars: block or
when spawn itself fails (binary missing, exit on startup, etc.).`,
		),
	},
}

// EnsureSpawnedInput targets one sidecar by its Univers.yaml name.
type EnsureSpawnedInput struct {
	Name string `json:"name" desc:"Sidecar name as declared in the Univers.yaml sidecars: block (e.g., \"ktw-agents\")" jsonschema:"required: true"`
}

// EnsureSpawnedOutput is intentionally empty — success or RPC error.
// Future fields (e.g., spawn duration, was-already-running flag) can be
// added without breaking compatibility.
type EnsureSpawnedOutput struct{}

// MembersInput asks for every member's state.
type MembersInput struct{}

// MembersOutput is every member's state.
type MembersOutput struct {
	Members []ConnStatus `json:"members"`
}

// RestartInput names the member to restart.
type RestartInput struct {
	Name string `json:"name" desc:"Sidecar name as declared in the Univers.yaml sidecars: block" jsonschema:"required: true"`
}

// RestartOutput says whether the member was restarted: false when it did not
// run (it was left alone).
type RestartOutput struct {
	Restarted bool `json:"restarted"`
}
