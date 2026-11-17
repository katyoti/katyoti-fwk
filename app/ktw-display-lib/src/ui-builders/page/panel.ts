// SPDX-License-Identifier: Apache-2.0
// A display panel as a screen follows it (F-061): its events
// - the Go side's internal/core/panel Event - folded into what the screen
// draws. Its open carries the one answer (answer.ts), its ops the one op set by
// node id: a panel adds its identity, its state and its lifecycle, no other
// shape. The host gives the transport (HostServices.followPanel); this file is
// the pure part.

import { applyPageOps, isNamedSpec, pageState, type PageAnswer, type PageOp, type PageState } from './answer.ts'
import type { UIPageNode } from './ui-page.types.ts'

/** What a panel is doing: open (running), asking (waiting for the person),
 *  complete (its work ended), closed. */
export type PanelState = 'open' | 'asking' | 'complete' | 'closed'

/** One event out of a panel, in the order of its seq (1 for open). */
export interface PanelEvent {
  panel: string
  seq: number
  kind: 'open' | 'ops' | 'progress' | 'ask' | 'complete' | 'close'
  open?: { type?: string; title?: string; owner?: string; parent?: string; answer?: PageAnswer }
  ops?: PageOp[]
  progress?: { label?: string; current: number; total?: number }
  ask?: { question?: string; form?: unknown; value?: unknown; actions?: { name: string; label?: string }[] }
  complete?: { status: string; result?: unknown }
}

/** A panel as its events built it so far. `page` is its tree and its ops'
 *  state - absent while its tree is named by URI and not resolved, or before
 *  its open. */
export interface PanelView {
  id: string
  seq: number
  state?: PanelState
  type?: string
  title?: string
  owner?: string
  /** The tree as its open named it, when it is named by URI: the host
   *  resolves it (its spec-part cache). */
  treeSpec?: { uri: string; hash?: string }
  /** While its named tree is not resolved: the open's seq and the ops taken
   *  since, in order - each event past reducePanel's seq guard once, so a
   *  replayed one is never kept twice. withPanelTemplate lays them over the
   *  template. */
  waiting?: { seq: number; ops: PageOp[] }
  page?: PageState
  data?: unknown
  progress?: PanelEvent['progress']
  ask?: PanelEvent['ask']
  complete?: PanelEvent['complete']
}

/** What the person sends a panel (the Go side's panel.Input): the answer to
 *  its question - a form's value (submit) or a button (action) -, a chat
 *  turn (message), or a cancel of its work. */
export interface PanelInput {
  panel: string
  kind: 'submit' | 'action' | 'message' | 'cancel'
  name?: string
  value?: unknown
  text?: string
  by?: string
}

/** panelAnswer is the person's answer to the question the panel asks: the
 *  action chosen among those it offers, else a form's value, else the text
 *  written; null when there is nothing to send. */
export function panelAnswer(view: Pick<PanelView, 'id' | 'ask'>, answer: { action?: string; value?: unknown; text?: string }, by?: string): PanelInput | null {
  const who = by ? { by } : {}
  const actions = view.ask?.actions ?? []
  if (actions.length > 0) {
    const name = answer.action ?? actions[0].name
    if (!actions.some((a) => a.name === name)) return null
    return { panel: view.id, kind: 'action', name, ...who }
  }
  if (answer.value !== undefined) return { panel: view.id, kind: 'submit', value: answer.value, ...who }
  const text = (answer.text ?? '').trim()
  return text === '' ? null : { panel: view.id, kind: 'submit', value: text, ...who }
}

/** panelAskForm is the form a question carrying one draws: the page's form
 *  node over its data type - by URI, by `{uri, hash}`, as a form node
 *  `{kind, schema}` naming it either way, or its schema inline -, prefilled with
 *  the question's value, its submit the answer (panel:answer); undefined when
 *  the question offers choices or carries no form (a text is written then).
 *  The one builder of the question's form: the inline panel node and every
 *  host drawing a panel use it. */
export function panelAskForm(ask: PanelView['ask']): UIPageNode | undefined {
  if (!ask || (ask.actions?.length ?? 0) > 0 || !ask.form) return undefined
  const schema = askFormSchema(ask.form)
  return { kind: 'form', id: 'panel:ask-form', schema, data: ask.value, hideTitle: true, mode: 'edit', submit: { uri: 'panel:answer' } } as unknown as UIPageNode
}

/** askFormSchema is the schema a question's form names: a node's own schema
 *  when the form is given as a node (`{kind, schema}`), a URI for a
 *  `{uri, hash}`, else the form itself (a URI or a schema inline). */
function askFormSchema(form: unknown): unknown {
  const f = form && typeof form === 'object' && !Array.isArray(form) && typeof (form as Record<string, unknown>).kind === 'string' && 'schema' in (form as Record<string, unknown>)
    ? (form as Record<string, unknown>).schema
    : form
  return isNamedSpec(f) ? f.uri : f
}

/** panelCancel asks the run owning the panel to stop its work. */
export function panelCancel(panel: string, by?: string): PanelInput {
  return { panel, kind: 'cancel', ...(by ? { by } : {}) }
}

/** isPanelId tells a panel id: dp- and a lower-case UUID. */
export function isPanelId(v: unknown): v is string {
  return typeof v === 'string' && /^dp-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v)
}

export function panelView(id: string): PanelView {
  return { id, seq: 0 }
}

/** reducePanel folds one event into the view: an event of another panel, or
 *  at or below the last seq folded, changes nothing. Pure. */
export function reducePanel(v: PanelView, e: PanelEvent): PanelView {
  if (e.panel !== v.id || e.seq <= v.seq) return v
  const next: PanelView = { ...v, seq: e.seq }
  // The run sends nothing when it takes the person's answer: what it sends
  // next says it runs again.
  if (v.state === 'asking' && (e.kind === 'ops' || e.kind === 'progress')) {
    next.state = 'open'
    next.ask = undefined
  }
  switch (e.kind) {
    case 'open': {
      const a = e.open?.answer ?? {}
      next.state = 'open'
      next.type = e.open?.type
      next.title = e.open?.title
      next.owner = e.open?.owner
      next.data = a.data
      // An open starts the panel's page again: nothing of an earlier open
      // stays (its page, its named tree, its question, its progress, its end).
      next.page = undefined
      next.treeSpec = undefined
      next.waiting = undefined
      next.ask = undefined
      next.progress = undefined
      next.complete = undefined
      if (isNamedSpec(a.tree)) {
        next.treeSpec = a.tree
        next.waiting = { seq: e.seq, ops: [] }
      } else if (a.tree !== undefined) next.page = { ...pageState(a.tree), seq: e.seq }
      return next
    }
    case 'ops':
      if (next.page) next.page = applyPageOps(next.page, e.ops ?? [])
      else if (next.waiting) next.waiting = { ...next.waiting, ops: [...next.waiting.ops, ...(e.ops ?? [])] }
      return next
    case 'progress':
      next.progress = e.progress
      return next
    case 'ask':
      next.state = 'asking'
      next.ask = e.ask
      return next
    case 'complete':
      next.state = 'complete'
      next.ask = undefined
      next.complete = e.complete
      return next
    case 'close':
      next.state = 'closed'
      return next
    default:
      return v
  }
}

/** templatePage is a panel's page once the template its open named is drawn:
 *  the template's tree at the open's seq, with the ops taken while it was
 *  asked laid over it in order. */
export function templatePage(tree: unknown, openSeq: number, ops: PageOp[]): PageState {
  return applyPageOps({ ...pageState(tree), seq: openSeq }, ops)
}

/** withPanelTemplate is the view once the template its open named by
 *  `{uri, hash}` is fetched: its page built from the tree and the ops taken
 *  meanwhile - only while the view still names that tree and holds no page;
 *  a template of an earlier open changes nothing. Pure: the inline panel node
 *  and every host drawing a panel use it. */
export function withPanelTemplate(view: PanelView, spec: { uri: string; hash?: string }, tree: unknown): PanelView {
  const named = view.treeSpec
  if (view.page || !named || tree === undefined || named.uri !== spec.uri || (named.hash ?? '') !== (spec.hash ?? '')) return view
  const held = view.waiting
  return { ...view, page: templatePage(tree, held?.seq ?? view.seq, held?.ops ?? []), waiting: undefined }
}

/** panelTemplateFetch fetches the template a view waits for through the
 *  host's `fetch` (HostServices.panelTemplate) and answers the step laying it
 *  on the view as it is then (withPanelTemplate); undefined when the view
 *  waits for no template or no fetch is given. */
export function panelTemplateFetch(
  view: PanelView,
  fetch?: (spec: { uri: string; hash?: string }) => Promise<Record<string, unknown> | undefined>,
): Promise<(v: PanelView) => PanelView> | undefined {
  const spec = view.page ? undefined : view.treeSpec
  if (!spec || !fetch) return undefined
  return fetch(spec).then((tree) => (v: PanelView) => (tree ? withPanelTemplate(v, spec, tree) : v))
}
