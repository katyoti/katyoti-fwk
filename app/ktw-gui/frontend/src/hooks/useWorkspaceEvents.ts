// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store'
import type { WorkspaceChangedEvent } from './usePanelRefresh'

/**
 * Live workspace refresh: listens for `workspace.changed` (emitted by the
 * Go side for every CWM node/item/edge mutation arriving on the bus —
 * including writes from OTHER processes: CLI, agent runs, a second GUI
 * window) and bumps `workspaceTreeRevision` so the tree + detail panels
 * re-fetch.
 *
 * Debounced trailing-edge: a cascade (e.g. delete with descendants, agent
 * batch-creating nodes) coalesces into one re-fetch after the burst.
 * GUI-initiated writes also arrive here (the substrate publishes
 * regardless of the writer) — the extra bump after the deterministic
 * post-write refresh is an idempotent no-op fetch.
 *
 * Must be called once in the root App component.
 */
export function useWorkspaceEvents() {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    // Wails runtime is injected by the WebView — not available in plain browser.
    if (!(window as any).runtime) return

    const off = EventsOn('workspace.changed', (_evt: WorkspaceChangedEvent) => {
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(() => {
        timer.current = null
        useAppStore.getState().bumpWorkspaceTree()
      }, 150)
    })

    return () => {
      off()
      if (timer.current) clearTimeout(timer.current)
    }
  }, [])
}
