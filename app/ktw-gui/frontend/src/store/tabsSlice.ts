// SPDX-License-Identifier: Apache-2.0
/**
 * Tabs slice — drives the right-pane tab strip (`Home | Sessions | Run:* | …`).
 *
 * Shape A of gui-mainpane-restructure (todo/ms06, 2026-05-08).
 *
 * Tabs decouple from `activeNodeId`: switching context nodes only changes
 * what `Home` shows; open Run tabs persist. The two anchor tabs (Home,
 * Sessions) are always present and never close. Run tabs are user-opened;
 * closing one archives its session to the Sessions tab (still browsable
 * via the universe-core's session journal).
 *
 * Per-universe scoping: each universe has its own openTabs list. We key
 * by `universeName` so switching universes doesn't bleed Run tabs across
 * boundaries. The active universe's tabs are exposed as the current
 * `openTabs` accessor.
 */

import { create } from 'zustand'

export type TabKind =
  | 'home'           // Renamed to "Global" in the UI; pure global quick-actions + global tasks.
  | 'object'         // Single Object tab; content swaps on object-click in the OBJECTS tree.
  | 'chat'           // Workspace chat session. Hosts ChatPanel. Single 'chat' tab at a time; subsequent launches focus the existing one. User can navigate Global / Node / Debug freely while the chat stays alive — closing the tab is the only way to end the session from the UI.
  | 'inbox'          // Inbox panel — pending approval-request items the agent wrote via the inbox channel. Single 'inbox' tab; opening it from the ribbon icon OR clicking the Inbox node in the workspace tree focuses the same tab.
  | 'run-task'
  | 'run-shell'
  | 'run-drawio'
  | 'run-skills'
  | 'run-code'
  | 'marketplace'    // Marketplace — everything that can be installed, in five sections (Infra, Sidecars, Config-sets, Seeds, Solutions) under the shared subscriptions. Single 'marketplace' tab; its section lives in MarketplaceTabPayload.
  | 'guide'          // Guide — search, Suggested for you, tours and recipes by capability (modules/guide/). Single 'guide' tab; its GuideTabPayload narrows it to the topics of a `?` button.
  | 'ready'          // Ready to start — the universe's sidecars' status, the programs to install and the keys. Single 'ready' tab; opened at a universe's first start and from the not-ready badge.
  | 'panels'         // The panels screen: the open display panels and the one selected, as the approved mock-up. Single 'panels' tab, opened from the ribbon behind its display switch.
  | 'panel'          // A display panel by its id: one tab per panel, `panel:<id>`; its PanelTabPayload names it. Behind its display switch.
  | 'debug'          // Debug panel — UI component demos, KTSP browser, resources, types, bus trace, workspace state. Single 'debug' tab; its selection lives in DebugTabPayload.

/**
 * Payload carried by the `debug` tab: which debug section is shown and,
 * in the UI Components section, which demo page is selected. Kept on the
 * tab so leaving and returning to it restores the view.
 */
export interface DebugTabPayload {
  section?: string
  test?: string | null
}

/** The five sections of the Marketplace tab. */
export type MarketplaceSection = 'infra' | 'sidecars' | 'configsets' | 'seeds' | 'solutions'

/** Payload carried by the `marketplace` tab: the section shown. */
export interface MarketplaceTabPayload {
  section?: MarketplaceSection
}

/**
 * Payload carried by the Global tab (the renamed Home). Same vertical-
 * strip pattern as the Object tab so the two surfaces feel symmetric.
 * Strip entries are fixed (chat / tasks / universe / settings) plus
 * any global quick-actions; `activeKind` selects which one shows.
 */
export interface GlobalTabPayload {
  activeKind?: string  // "chat" | "tasks" | "universe" | "quick-actions" | ...
}

/**
 * Payload carried by the `chat` tab (cwm-v3 US-3 Phase 1).
 *
 * `taskName` is the canonical TASK.md name being run (default `"chat"`
 * — the system chat task). Future versions may carry the active
 * node/item context so the cwm AgentPlugin's `_target` envelope
 * is auto-derived; for now the plugin reads it independently from the
 * Wails kickoff.
 */
export interface ChatTabPayload {
  taskName: string
  /** Journal session id this chat tab is bound to (parallel chats). Each
   *  run gets its own `chat:<sessionId>` tab; ChatPanel focuses this
   *  session so the global mirror shows the right conversation. Absent on
   *  the legacy single 'chat' tab. */
  sessionId?: string
  /** Name of the CWM node this chat was launched against (`_target`),
   *  captured at launch. Drives the "Scoped to <node>" chip in the chat
   *  header (sdd-ux-p0). Absent for global-scope chats. */
  scopedNodeName?: string
}

export interface OpenTab {
  /** Stable id for this tab. For run tabs, often the session/run id. */
  id: string
  kind: TabKind
  /** Display label in the tab strip. */
  label: string
  /** Optional sub-label / status (e.g. "running", "ended"). */
  status?: 'running' | 'ended' | 'idle'
  /** When closeable (false for Home + Sessions anchors). */
  closeable: boolean
  /** Per-kind payload — e.g. for run-task: { taskName, sessionId }. */
  payload?: unknown
  /** Mount timestamp for sorting / auto-archive. */
  openedAt: number
  /** Last activity timestamp — bumped by chat events for auto-archive. */
  lastActivityAt: number
}

const HOME_TAB: OpenTab = {
  id: 'home',                 // kind stays 'home' (internal id stable for backward-compat).
  kind: 'home',               // 'home' === Global tab post cwm-v3 UX restructure 2026-06-06.
  label: 'Global',            // User-facing label renamed.
  closeable: false,
  openedAt: 0,
  lastActivityAt: 0,
}

interface UniverseTabs {
  openTabs: OpenTab[]
  activeTabId: string
}

interface TabsState {
  /** Per-universe tab state. Empty universe key gets the anchor pair on
   *  first read. */
  byUniverse: Map<string, UniverseTabs>

  /** Read the current universe's tabs + active id. Returns the anchor
   *  pair when the universe has no entry yet. */
  getForUniverse: (universeName: string | null) => UniverseTabs

  /** Open a new tab (or focus an existing one with the same id). */
  openTab: (universeName: string, tab: Omit<OpenTab, 'openedAt' | 'lastActivityAt'>) => void

  /** Close a tab. Anchors (Home, Sessions) are not closeable; the call
   *  is a no-op if the id resolves to one. Switches active to the
   *  previous tab if the closed one was active. */
  closeTab: (universeName: string, tabId: string) => void

  /** Switch the active tab. */
  setActive: (universeName: string, tabId: string) => void

  /** Update a tab's payload + (optionally) label. Used by the
   *  single-Node-tab content-swap pattern: subsequent node clicks
   *  call updateTabPayload('node', { nodeId, ... }) to retarget
   *  without opening a new tab. Also used by the vertical-strip
   *  active-entry persistence (writes activeKind into payload). */
  updateTabPayload: (universeName: string, tabId: string, payload: unknown, label?: string) => void

  /** Bump a tab's lastActivityAt (called from chat / message handlers
   *  for auto-archive). */
  bumpActivity: (universeName: string, tabId: string) => void
}

function defaultUniverseTabs(): UniverseTabs {
  return {
    // Only Home is an anchor tab in the right pane. Historical sessions
    // live in the LEFT pane (ProjectPanel's Sessions tab → LiveSessionList →
    // SessionViewer). The right-pane Sessions anchor was a Stage-1
    // placeholder from gui-mainpane-restructure; it duplicated the same
    // surface with mock data and was removed in favor of the canonical
    // left-pane list.
    openTabs: [HOME_TAB],
    activeTabId: 'home',
  }
}

export const useTabsStore = create<TabsState>((set, get) => ({
  byUniverse: new Map(),

  getForUniverse: (universeName) => {
    if (!universeName) return defaultUniverseTabs()
    const existing = get().byUniverse.get(universeName)
    return existing ?? defaultUniverseTabs()
  },

  openTab: (universeName, tab) =>
    set((s) => {
      const byUniverse = new Map(s.byUniverse)
      const existing = byUniverse.get(universeName) ?? defaultUniverseTabs()
      // Focus existing if same id.
      const idx = existing.openTabs.findIndex((t) => t.id === tab.id)
      if (idx >= 0) {
        byUniverse.set(universeName, { ...existing, activeTabId: tab.id })
        return { byUniverse }
      }
      const now = Date.now()
      const next: OpenTab = { ...tab, openedAt: now, lastActivityAt: now }
      byUniverse.set(universeName, {
        openTabs: [...existing.openTabs, next],
        activeTabId: next.id,
      })
      return { byUniverse }
    }),

  closeTab: (universeName, tabId) =>
    set((s) => {
      const byUniverse = new Map(s.byUniverse)
      const existing = byUniverse.get(universeName) ?? defaultUniverseTabs()
      const target = existing.openTabs.find((t) => t.id === tabId)
      if (!target || !target.closeable) return s
      const filtered = existing.openTabs.filter((t) => t.id !== tabId)
      // If the closed tab was active, fall back to the previous tab in
      // the strip (or Home as a final fallback).
      let nextActive = existing.activeTabId
      if (existing.activeTabId === tabId) {
        const closedIdx = existing.openTabs.findIndex((t) => t.id === tabId)
        const fallback = existing.openTabs[closedIdx - 1] ?? existing.openTabs[0]
        nextActive = fallback?.id ?? 'home'
      }
      byUniverse.set(universeName, { openTabs: filtered, activeTabId: nextActive })
      return { byUniverse }
    }),

  setActive: (universeName, tabId) =>
    set((s) => {
      const byUniverse = new Map(s.byUniverse)
      const existing = byUniverse.get(universeName) ?? defaultUniverseTabs()
      if (!existing.openTabs.some((t) => t.id === tabId)) return s
      byUniverse.set(universeName, { ...existing, activeTabId: tabId })
      return { byUniverse }
    }),

  updateTabPayload: (universeName, tabId, payload, label) =>
    set((s) => {
      const byUniverse = new Map(s.byUniverse)
      const existing = byUniverse.get(universeName) ?? defaultUniverseTabs()
      const idx = existing.openTabs.findIndex((t) => t.id === tabId)
      if (idx < 0) return s
      const updated = [...existing.openTabs]
      updated[idx] = {
        ...updated[idx],
        payload,
        lastActivityAt: Date.now(),
        ...(label !== undefined ? { label } : {}),
      }
      byUniverse.set(universeName, { ...existing, openTabs: updated })
      return { byUniverse }
    }),

  bumpActivity: (universeName, tabId) =>
    set((s) => {
      const byUniverse = new Map(s.byUniverse)
      const existing = byUniverse.get(universeName) ?? defaultUniverseTabs()
      const idx = existing.openTabs.findIndex((t) => t.id === tabId)
      if (idx < 0) return s
      const updated = [...existing.openTabs]
      updated[idx] = { ...updated[idx], lastActivityAt: Date.now() }
      byUniverse.set(universeName, { ...existing, openTabs: updated })
      return { byUniverse }
    }),
}))
