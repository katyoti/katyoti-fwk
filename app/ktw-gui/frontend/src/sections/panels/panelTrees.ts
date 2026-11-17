// SPDX-License-Identifier: Apache-2.0
// panelTrees — every tree a display panel draws made self-contained before it
// is drawn: its page tree (a view naming its schema by id, as the
// release cards do) and its question's form (a data type named by URI). The
// renderer draws inline schemas only and refuses a reference on purpose; the
// panel resolves them the way every other host does (RenderDefView, the
// host's loadSection): the object pane's ResolvePanel, asked only when the
// tree still needs it (needsResolve). A failure is shown in the panel, never
// thrown at the screen.

import { useEffect, useMemo, useState } from 'react'

import { needsResolve } from '../../lib/specCache'
import { resolvePanelTree, shownResolution, type Resolution } from '../workspace/resolvePanel'

/** Resolves one tree: the object pane's ResolvePanel, or a test's stub. */
export type TreeResolve = (tree: unknown) => Promise<unknown>

/** drawnTree is the tree a panel draws: the tree itself when it holds no
 *  reference left, else the tree resolved. */
export async function drawnTree(tree: unknown, resolve: TreeResolve): Promise<unknown> {
  return needsResolve(tree) ? resolve(tree) : tree
}

/** panelTreeResolve resolves a tree through a universe's object pane. */
export function panelTreeResolve(universeName: string): TreeResolve {
  return (tree) => resolvePanelTree(universeName, tree)
}

/** useDrawnTree answers what a panel draws for `tree`: at once when it needs
 *  no resolution, else once resolved - the last resolved tree of the same
 *  `identity` staying drawn while the next one resolves, so an op redraws
 *  the page in place. `onResolved` is told each resolution (the panel keeps
 *  it as its page, so later ops apply by node id over the resolved tree). */
export function useDrawnTree(
  tree: unknown,
  identity: string,
  resolve: TreeResolve,
  onResolved?: (from: unknown, to: unknown) => void,
): { tree?: unknown; error?: string } {
  const needs = tree !== undefined && needsResolve(tree)
  const key = needs ? JSON.stringify(tree) : ''
  const [got, setGot] = useState<Resolution>({ key: '' })
  useEffect(() => {
    if (!needs) return
    let gone = false
    drawnTree(tree, resolve).then(
      (to) => {
        if (gone) return
        setGot({ identity, key, tree: to })
        onResolved?.(tree, to)
      },
      (err: unknown) => { if (!gone) setGot({ identity, key, error: String((err as Error)?.message ?? err) }) },
    )
    return () => { gone = true }
  }, [identity, key]) // eslint-disable-line react-hooks/exhaustive-deps
  if (tree === undefined) return {}
  if (!needs) return { tree }
  return shownResolution(key, got, identity)
}

/** formShell splits a question's form node in two: the
 *  node without its data - what is resolved, the same for every ask of the
 *  same data type, so asking again after a refusal starts no new resolution
 *  - and its data (the ask's value), laid over the resolved node. */
export function formShell(form: unknown): { shell?: unknown; data?: unknown } {
  if (!form || typeof form !== 'object' || Array.isArray(form)) return { shell: form }
  const { data, ...shell } = form as Record<string, unknown>
  return { shell, data }
}

/** withFormData lays a question's value over its resolved form node. */
export function withFormData(tree: unknown, data: unknown): unknown {
  if (!tree || typeof tree !== 'object' || Array.isArray(tree) || data === undefined) return tree
  return { ...(tree as Record<string, unknown>), data }
}

/** useDrawnForm answers the form a panel's question draws: its schema
 *  resolved once (useDrawnTree over formShell), its value laid over it - a
 *  question asked again with another value draws at once, over the form the
 *  person types into. */
export function useDrawnForm(form: unknown, identity: string, resolve: TreeResolve): { tree?: unknown; error?: string } {
  const { shell, data } = formShell(form)
  const drawn = useDrawnTree(shell, identity, resolve)
  const tree = useMemo(() => withFormData(drawn.tree, data), [drawn.tree, data])
  return { tree, error: drawn.error }
}

/** canonicalJSON writes a value as JSON with every object's keys sorted -
 *  Go re-marshals a map with its keys sorted, so a value sent and the same
 *  value asked again compare equal whatever their key order. */
function canonicalJSON(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(canonicalJSON).join(',')}]`
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>
    return `{${Object.keys(o).filter((k) => o[k] !== undefined).sort().map((k) => `${JSON.stringify(k)}:${canonicalJSON(o[k])}`).join(',')}}`
  }
  return JSON.stringify(v ?? null)
}

/** sameValue tells two answer values equal, keys in any order. */
export function sameValue(a: unknown, b: unknown): boolean {
  return canonicalJSON(a) === canonicalJSON(b)
}

/** The identity of a panel question's form: its shell (the
 *  form without its value) and an ask generation; `key` keys the drawn form. */
export interface FormIdentity { shell: string; gen: number; key: string }

/** nextFormIdentity answers the identity of a new ask's form. A refusal asks
 *  again over the same form with the very value the person just sent: that
 *  re-ask keeps the previous identity, so the form stays mounted and keeps
 *  what was typed. Any other ask - a new question, another
 *  data type, or an ask with nothing sent through the form - takes a new
 *  generation, so its form starts fresh from its own value. */
export function nextFormIdentity(prev: FormIdentity | undefined, form: unknown, lastSent: unknown): FormIdentity {
  const { shell, data } = formShell(form)
  const shellKey = canonicalJSON(shell)
  if (prev && prev.shell === shellKey && lastSent !== undefined && sameValue(data ?? {}, lastSent)) return prev
  const gen = (prev?.gen ?? 0) + 1
  return { shell: shellKey, gen, key: `${shellKey}|${gen}` }
}
