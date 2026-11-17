// SPDX-License-Identifier: Apache-2.0
/**
 * Terminal sessions slice — running terminals listed in the sidebar's
 * TERM tab. Two source kinds in v1:
 *
 *   - `log` — Go-side log-tail (StartLogTerminal). Output streams into
 *     `scrollback` via Wails events; the right pane shows it as a
 *     monospace auto-scrolling viewer.
 *   - `shell` — external-window launch via the existing LaunchTerminal
 *     binding. The running entry exists for tracking only — the actual
 *     interactive session lives outside ktw-gui.
 *
 * Wire format:
 *   - StartLogTerminal(source) → returns id; backend emits
 *     "terminal.<id>.append" events with {id, chunk} payload, and
 *     "terminal.<id>.end" when the tailer stops.
 *   - StopLogTerminal(id) → backend cancels the tailer.
 *
 * Companion to gui-terminal-pane (todo to be filed) — see also
 * app/ktw-gui/app_terminal_session.go.
 */

import { create } from 'zustand'
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import {
  StartLogTerminal,
  StopLogTerminal,
  StartShellPTY,
  StopShellPTY,
} from '../../wailsjs/go/main/App'

export type TerminalSourceKind = 'log' | 'shell'

export type LogSource = 'ktw-gui-log'

export interface TerminalSession {
  id: string
  kind: TerminalSourceKind
  /** Human-friendly title shown in the TERM list and the main pane. */
  label: string
  /** Source identifier — log source name OR shell id. */
  source: string
  /** Live output captured for `log` kind. Empty for `shell`. */
  scrollback: string
  /** Lifecycle status. `external` is a terminal terminal kind = shell
   *  whose interactive session lives outside ktw-gui (we just track it). */
  status: 'running' | 'ended' | 'external'
  /** Mount timestamp (ms epoch) for sorting. */
  startedAt: number
}

interface TerminalSessionsState {
  sessions: Map<string, TerminalSession>
  activeId: string | null

  /** Spawn a log-tail terminal. Returns the new session id. */
  startLog: (source: LogSource, label?: string) => Promise<string | null>
  /** Spawn an embedded interactive shell (PTY-backed). Returns the new
   *  session id. The shell renders via XtermView which subscribes to
   *  `shell.<id>.data` events for output and forwards keystrokes via
   *  WriteShellPTY. */
  startShell: (shellId: string, label: string, cwd: string) => Promise<string | null>
  /** Close a session — stops the tailer / kills the shell, removes from list. */
  close: (id: string) => void
  /** Mark a session as ended (called by XtermView when the shell exits). */
  setEnded: (id: string) => void
  /** Switch the active session shown in the main pane. */
  setActive: (id: string | null) => void
  /** Clear scrollback for a given session — useful in long-running tails. */
  clearScrollback: (id: string) => void
}

export const useTerminalSessionsStore = create<TerminalSessionsState>((set, get) => ({
  sessions: new Map(),
  activeId: null,

  startLog: async (source, label) => {
    try {
      const id = await StartLogTerminal(source)
      if (!id) return null
      const session: TerminalSession = {
        id,
        kind: 'log',
        label: label ?? defaultLogLabel(source),
        source,
        scrollback: '',
        status: 'running',
        startedAt: Date.now(),
      }

      // Subscribe to append + end events for this session.
      EventsOn(`terminal.${id}.append`, (evt: { chunk?: string } | undefined) => {
        const chunk = evt?.chunk
        if (!chunk) return
        set((s) => {
          const sessions = new Map(s.sessions)
          const existing = sessions.get(id)
          if (!existing) return s
          // Cap scrollback at ~1 MB so a runaway log doesn't OOM the GUI.
          const next = (existing.scrollback + chunk).slice(-1024 * 1024)
          sessions.set(id, { ...existing, scrollback: next })
          return { sessions }
        })
      })
      EventsOn(`terminal.${id}.end`, () => {
        set((s) => {
          const sessions = new Map(s.sessions)
          const existing = sessions.get(id)
          if (!existing) return s
          sessions.set(id, { ...existing, status: 'ended' })
          return { sessions }
        })
      })

      set((s) => {
        const sessions = new Map(s.sessions)
        sessions.set(id, session)
        return { sessions, activeId: s.activeId ?? id }
      })
      return id
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[terminal] startLog failed:', err)
      return null
    }
  },

  startShell: async (shellId, label, cwd) => {
    try {
      const id = await StartShellPTY(shellId, cwd)
      if (!id) return null
      const session: TerminalSession = {
        id,
        kind: 'shell',
        label,
        source: shellId,
        scrollback: '',
        status: 'running',
        startedAt: Date.now(),
      }
      // The XtermView component subscribes to shell.<id>.data + .exit
      // when it mounts; the slice stays out of that loop to avoid
      // duplicate subscriptions.
      set((s) => {
        const sessions = new Map(s.sessions)
        sessions.set(id, session)
        return { sessions, activeId: s.activeId ?? id }
      })
      return id
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[terminal] startShell failed:', err)
      return null
    }
  },

  close: (id) => {
    const session = get().sessions.get(id)
    if (!session) return
    if (session.kind === 'log') {
      void StopLogTerminal(id).catch(() => {})
      EventsOff(`terminal.${id}.append`)
      EventsOff(`terminal.${id}.end`)
    } else if (session.kind === 'shell') {
      void StopShellPTY(id).catch(() => {})
      // XtermView's effect cleanup handles unsubscribing the events.
    }
    set((s) => {
      const sessions = new Map(s.sessions)
      sessions.delete(id)
      const nextActive =
        s.activeId === id ? (sessions.size > 0 ? sessions.keys().next().value ?? null : null) : s.activeId
      return { sessions, activeId: nextActive }
    })
  },

  setEnded: (id) =>
    set((s) => {
      const sessions = new Map(s.sessions)
      const existing = sessions.get(id)
      if (!existing) return s
      sessions.set(id, { ...existing, status: 'ended' })
      return { sessions }
    }),

  setActive: (id) => set({ activeId: id }),

  clearScrollback: (id) =>
    set((s) => {
      const sessions = new Map(s.sessions)
      const existing = sessions.get(id)
      if (!existing) return s
      sessions.set(id, { ...existing, scrollback: '' })
      return { sessions }
    }),
}))

function defaultLogLabel(source: LogSource): string {
  switch (source) {
    case 'ktw-gui-log': return 'ktw-gui logs'
    default:            return source
  }
}
