// SPDX-License-Identifier: Apache-2.0
/**
 * devStore — in-app developer console state.
 *
 * Captures console.* output, uncaught errors, and backend IPC calls
 * (the Wails `window.go.main.App.*` methods — the "network" of a Wails
 * app) into a bounded ring buffer the DevPanel renders. Toggled from the
 * IconRibbon. installDevCapture() (called once at boot) does the patching.
 */

import { create } from 'zustand'

export type DevLevel = 'log' | 'info' | 'warn' | 'error' | 'ipc' | 'event'

export interface DevLogEntry {
  id: number
  level: DevLevel
  // milliseconds since capture install (monotonic-ish; avoids Date.now in
  // hot paths and is enough for ordering + relative timing).
  t: number
  text: string
}

const MAX_LOGS = 1000

interface DevState {
  open: boolean
  toggle: () => void
  setOpen: (v: boolean) => void
  logs: DevLogEntry[]
  push: (level: DevLevel, text: string) => void
  clear: () => void
}

let seq = 0

export const useDevStore = create<DevState>((set) => ({
  open: false,
  toggle: () => set((s) => ({ open: !s.open })),
  setOpen: (v) => set({ open: v }),
  logs: [],
  push: (level, text) =>
    set((s) => {
      const next = s.logs.length >= MAX_LOGS ? s.logs.slice(s.logs.length - MAX_LOGS + 1) : s.logs.slice()
      next.push({ id: ++seq, level, t: Math.round(performance.now()), text })
      return { logs: next }
    }),
  clear: () => set({ logs: [] }),
}))
