// SPDX-License-Identifier: Apache-2.0
// The page's context, shared by the dispatcher and the node renderers: the
// working copy of the data (page-data.ts), the submit and action handlers of
// the host, the page's selection (shared by the sections of the page), and —
// inside a list, a tree, a card — the element a template is rendered for.

import { createContext, useContext } from 'react'

import type { PageData } from './page-data.ts'
import type { UIPageNode } from './ui-page.types.ts'
import { actionVerb, resolveActionArgs, type UIPageAction, type UIPageActionEvent } from './ui-page.interactive.ts'

export interface PageContext extends Partial<PageData> {
  onSubmit?: (value: unknown, meta: { path?: string; nodeId?: string }) => void
  /** The host runs a declared action; absent when the page has no host (a
   *  chat artifact) — its actions then show disabled. */
  onAction?: (event: UIPageActionEvent) => void
  /** A host runs the page's actions; without one only its `tab` and
   *  `select` actions run (the page's own). */
  hasHost?: boolean
  /** The page around this one: a section's page sees the page holding the
   *  section; absent on the outermost page. */
  enclosing?: PageContext
}

/** enclosingPage is the page context a page drawn inside `around` keeps as
 *  its enclosing one: `around` when it is a page's (it holds a working copy),
 *  else none. */
export function enclosingPage(around: PageContext): PageContext | undefined {
  return 'draft' in around ? around : undefined
}

/** enclosingDrafts are the working copies of the pages around a page,
 *  nearest first. */
export function enclosingDrafts(page: PageContext): unknown[] {
  const out: unknown[] = []
  for (let p = page.enclosing; p; p = p.enclosing) out.push(p.draft)
  return out
}

export const DataContext = createContext<PageContext>({})

/** A page's action as its page runs it: a page-own `select` carries the
 *  working copies of its page and of the pages around it, nearest first
 *  - its value reads in them a name the event lacks; they
 *  never reach the host. */
export interface PageActionEvent extends UIPageActionEvent {
  drafts?: unknown[]
}

/** The tab a `tab` action asked for: a tabs node holding that panel shows it.
 *  `n` counts the asks, so asking the same tab again shows it again; `focus`
 *  is the element of it the page then brings into view. */
export interface TabRequest { id: string; n: number; focus?: string }
export const TabRequestContext = createContext<TabRequest | undefined>(undefined)

/** The page's selection: the keys a `select` action set (`release`,
 *  `increment`), shared by the page and the pages of its sections. `revision`
 *  counts the re-reads of the outermost page's data: a section re-reads with it. */
export interface PageSelection {
  selection: Record<string, unknown>
  select: (key: string, value: unknown) => void
  revision: number
}

/** Provided by the outermost page; a section's page reuses it. */
export const SelectionContext = createContext<PageSelection | undefined>(undefined)

const NO_SELECTION: PageSelection = { selection: {}, select: () => {}, revision: 0 }

export function useSelection(): PageSelection {
  return useContext(SelectionContext) ?? NO_SELECTION
}

/** The loads of the section a page is drawn in: it counts the answers the
 *  section read, so a node of its page tells a section read again from a
 *  redraw, even when the answer is the same; 0 outside a
 *  section. Provided by the nearest section around. */
export const SectionLoadContext = createContext<number>(0)

export function useSectionLoad(): number {
  return useContext(SectionLoadContext)
}

/** initialSelection is a page's selection before any `select`: its data's
 *  `selection` when that is an object. */
export function initialSelection(data: unknown): Record<string, unknown> {
  const sel = data && typeof data === 'object' && !Array.isArray(data) ? (data as Record<string, unknown>).selection : undefined
  return sel && typeof sel === 'object' && !Array.isArray(sel) ? { ...(sel as Record<string, unknown>) } : {}
}

/** routePageAction runs an action where it belongs: a `tab` action shows the
 *  tab, a `select` action sets its key of the selection (its value resolved
 *  against the element: `${item.id}`, the event's extras and the selection,
 *  then the working copies it carries: `${value.name}` from the form of the
 *  page holding a section) — both on the page; any other goes to the host
 *  with the selection in its extras, so `${selection.<key>}` fills its
 *  templates. */
export function routePageAction(e: PageActionEvent, on: {
  show: (tab: string, focus?: string) => void
  select: (key: string, value: unknown) => void
  host: (e: UIPageActionEvent) => void
  selection: Record<string, unknown>
  /** A drawer holding a node is the page's own; one holding
   *  a panel goes to the host. */
  drawer?: (d: { title?: string; node: UIPageNode; item?: unknown }) => void
  /** A modal is the page's own. */
  modal?: (m: { title?: string; node: UIPageNode; size?: string; item?: unknown }) => void
}): void {
  const { tab, select, drawer, modal } = e.action
  if (tab) { on.show(tab, e.action.focus); return }
  if (drawer?.node && on.drawer) { on.drawer({ title: drawer.title, node: drawer.node, item: e.item }); return }
  if (modal?.node && on.modal) { on.modal({ title: modal.title, node: modal.node, size: modal.size, item: e.item }); return }
  if (select) {
    const scope = { ...(e.extra ?? {}), selection: on.selection }
    on.select(select.key, resolveActionArgs({ value: select.value }, e.item, scope, e.drafts).value)
    return
  }
  const { drafts: _drafts, ...hosted } = e
  on.host({ ...hosted, extra: { ...(e.extra ?? {}), selection: on.selection } })
}

/** pageOwn says whether an action is the page's own - it shows a tab, sets
 *  the selection, opens a node in a drawer or a modal -: run from a drawer
 *  or a modal, such an action leaves it open; any other closes it. */
export function pageOwn(a: UIPageAction): boolean {
  return !!(a.tab || a.select || a.drawer?.node || a.modal?.node)
}

/** askTab is the request after asking for a tab: counted, so asking the same
 *  tab again — after the person moved away — shows it again. */
export function askTab(prev: TabRequest | undefined, id: string, focus?: string): TabRequest {
  return focus ? { id, n: (prev?.n ?? 0) + 1, focus } : { id, n: (prev?.n ?? 0) + 1 }
}

/** focusSelectors are where a focus names an element, first match wins:
 *  a node's element id (`picker:sources`, or `sources`
 *  ending one), a row's or a card's name, a section's id, a DOM id. */
export function focusSelectors(focus: string): string[] {
  const v = focus.replace(/["\\]/g, '')
  return [`[data-element="${v}"]`, `[data-element$=":${v}"]`, `[data-guide-name="${v}"]`, `[data-section="${v}"]`, `[id="${v}"]`]
}

/** findFocus is the element a focus names under root, if it is drawn. */
export function findFocus(root: Pick<ParentNode, 'querySelector'> | null | undefined, focus: string): Element | null {
  if (!root) return null
  for (const s of focusSelectors(focus)) {
    const el = root.querySelector(s)
    if (el) return el
  }
  return null
}

/** FOCUS_TRIES is how many times, 100 ms apart, the page looks for a focus
 *  not drawn yet (a section loading it). */
export const FOCUS_TRIES = 20

/** FOCUS_MS is how long a focused element stays highlighted. */
export const FOCUS_MS = 2000

/** highlight brings an element into view and marks it for a while
 *  (`data-focused`, an outline), then unmarks it; it answers the unmarking. */
export function highlight(el: Element, ms = FOCUS_MS): () => void {
  const h = el as HTMLElement
  h.scrollIntoView?.({ behavior: 'smooth', block: 'center' })
  const before = { outline: h.style?.outline ?? '', offset: h.style?.outlineOffset ?? '' }
  h.setAttribute('data-focused', 'true')
  if (h.style) { h.style.outline = '2px solid var(--mantine-color-blue-5)'; h.style.outlineOffset = '2px' }
  let done = false
  const off = () => {
    if (done) return
    done = true
    h.removeAttribute('data-focused')
    if (h.style) { h.style.outline = before.outline; h.style.outlineOffset = before.offset }
  }
  setTimeout(off, ms)
  return off
}

/** tabShown is the panel a tabs node shows for a request: the one asked when
 *  it holds it and the request came after `since` (the last request when the
 *  node mounted — a node mounted later never jumps to an old ask), else none.
 *  A panel id is unique in its page: every tabs node holding it shows it. */
export function tabShown(asked: TabRequest | undefined, panels: string[], since = 0): string | undefined {
  return asked && asked.n > since && panels.includes(asked.id) ? asked.id : undefined
}

/** The element a template is rendered for (a list item, a tree row, a card);
 *  an action inside the template is on it. */
export const ItemContext = createContext<unknown>(undefined)

export function usePageContext(): PageContext {
  return useContext(DataContext)
}

export function useItem(): unknown {
  return useContext(ItemContext)
}

/** useRunAction answers how a node runs one of its actions: a function that
 *  hands it to the host with the element it is on, or undefined when it
 *  cannot run — the page has no host (a chat artifact), or the action names
 *  no verb or several (`actionVerb`). A node shows such an action disabled. */
export function useRunAction(): (action: UIPageAction | undefined) =>
  ((item?: unknown, extra?: Record<string, unknown>) => void) | undefined {
  const page = usePageContext()
  const { onAction, hasHost } = page
  return (action) => {
    if (!onAction || !action || !actionVerb(action)) return undefined
    if (!hasHost && !action.tab && !action.select && !action.drawer?.node && !action.modal?.node) return undefined
    if (action.select) {
      // A select reads a name the event lacks in the pages it runs in.
      return (item, extra) => {
        const e: PageActionEvent = { action, item, extra, drafts: [page.draft, ...enclosingDrafts(page)] }
        onAction(e)
      }
    }
    return (item, extra) => onAction({ action, item, extra })
  }
}

/** withItem lays the element over the page's data for a template: `$.item`
 *  is the element and `$.index` its position; every other path keeps the
 *  page's value. */
export function withItem(data: unknown, item: unknown, index: number): unknown {
  const base = data && typeof data === 'object' && !Array.isArray(data) ? data as Record<string, unknown> : {}
  return { ...base, item, index }
}
