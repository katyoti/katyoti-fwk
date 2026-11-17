// SPDX-License-Identifier: Apache-2.0
// Host services — what a component asks of the surface it is mounted in and
// cannot do itself: search the project's objects, list an object's panels,
// open an object, call a method, load a page section. The GUI provides them
// (HostServicesContext.Provider); a surface that provides none (the chat
// iframe, a test) leaves every consumer on its fallback — an object reference
// is then typed as text, and shown as text; a section with nothing of its
// own shows its empty text.

import type { PanelEvent, PanelInput } from '../../ui-builders/page/panel.ts'
import { createContext, useContext } from 'react'

/** Which objects a reference may name: an ObjectFilter (CWM) — the params
 *  of a `string@object-ref(...)` / `string@panel-ref(...)` field. */
export interface RefFilter {
  types?: string[]
  capabilities?: string[]
  incoming?: string[]
  outgoing?: string[]
  when?: string
}

/** An object a picker offers. */
export interface ObjectChoice {
  id: string
  name?: string
  type?: string
  module?: string
}

/** A panel of an object a panel picker offers: its name (the part after
 *  `#` in a panel reference) and its label. */
export interface PanelChoice {
  id: string
  name?: string
}

/** An object to open, on one of its panels when `panel` is given. */
export interface RefTarget {
  object: string
  panel?: string
}

export interface HostServices {
  /** The objects matching a query and a filter, best first. */
  pickObjects?: (query: string, filter: RefFilter) => Promise<ObjectChoice[]>
  /** The declared panels of an object. */
  objectPanels?: (objectId: string) => Promise<PanelChoice[]>
  /** Open an object, on one of its panels. */
  navigate?: (target: RefTarget) => void
  /** Call a contributed method - its URI `provider:slot/method` - as the
   *  person the surface serves, answering its result: what a form field or a
   *  picker driven by method URIs calls. */
  callMethod?: (uri: string, input?: unknown) => Promise<unknown>
  /** Load a section: run its load method with its args and
   *  answer what it shows - the one answer ({tree, parts,
   *  data} with its parts resolved, or a page {dslTree, data}). The signal
   *  ends when the section no longer needs it (cancels it). */
  loadSection?: (load: { method: string; args?: Record<string, unknown> }, signal?: AbortSignal) => Promise<unknown>
  /** Call onChange once a burst of object changes a section's refresh
   *  matches is over - its scope, types, object or module, as a
   *  panel's refresh; answers the function that stops watching. */
  watchChanges?: (refresh: { scope: string; types?: string[]; object?: string; module?: string }, onChange: () => void) => () => void
  /** The person at the screen, as an actor: what a picker's apply names as
   *  who kept its items - a method call carries no caller. */
  person?: { id: string; kind: 'person' }
  /** Whether pages draw progressively - sections loaded as they come into
   *  view, lists and tables drawing only what is seen, each section refreshed
   *  and failing on its own. The host's display switch
   *  for the increment that brings it; absent or false, pages draw
   *  as before. */
  progressive?: boolean
  /** Whether display panels show - a panel in a tab, inline where its action
   *  ran and in the run drawer, a long action answering with its panel.
   *  The host's display switch for the increment that
   *  brings them; absent or false, today's screens show. */
  displayPanels?: boolean
  /** Whether the source model's views show - the source cards, a source's
   *  and an item's detail, the multi-source search panel, the object pane's
   *  one header, the reader. The host's display switch for the increment
   *  that brings them; absent or false, the former display shows. */
  sourceModel?: boolean
  /** Follow a display panel: call onEvent with each of its
   *  events in order - its snapshot's first, for a screen joining late -;
   *  answers the function that stops following. */
  followPanel?: (id: string, onEvent: (event: PanelEvent) => void) => () => void
  /** Follow the events of a type raised on objects - a
   *  source's `source.sync` -: call onEvent with each one's payload and the
   *  object it was raised on, as they come; answers the function that stops
   *  following. Without it, a progress list's per-source lines stay as drawn. */
  followEvents?: (event: string, onEvent: (payload: unknown, object?: string) => void) => () => void
  /** Open a display panel in a tab of its own, like a document. */
  openPanel?: (id: string) => void
  /** Send the person's input to a display panel: the answer
   *  to its question, a cancel - it goes back to the run owning it. */
  sendPanelInput?: (input: PanelInput) => Promise<void>
  /** The tree a panel's open names by `{uri, hash}` - a page template
   *  -, from the host's spec cache, asked once on a miss;
   *  undefined when it is not known. Without it, a panel with a named tree
   *  shows its title only. */
  panelTemplate?: (spec: { uri: string; hash?: string }) => Promise<Record<string, unknown> | undefined>
}

export const HostServicesContext = createContext<HostServices>({})

export function useHostServices(): HostServices {
  return useContext(HostServicesContext)
}

/** Whether the host has pages draw progressively (HostServices.progressive). */
export function useProgressive(): boolean {
  return useContext(HostServicesContext).progressive === true
}

/** Whether the host shows display panels (HostServices.displayPanels). */
export function useDisplayPanels(): boolean {
  return useContext(HostServicesContext).displayPanels === true
}

/** Whether the host shows the source model's views (HostServices.sourceModel). */
export function useSourceModel(): boolean {
  return useContext(HostServicesContext).sourceModel === true
}

/** refTarget reads a reference value: an object id, or `<id>#<panel>` (a
 *  panel reference). Undefined for an empty value. */
export function refTarget(value: unknown): RefTarget | undefined {
  if (typeof value !== 'string' || value === '') return undefined
  const i = value.indexOf('#')
  if (i < 0) return { object: value }
  return { object: value.slice(0, i), panel: value.slice(i + 1) || undefined }
}

/** refFilterOf reads the filter a reference field declares from its content
 *  params (a list param may arrive as a single name). */
export function refFilterOf(params: Record<string, unknown> | undefined): RefFilter {
  const list = (v: unknown): string[] | undefined =>
    Array.isArray(v) ? v.map(String) : typeof v === 'string' && v !== '' ? v.split('|').map((s) => s.trim()).filter(Boolean) : undefined
  const out: RefFilter = {}
  if (!params) return out
  for (const k of ['types', 'capabilities', 'incoming', 'outgoing'] as const) {
    const l = list(params[k])
    if (l?.length) out[k] = l
  }
  if (typeof params.when === 'string' && params.when !== '') out.when = params.when
  return out
}
