// SPDX-License-Identifier: Apache-2.0
// The one answer: what a page source, a section's load, an
// action's result and a display panel's opening event carry, and the one op
// set that patches any node of a page by its id, pulled or pushed. The Go
// side is internal/core/render/answer.go; modules/display/page-dsl.md
// « Sections, answers and ops » states the contract.

import { mergePatch } from '../../utils/merge-patch.ts'

/** A part of a specification - a page tree, a data type, a ui variant -
 *  inline (the part itself) or named by its contribution URI and the sha256
 *  of its body. Only a named part is cached (by uri + hash). */
export type Spec = { uri: string; hash?: string } | unknown

/** isNamedSpec tells a named part ({uri} or {uri, hash}, nothing else) from
 *  an inline one. */
export function isNamedSpec(s: unknown): s is { uri: string; hash?: string } {
  if (!s || typeof s !== 'object' || Array.isArray(s)) return false
  const keys = Object.keys(s as Record<string, unknown>)
  const uri = (s as Record<string, unknown>).uri
  return typeof uri === 'string' && uri !== '' && keys.every((k) => k === 'uri' || k === 'hash')
}

/** The answer: a tree (a page root, or one node for a section), the parts it
 *  references, its data - or a display panel whose events carry the same
 *  answer and ops. */
export interface PageAnswer {
  tree?: Spec
  parts?: Record<string, Spec>
  data?: unknown
  panel?: string
}

export type PageOpKind = 'replace' | 'append' | 'merge' | 'remove' | 'fail' | 'complete'

/** One change to a page by node id, applied in the order of seq. */
export interface PageOp {
  seq: number
  op: PageOpKind
  /** A node id, unique in the page and stable across rebuilds; never a JSON
   *  Pointer. complete takes none. */
  target?: string
  node?: Record<string, unknown>
  items?: unknown[]
  value?: unknown
  message?: string
}

/** A section's load: the method the host calls, and its args. */
export interface SectionLoad {
  method: string
  args?: Record<string, unknown>
}

/** The props any node takes to be a section. */
export interface SectionProps {
  id: string
  load: SectionLoad
  placeholder?: { kind: 'skeleton' | 'cards' | 'rows'; count?: number }
  refresh?: SectionRefresh
  when?: 'visible' | 'open' | 'eager'
}

/** isSection tells a node carrying a section's load. */
export function isSection(n: unknown): n is Record<string, unknown> & SectionProps {
  if (!n || typeof n !== 'object') return false
  const r = n as Record<string, unknown>
  const load = r.load as Record<string, unknown> | undefined
  return typeof r.id === 'string' && r.id !== '' && !!load && typeof load.method === 'string' && load.method !== ''
}

/** isPushedSection tells a section its producer pushes: an
 *  id and a placeholder object, no load - the producer of a page panel
 *  replaces it by its id. (An input's `placeholder` text is no section.) */
export function isPushedSection(n: unknown): n is Record<string, unknown> & { id: string; placeholder: SectionProps['placeholder'] } {
  if (!n || typeof n !== 'object') return false
  const r = n as Record<string, unknown>
  const ph = r.placeholder as Record<string, unknown> | undefined
  return typeof r.id === 'string' && r.id !== '' && r.load === undefined && !!ph && typeof ph === 'object' &&
    (ph.kind === 'skeleton' || ph.kind === 'cards' || ph.kind === 'rows')
}

/** The state ops build: the tree, the nodes failed (id → message), whether
 *  the page completed, the last seq applied and the ops whose target is not
 *  drawn yet (held until it is; the host drops them after a short while). */
export interface PageState {
  tree: unknown
  failed: Record<string, string>
  complete: boolean
  seq: number
  pending: PageOp[]
}

export function pageState(tree: unknown): PageState {
  return { tree, failed: {}, complete: false, seq: 0, pending: [] }
}

// The keys a node holds its children under, as the Go walker reads them
// (internal/core/render/page.go ValidatePage).
const CHILD_LISTS = ['children', 'aside'] as const
const CHILD_ENTRIES = ['tabs', 'panels'] as const
const CHILD_ONE = ['item', 'main', 'side', 'panel'] as const

type Edit = (n: Record<string, unknown>) => Record<string, unknown> | null

/** editById rebuilds the tree with the node id passed through edit (null
 *  removes it); found says whether the id was met. Pure. */
function editById(v: unknown, id: string, edit: Edit): { v: unknown; found: boolean } {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return { v, found: false }
  const n = v as Record<string, unknown>
  if (n.id === id) return { v: edit(n), found: true }
  let found = false
  let out: Record<string, unknown> | null = null
  const set = (k: string, val: unknown) => {
    if (!out) out = { ...n }
    out[k] = val
  }
  for (const k of CHILD_LISTS) {
    const kids = n[k]
    if (!Array.isArray(kids) || found) continue
    const next: unknown[] = []
    for (const c of kids) {
      if (found) { next.push(c); continue }
      const r = editById(c, id, edit)
      if (r.found) {
        found = true
        if (r.v !== null) next.push(r.v)
      } else next.push(c)
    }
    if (found) set(k, next)
  }
  for (const k of CHILD_ENTRIES) {
    const entries = n[k]
    if (!Array.isArray(entries) || found) continue
    const next = entries.map((e) => {
      if (found || !e || typeof e !== 'object') return e
      const r = editById((e as Record<string, unknown>).content, id, edit)
      if (!r.found) return e
      found = true
      return { ...(e as Record<string, unknown>), content: r.v }
    })
    if (found) set(k, next)
  }
  for (const k of CHILD_ONE) {
    if (found || !(k in n)) continue
    const r = editById(n[k], id, edit)
    if (r.found) { found = true; set(k, r.v) }
  }
  return { v: out ?? n, found }
}

/** applyPageOps applies ops in the order of seq to a page state, by node
 *  id; an op already applied (seq at or below the state's) is skipped, an op
 *  whose target is not in the tree waits in pending and is tried again with
 *  the next ops. Pure: returns a new state. */
export function applyPageOps(state: PageState, ops: PageOp[]): PageState {
  let s: PageState = { ...state, failed: { ...state.failed }, pending: [] }
  const todo = [...state.pending, ...ops].sort((a, b) => a.seq - b.seq)
  for (const op of todo) {
    if (op.seq <= s.seq && !state.pending.includes(op)) continue
    if (op.op === 'complete') {
      s = { ...s, complete: true, seq: Math.max(s.seq, op.seq) }
      continue
    }
    const id = op.target ?? ''
    const edit: Edit | undefined = {
      replace: () => (op.node ? { ...op.node, id } : null),
      remove: () => null,
      merge: (n: Record<string, unknown>) => ({ ...(mergePatch(n, op.value) as Record<string, unknown>), id }),
      append: (n: Record<string, unknown>) => ({ ...n, data: [...(Array.isArray(n.data) ? n.data : []), ...(op.items ?? [])] }),
      fail: (n: Record<string, unknown>) => n,
    }[op.op as Exclude<PageOpKind, 'complete'>]
    if (!edit) continue
    const r = editById(s.tree, id, edit)
    if (!r.found) {
      s.pending.push(op)
      continue
    }
    const failed = { ...s.failed }
    if (op.op === 'fail') failed[id] = op.message ?? ''
    else delete failed[id]
    s = { ...s, tree: r.v, failed, seq: Math.max(s.seq, op.seq) }
  }
  return s
}

/** What re-reads a section: object changes in its scope - the
 *  panel's PanelRefresh `{scope, types}` - about its object or module when it
 *  names one (`self` and `children` read `object`, `module` its module).
 *  Without them the page's place decides, as a panel's refresh does. */
export interface SectionRefresh {
  scope: string
  types?: string[]
  object?: string
  module?: string
}
