// SPDX-License-Identifier: Apache-2.0
// The GUI's host services (display library ui-core/host): what a component
// mounted in the GUI — a reference field's picker, a reference's link — asks
// of it. Objects are searched with CWM's ObjectSearch and the field's
// ObjectFilter, an object's panels are its declared ones (object-pane/Detail),
// opening one is the Object tab (openObjectTab), a contributed method is
// called by its URI (callMethod), and a page section's load is run with the
// person's language and level, its page resolved as a panel's
// (loadSection). Calls go through the one
// generic KTSP binding (dispatcherFor), so a search leaves no invocation
// record.

import { useMemo, type ReactNode } from 'react'

import {
  HostServicesContext, type HostServices, type ObjectChoice, type PanelChoice, type RefFilter,
} from '@ktw/display-lib/ui-core/host/index.tsx'
import { WorkspaceCancelCall, WorkspaceRunActionAs } from '../../../wailsjs/go/main/App'
import { resolvePanelTree } from '../../sections/workspace/resolvePanel'
import { appLanguage, useAppLanguage } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { ct, ctSchema } from '../../i18n/contrib'
import { dispatcherFor } from '../../ktsp'
import { openObjectTab } from '../../sections/workspace/openObjectTab'
import { moduleOfId, panelRefreshMatches, PANEL_REFRESH_DEBOUNCE_MS, type WorkspaceChangedEvent } from '../../hooks/usePanelRefresh'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import { inlineParts, namedTrees, needsResolve, RESOLVE_PART, specCacheOf, templateFetchOf, templateFor, type Localize, type PartAnswer, type PartFetch } from '../../lib/specCache'
import { useDisplayLevel } from '../../hooks/useUniverseGui'
import { usePersonId } from '../../store/profileStore'
import { followPanelWith, sendPanelInput, wailsPanelTransport, type PanelInput } from '../../lib/displayPanels'
import { inputBy } from '../../sections/panels/panelInput'
import { openPanelTab } from '../../sections/panels/openPanelTab'
import type { PanelEvent } from '@ktw/display-lib/ui-builders/page/panel.ts'
import type { DisplayLevel } from '../../types/settings'
import { unlocate } from '../../sections/workspace/projectMark'

/** How many objects a picker shows for one search. */
const PICK_LIMIT = 50

/** How many section loads one host runs at once: the rest wait
 *  their turn, in order, so a long page never floods the hub. */
export const SECTION_LOADS_AT_ONCE = 4

let callSeq = 0
/** nextCallId names one call the GUI may give up (WorkspaceCancelCall). */
export function nextCallId(): string {
  callSeq += 1
  return `sec-${Date.now().toString(36)}-${callSeq}`
}

/** How long a read may go unanswered, counted from its turn
 *  at the gate: a read queued behind slow but answering reads
 *  is not failed for its wait. Past it the read ends with a readable error and
 *  « Réessayer », never placeholder rows for minutes while a hub fails it -
 *  the call is given up on the server. A long method answers at once with
 *  its run, so no read the GUI waits on needs longer. */
export const READ_DEADLINE_MS = 90_000

/** readTimedOut is the failure of a read past its deadline, said in the GUI
 *  language (a page section shows its message as it is) and by key. */
export function readTimedOut(ms: number): Error {
  const seconds = Math.round(ms / 1000)
  const msg = { key: 'load.timedOut', params: { seconds } }
  const english = `The read did not answer within ${seconds} s - the universe may be busy or restarting.`
  // Before the locales load, i18n answers the bare key: the English text then.
  const said = errorText({ error: english, errorMsg: msg })
  return Object.assign(new Error(said && said !== msg.key ? said : english), { name: 'ReadTimeout', msg })
}

/** isReadTimeout tells a read failed by its deadline: asked again at once,
 *  it would wait as long again. */
export function isReadTimeout(e: unknown): boolean {
  return (e as { name?: string } | null)?.name === 'ReadTimeout'
}

/** untilAborted answers what `p` answers, or an AbortError once `signal`
 *  ends: a load given up stops waiting on a step that may never answer. */
export function untilAborted<T>(p: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(new DOMException('The section no longer needs its load', 'AbortError'))
  return new Promise<T>((resolve, reject) => {
    const quit = () => reject(new DOMException('The section no longer needs its load', 'AbortError'))
    signal.addEventListener('abort', quit, { once: true })
    p.then(
      (v) => { signal.removeEventListener('abort', quit); resolve(v) },
      (e) => { signal.removeEventListener('abort', quit); reject(e) },
    )
  })
}

/** sharedLoads runs one load per key at a time: a section asking
 *  for a load already in flight shares it rather than starting it twice. The
 *  shared load's signal ends only once every section waiting on it gave up;
 *  a section that gives up alone gets its own AbortError at once. A load
 *  runs at its turn of `gate` when one is given; unanswered `deadlineMs`
 *  after its turn came, it fails every section waiting on it
 *  with readTimedOut, its call given up - and its turn handed
 *  on at once, whatever step it still waits on. */
export function sharedLoads(deadlineMs: number = READ_DEADLINE_MS, gate?: ReturnType<typeof sectionGate>) {
  const inFlight = new Map<string, { promise: Promise<unknown>; waiting: number; abort: AbortController }>()
  return {
    load<T>(key: string, signal: AbortSignal | undefined, start: (shared: AbortSignal) => Promise<T>): Promise<T> {
      if (signal?.aborted) return Promise.reject(new DOMException('The section no longer needs its load', 'AbortError'))
      let entry = inFlight.get(key)
      if (!entry) {
        const abort = new AbortController()
        let timer: ReturnType<typeof setTimeout> | undefined
        let arm = () => {}
        const late = new Promise<never>((_, fail) => {
          arm = () => { timer = setTimeout(() => { fail(readTimedOut(deadlineMs)); abort.abort() }, deadlineMs) }
        })
        // The clock starts with the turn; a load given up leaves its turn.
        const run = () => { arm(); return untilAborted(start(abort.signal), abort.signal) }
        const promise = Promise.race([gate ? gate(run, abort.signal) : run(), late])
          .finally(() => { clearTimeout(timer); if (inFlight.get(key) === entry) inFlight.delete(key) })
        entry = { promise, waiting: 0, abort }
        inFlight.set(key, entry)
      }
      const e = entry
      e.waiting += 1
      return new Promise<T>((resolve, reject) => {
        let settled = false
        const leave = () => {
          if (settled) return
          settled = true
          e.waiting -= 1
          if (e.waiting === 0) { e.abort.abort(); if (inFlight.get(key) === e) inFlight.delete(key) }
          reject(new DOMException('The section no longer needs its load', 'AbortError'))
        }
        signal?.addEventListener('abort', leave, { once: true })
        ;(e.promise as Promise<T>).then(
          (v) => { if (!settled) { settled = true; e.waiting -= 1; signal?.removeEventListener('abort', leave); resolve(v) } },
          (err) => { if (!settled) { settled = true; e.waiting -= 1; signal?.removeEventListener('abort', leave); reject(err) } },
        )
      })
    },
    /** How many loads are in flight (tests). */
    size: () => inFlight.size,
  }
}

/** sectionGate bounds how many loads run at once: run(job) waits for a free
 *  turn - a job whose signal ends while it waits is never started. */
export function sectionGate(limit: number) {
  let running = 0
  const waiting: Array<() => void> = []
  // An ending job hands its turn to the first waiting one, else frees it.
  const next = () => { const go = waiting.shift(); if (go) go(); else running-- }
  return async function run<T>(job: () => Promise<T>, signal?: AbortSignal): Promise<T> {
    if (running >= limit) await new Promise<void>((go) => waiting.push(go))
    else running++
    try {
      if (signal?.aborted) throw new DOMException('The section no longer needs its load', 'AbortError')
      return await job()
    } finally {
      next()
    }
  }
}

/** The universe's one load: every page section (loadSection)
 *  and every view part (loadPart, useHostLoad) of a universe goes through one
 *  gate and one set of shared loads - however many hosts and views are open,
 *  at most SECTION_LOADS_AT_ONCE run at once, and a load asked twice at once
 *  runs once. */
const universeLoads = new Map<string, { shared: ReturnType<typeof sharedLoads> }>()
export function hostLoads(universe: string) {
  let loads = universeLoads.get(universe)
  if (!loads) {
    loads = { shared: sharedLoads(READ_DEADLINE_MS, sectionGate(SECTION_LOADS_AT_ONCE)) }
    universeLoads.set(universe, loads)
  }
  return loads
}

/** loadPart runs one view part's load through the universe's one load: a
 *  same key in flight is shared, its turn waits on the gate, and a signal
 *  ended before the turn never starts it. */
export function loadPart<T>(universe: string, key: string, start: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal): Promise<T> {
  return hostLoads(universe).shared.load(`part:${key}`, signal, start)
}

/** runMethod runs a method as the person under a call id: an
 *  ended signal cancels it on the server. It answers the method's value; a
 *  method's error is thrown with its keyed message (errorText reads it). */
export async function runMethod(universe: string, uri: string, input: string, signal?: AbortSignal): Promise<unknown> {
  const callId = nextCallId()
  const giveUp = () => { void WorkspaceCancelCall(callId) }
  signal?.addEventListener('abort', giveUp, { once: true })
  const r = await WorkspaceRunActionAs(universe, uri, input, callId)
    .finally(() => signal?.removeEventListener('abort', giveUp))
  if (signal?.aborted) throw new DOMException('The section no longer needs its load', 'AbortError')
  if (r.error) throw Object.assign(new Error(r.error), { msg: (r as { errorMsg?: unknown }).errorMsg ?? undefined })
  return JSON.parse(r.value || 'null')
}

export function guiHostServices(universeName: string, level: DisplayLevel = 'expert', language?: string, personId?: string, sourceModel = false): HostServices {
  const call = dispatcherFor(universeName)
  // Below Expert a picker offers nothing of a system module:
  // the modules carrying the system-module capability, asked once.
  let system: Promise<Set<string>> | null = null
  const systemModules = () => (system ??= (call('ktw-agents:cwm/object-list', { capability: 'system-module' }) as Promise<{ objects?: Array<{ id: string }> } | undefined>)
    .then((out) => new Set((out?.objects ?? []).map((o) => o.id)), () => new Set<string>()))
  const { shared: sectionLoads } = hostLoads(universeName)
  const fetchTemplate = templateFetchOf(universeName)
  // A part inlined reads in the person's language: its data type's texts
  // (field labels, menus, actions, groups, axis label) from the contribution
  // translations, each falling back to its own (ctSchema).
  const localize: Localize = (uri, part) => ctSchema((u, path, fb) => ct(u, path, fb, language ?? appLanguage()), uri, part)
  return {
    // The person at the screen, as an actor: who a picker's apply names.
    ...(personId ? { person: { id: personId, kind: 'person' as const } } : {}),
    // Progressive pages: sections loaded as they come into view,
    // lists and tables drawing only what is seen, each section on its own.
    progressive: true,
    // Display panels: a panel node follows its panel - the panel server's snapshot, then its live
    // events - opens it in a tab of its own, and sends the
    // person's answer or cancel back to the run owning it, as the person at
    // the screen; a tree its open names is drawn from the spec
    // cache.
    displayPanels: true,
    followPanel: (id: string, onEvent: (e: PanelEvent) => void) => followPanelWith(wailsPanelTransport(universeName), id, onEvent),
    openPanel: (id: string) => openPanelTab(universeName, id),
    sendPanelInput: (input: PanelInput) => sendPanelInput(universeName, inputBy(input, personId)),
    // A panel's tree named by {uri, hash}: from the
    // universe's spec cache, the panel server's Template asked on a miss.
    panelTemplate: (spec: { uri: string; hash?: string }) => templateFor(specCacheOf(universeName), spec, fetchTemplate),
    // The source model's views, on for every user the GUI serves.
    ...(sourceModel ? { sourceModel: true } : {}),
    pickObjects: async (query: string, filter: RefFilter): Promise<ObjectChoice[]> => {
      const hasFilter = Object.keys(filter).length > 0
      const out = await call('ktw-agents:cwm/object-search', { query, limit: PICK_LIMIT, ...(hasFilter ? { filter } : {}) }) as
        { objects?: Array<{ id: string; name?: string; type?: string }> } | undefined
      const sys = level === 'expert' ? new Set<string>() : await systemModules()
      // An object of the answer, bare (one store) or located in its project
      // (the store set).
      return (out?.objects ?? []).map((o) => unlocate(o))
        .map((o) => ({ ...o, system: sys.has(o.id) || sys.has(moduleOfId(o.id)) }))
        .filter((o) => !o.system)
        .map((o) => ({ id: o.id, name: o.name, type: o.type }))
    },
    objectPanels: async (objectId: string): Promise<PanelChoice[]> => {
      const out = await call('ktw-agents:object-pane/detail', { id: objectId }) as
        { panels?: Array<{ id: string; name?: string }> } | undefined
      // A declared panel's tab id is `tab:<stem>`; a panel reference names its
      // stem (a children group, `tab:children/<type>`, is no declared panel).
      return (out?.panels ?? [])
        .filter((p) => p.id.startsWith('tab:') && !p.id.startsWith('tab:children/'))
        .map((p) => ({ id: p.id.slice('tab:'.length), name: p.name }))
    },
    navigate: (t) => openObjectTab(universeName, { id: t.object, ...(t.panel ? { panel: t.panel } : {}) }),
    // A contributed method, called as the person through the one KTSP
    // binding: the display library's fields and pickers stay
    // generic - they hold method URIs, the GUI only carries the call.
    callMethod: (uri: string, input?: unknown) => call(uri, input ?? {}),
    // A section's load: its method run with the
    // person's language and level, a few at a time; the page or the one answer it
    // gives resolved as a declared panel's (schema-bound nodes inlined); a
    // tree named by {uri, hash} drawn from the spec cache.
    // The signal ends a load the section no longer needs before it starts;
    // stopping one already running on the server is runMethod's.
    // The key names the person's language and level: the universe's shared
    // loads serve every host of it.
    loadSection: (load: { method: string; args?: Record<string, unknown> }, signal?: AbortSignal) => sectionLoads.load(
      JSON.stringify([load.method, load.args ?? {}, language ?? '', level]), signal, async (shared) => {
      // Run under a call id: the load given up by every section waiting on
      // it is cancelled on the server.
      const got = await runMethod(universeName, load.method, JSON.stringify({ ...(load.args ?? {}), ...(language ? { language } : {}), level }), shared) as Record<string, unknown> | null
      if (!got || typeof got !== 'object') return got
      // A tree the answer names by {uri, hash} - a page template - comes from
      // the same cache, Template asked once on a miss.
      // Each step after the call takes the load's signal: a load
      // given up starts no further call.
      const answer = await untilAborted(namedTrees(got, specCacheOf(universeName), fetchTemplate), shared)
      // Parts the answer names by {uri, hash} come from the universe's spec
      // cache, asked once on a miss; the object pane is asked only
      // for what the tree still needs resolved.
      const parts = answer.parts && typeof answer.parts === 'object' ? answer.parts as Record<string, unknown> : undefined
      const fetchPart: PartFetch = async (input) => (await untilAborted(call(RESOLVE_PART, input), shared)) as PartAnswer
      for (const key of ['dslTree', 'tree'] as const) {
        const tree = answer[key]
        if (tree && typeof tree === 'object' && !('uri' in (tree as object))) {
          const inlined = await untilAborted(inlineParts(tree, parts, specCacheOf(universeName), fetchPart, localize), shared)
          const { parts: _held, ...rest } = answer
          return { ...rest, [key]: parts && !needsResolve(inlined) ? inlined : await untilAborted(resolvePanelTree(universeName, inlined, localize), shared) }
        }
      }
      return 'kind' in answer ? await untilAborted(resolvePanelTree(universeName, answer, localize), shared) : answer
    }),
    // A section's own refresh: the object changes the GUI is told
    // of (workspace.changed), matched as a panel's refresh is - against the
    // object or module the section names, else the universe -, a burst
    // calling onChange once, 300 ms after its last event.
    watchChanges: (refresh, onChange) => {
      if (!(window as unknown as { runtime?: unknown }).runtime) return () => {}
      const place = { universe: universeName, object: refresh.object, module: refresh.module }
      let timer: ReturnType<typeof setTimeout> | null = null
      const off = EventsOn('workspace.changed', (ev: WorkspaceChangedEvent) => {
        if (!panelRefreshMatches(refresh, ev, place)) return
        if (timer) clearTimeout(timer)
        timer = setTimeout(() => { timer = null; onChange() }, PANEL_REFRESH_DEBOUNCE_MS)
      })
      return () => { off(); if (timer) clearTimeout(timer) }
    },
    // The events of a type raised on objects - a source's
    // source.sync -, with the source model: each raised event the GUI is told
    // of (workspace.changed, action event) of that type in this universe, its
    // payload read as JSON and passed on with the object it was raised on. A
    // payload that is not whole JSON is skipped.
    ...(sourceModel ? {
      followEvents: (event: string, onEvent: (payload: unknown, object?: string) => void) => {
        if (!(window as unknown as { runtime?: unknown }).runtime) return () => {}
        return EventsOn('workspace.changed', (ev: WorkspaceChangedEvent) => {
          if (ev.universe !== universeName || ev.action !== 'event' || ev.event !== event || !ev.payload) return
          let payload: unknown
          try { payload = JSON.parse(ev.payload) } catch { return }
          onEvent(payload, ev.object)
        })
      },
    } : {}),
  }
}

/** GuiHostServices provides the GUI's host services to what it wraps. */
export function GuiHostServices({ universeName, children }: { universeName: string; children: ReactNode }) {
  const level = useDisplayLevel()
  const language = useAppLanguage()
  const personId = usePersonId()
  // The source model's views show to every user (its switch retired).
  const services = useMemo(() => guiHostServices(universeName, level, language, personId, true), [universeName, level, language, personId])
  return <HostServicesContext.Provider value={services}>{children}</HostServicesContext.Provider>
}
