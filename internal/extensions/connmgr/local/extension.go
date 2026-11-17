// SPDX-License-Identifier: Apache-2.0

// Package local exposes public constructors for the ConnectionManager.
//
// The internal/core package is restricted; hub binaries import this
// package to wire the manager. Use NewManager to construct, or
// AcquireRelease to wrap a Sender call with refcount-correct lifecycle.
package local

import (
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	cmcore "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/internal/core"
)

// NewManager constructs a stdio-only ConnectionManager pre-populated
// with one DECLARED entry per spec. Order is preserved. Sidecars with
// `transport: nats` will fail at Acquire — use NewManagerWithBus to
// enable nats-mode spawning.
func NewManager(specs []cmapi.SpawnSpec) (cmapi.ConnectionManager, error) {
	return cmcore.NewManager(specs)
}

// BusScope re-exports the (bus, tenant, universe) triple for hub
// binaries that wire NewManagerWithBus.
type BusScope = cmcore.BusScope

// NewManagerWithBus constructs a ConnectionManager that can spawn
// sidecars in either `transport: stdio` (legacy) or `transport: nats`
// (NATS-bus dispatch). The bus must already be connected; tenant +
// universe identify the scope for SlotSubject construction.
func NewManagerWithBus(specs []cmapi.SpawnSpec, scope BusScope) (cmapi.ConnectionManager, error) {
	return cmcore.NewManagerWithBus(specs, scope)
}

// MemberSwapper exposes the optional admin surface on the impl —
// hub binaries call SetMembers when Univers.yaml changes.
type MemberSwapper interface {
	SetMembers(specs []cmapi.SpawnSpec) error
}
