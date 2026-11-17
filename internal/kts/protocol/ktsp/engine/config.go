// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// SMMode determines how the Engine connects to a Session Manager.
type SMMode string

const (
	// SMEmbedded creates an in-process SM (default).
	SMEmbedded SMMode = "embedded"
	// SMRemote connects to an external SM via WebSocket.
	SMRemote SMMode = "remote"
	// SMStandalone is for an SM-only process (no local Engine container).
	SMStandalone SMMode = "standalone"
	// SMProvided uses a pre-built SM transport (for topology-driven launcher).
	SMProvided SMMode = "provided"
)

// EngineConfig is the flat, Cobra-free configuration for an Engine.
// CLI commands build this from flags; GUI builds it programmatically.
//
// Note: LLM config (provider, capability, model, maxTurns) belongs to the
// agent, not the Engine. Agent slots resolve their own provider at runtime.
type EngineConfig struct {
	// Identity
	Name string // container name (e.g., "ktw-agents", "ktw-gui")

	// Workspace
	WorkspaceRoot string
	SessionsDir   string
	HomeDir       string

	// Extensions
	EmbeddedFS fs.FS

	// SM connection
	SMMode      SMMode         // embedded (default), remote, standalone, provided
	SMURL       string         // for remote mode: "ws://<host>:<port>"
	SMPort      int            // for embedded mode: WS listener port (0 = no listener)
	SMTransport ktsp.Transport // for provided mode: pre-built transport to SM
	SessionID   string         // for provided mode: pre-created session ID

	// Observability
	Trace bool
}

// SlotRef is a pointer to a discovered slot (returned by FindBySlotType).
type SlotRef struct {
	Path     string // "container_id/slot_name"
	SlotType ktsp.SlotType
	Methods  []string
}
