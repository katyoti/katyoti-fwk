// SPDX-License-Identifier: Apache-2.0
/**
 * usePanelRefresh — a declared panel re-reads when the objects it shows change.
 *
 * The Go side forwards every CWM object event of an active universe as the
 * `workspace.changed` Wails event, with the object it is about (its id,
 * module, type, parent; a move also where it was). A panel declares which of
 * those concern it (`refresh: {scope, types}` on its object-panel; the object
 * pane defaults it: `module` for a panel with a source, `none` for static
 * content). Only a MOUNTED panel listens; a burst of matching events re-reads
 * it once, 300 ms after the last one (trailing debounce), and the panel keeps
 * its current page on screen while it re-reads. A panel whose read is costly
 * (« À faire ») paces it further: a longer debounce and a longest wait, so a
 * steady stream of changes re-reads it at most once per that wait.
 */

import { useEffect, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { bareOfRef } from '../sections/workspace/objectRef'

/** A panel's declared refresh (object-pane PanelRefresh). */
export interface PanelRefresh {
  scope?: 'self' | 'children' | 'module' | 'universe' | 'none' | string
  types?: string[]
  /** The GUI's own pacing of a costly panel (never declared by a pack): the
   *  quiet time after a burst before it re-reads (PANEL_REFRESH_DEBOUNCE_MS
   *  by default) and the longest a steady stream holds a re-read back. */
  debounceMs?: number
  maxWaitMs?: number
}

/** One CWM object event as the GUI forwards it (orchestrator.WorkspaceEvent). */
export interface WorkspaceChangedEvent {
  universe: string
  subject: string
  object?: string
  module?: string
  type?: string
  parent?: string
  action?: string
  fromModule?: string
  fromParent?: string
  /** A raised event's type name (`source.sync`); its action is `event`. */
  event?: string
  /** A raised event's payload as compact JSON. */
  payload?: string
}

/** Where a panel is shown: its universe, its object (none on the Global
 *  strip) and the module it is about (the object's; a global panel's target
 *  module; empty = every module). */
export interface PanelPlace {
  universe: string
  object?: string
  module?: string
}

/** The trailing debounce of a panel's re-read, in ms. */
export const PANEL_REFRESH_DEBOUNCE_MS = 300

/** moduleOfId is the module an object id is in: an authored id is
 *  module-qualified (`<module>.<local>`), a module's own id is its name. */
export function moduleOfId(id: string | undefined): string {
  if (!id) return ''
  const i = id.indexOf('.')
  return i < 0 ? id : id.slice(0, i)
}

/** panelRefreshMatches says whether an event re-reads a panel: never with no
 *  refresh or scope `none`, never for another universe or (with `types`)
 *  another type; then by scope — `universe` any object; `module` an object
 *  of the panel's module, or that left it (every module when the panel has
 *  none); `self` the panel's object; `children` it or an object directly
 *  under it, or that left it. A panel with no object (the Global strip) reads
 *  `self` and `children` as `module`. What the event does not say (a payload
 *  that could not be read) matches: a re-read too many beats a stale panel. */
export function panelRefreshMatches(refresh: PanelRefresh | undefined | null, ev: WorkspaceChangedEvent, place: PanelPlace): boolean {
  const scope = refresh?.scope || 'none'
  if (scope === 'none') return false
  if (ev.universe !== place.universe) return false
  const types = refresh?.types ?? []
  if (types.length > 0 && ev.type && !types.includes(ev.type)) return false
  // An object event names the object bare (no project): a panel placed on a
  // qualified id `<key>:<module>.<local>` matches by its bare
  // part - a homonym's change in another project re-reads it once too many.
  const object = bareOfRef(place.object ?? '')
  const inModule = () => {
    const m = place.module || moduleOfId(object)
    return !m || !ev.module || ev.module === m || ev.fromModule === m
  }
  switch (scope) {
    case 'universe':
      return true
    case 'module':
      return inModule()
    case 'self':
      if (!object) return inModule()
      return !ev.object || ev.object === object
    case 'children':
      if (!object) return inModule()
      return !ev.object || ev.object === object || ev.parent === object || ev.fromParent === object
    default:
      return false
  }
}

/** refreshPacer calls fire once a burst of events is over - `debounceMs`
 *  after the last one - or, under a steady stream, once `maxWaitMs` passed
 *  since the first event it holds back (none: the debounce alone). */
export function refreshPacer(fire: () => void, debounceMs: number = PANEL_REFRESH_DEBOUNCE_MS, maxWaitMs?: number) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let first = 0
  const go = () => { timer = null; first = 0; fire() }
  return {
    event() {
      const now = Date.now()
      if (!first) first = now
      if (timer) clearTimeout(timer)
      const wait = maxWaitMs && maxWaitMs > 0 ? Math.max(0, Math.min(debounceMs, first + maxWaitMs - now)) : debounceMs
      timer = setTimeout(go, wait)
    },
    stop() { if (timer) clearTimeout(timer); timer = null; first = 0 },
  }
}

/** usePanelRefresh calls onRefresh once a burst of events matching the
 *  panel's refresh is over, while the panel is mounted, paced as it says. */
export function usePanelRefresh(refresh: PanelRefresh | undefined | null, place: PanelPlace, onRefresh: () => void): void {
  const cb = useRef(onRefresh)
  cb.current = onRefresh
  const scope = refresh?.scope || 'none'
  const types = (refresh?.types ?? []).join(',')
  const { debounceMs, maxWaitMs } = refresh ?? {}
  const { universe, object = '', module = '' } = place
  useEffect(() => {
    // The Wails runtime is injected by the WebView — absent in a plain browser.
    if (scope === 'none' || !(window as unknown as { runtime?: unknown }).runtime) return
    const declared: PanelRefresh = { scope, types: types ? types.split(',') : [] }
    const here: PanelPlace = { universe, object, module }
    const pacer = refreshPacer(() => cb.current(), debounceMs ?? PANEL_REFRESH_DEBOUNCE_MS, maxWaitMs)
    const off = EventsOn('workspace.changed', (ev: WorkspaceChangedEvent) => {
      if (panelRefreshMatches(declared, ev, here)) pacer.event()
    })
    return () => {
      off()
      pacer.stop()
    }
  }, [scope, types, universe, object, module, debounceMs, maxWaitMs])
}
