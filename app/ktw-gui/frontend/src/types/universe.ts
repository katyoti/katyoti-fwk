// SPDX-License-Identifier: Apache-2.0
// Universe model — an ALIAS of the generated client's shape, not a hand-copy.
//
// It used to be declared here by hand, and it had drifted: it promised
// `protected: boolean` while the service marks that field omitempty, so the wire
// can omit it entirely. Every consumer was therefore trusting a guarantee the
// service never made. Aliasing the generated type makes that class of mistake
// impossible — the shape now comes FROM the descriptor the wire is built from.
//
// Universes are the user's first-class context: picking a universe in the
// IconRibbon switches the entire downstream agent/sidecar/config tree to
// that universe's KTW_HOME. See:
//   modules/katyoti/layout.md

// `busRunning` is joined in from this GUI process (see ktsp/universes.ts) — it is
// not a service fact, so it cannot come from the descriptor.
import type { GetUniverseOutput } from '../ktsp/orchestrator'
import type { UniverseWithStatus } from '../ktsp/universes'

export type Universe = UniverseWithStatus

// Sidecar membership for a single universe (from <home>/extensions.yaml).
export interface UniverseSidecarSpec {
  name:       string
  transport:  string
  binary?:    string
  configDir?: string
}

// UniverseInfo is the full payload of the orchestrator's get-universe.
//
// Its `summary` is deliberately NOT the busRunning-enriched `Universe`: that flag
// is this process's own state, and get-universe does not report it. Aliasing the
// generated output keeps the distinction honest instead of promising a field the
// call never returns.
export type UniverseInfo = GetUniverseOutput

// SidecarBinary is one entry from App.ScanSidecarInventory — installed
// ktw-* binaries available on the system. Used by the universe creation
// wizard to populate the sidecar selection list.
export interface SidecarBinary {
  name:        string
  binaryPath:  string
  version?:    string
  source:      string  // "PATH" | "configured"
  lastSeen?:   string  // RFC3339 timestamp from sidecars.yaml cache
}

// ── Main pane router ───────────────────────────────────────────────────────
//
// The main pane (right of the IconRibbon, right of the optional
// ProjectPanel) renders one of these views. Welcome shows when no universe
// is displayed; universe shows the chat/agent UI; edit/delete are
// invoked by right-clicking a universe icon in the ribbon.

export type MainPaneRoute =
  | { kind: 'welcome' }
  | { kind: 'universe' }
  | { kind: 'edit',     name: string }
  | { kind: 'delete',   name: string }
  | { kind: 'terminal', id: string }
  | { kind: 'session',  universeName: string; sessionId: string }

// localStorage key — per-window last-displayed universe (per-window because
// each Wails webview has its own localStorage; lets two ktw-gui windows
// remember different universes).
export const LS_LAST_DISPLAYED = 'ktw.lastDisplayed'
