// SPDX-License-Identifier: Apache-2.0
// The page's working copy of its data — how a form and a view of the same
// data stay in sync while the user types.
//
// - `data` is the producer's (the document's content, a stream update, a
//   patch). The page never writes to it.
// - A form bound by `dataField` writes its value at that path into the
//   working copy (`draft`); every node bound under it reads the draft, so a
//   view follows the form as the user types. Nothing reaches the producer
//   until the form submits.
// - A form reads `seed`, not the draft: the form holds its own edits, and a
//   value that changed identity on every keystroke would reset it. The seed
//   moves only when the producer sends new data.
// - New data from the producer replaces the draft, except at the paths the
//   user has edited: those keep the user's value, and a path whose producer
//   value changed meanwhile is reported in `upstream` (the form shows it).

import { useCallback, useMemo, useState } from 'react'

import { resolveJSONPath, setJSONPath } from '../../utils/json-path.ts'

export interface PageData {
  /** What bound nodes read: the producer's data with the user's edits. */
  draft: unknown
  /** What forms read: `draft` as it was when the producer last sent data. */
  seed: unknown
  /** Paths edited by the user whose producer value changed since. */
  upstream: ReadonlySet<string>
  /** A form's value at its `dataField` path. */
  write: (path: string, value: unknown) => void
}

interface State {
  base: unknown
  seed: unknown
  edits: ReadonlyMap<string, unknown>
  upstream: ReadonlySet<string>
}

function same(a: unknown, b: unknown): boolean {
  return a === b || JSON.stringify(a) === JSON.stringify(b)
}

function withEdits(data: unknown, edits: ReadonlyMap<string, unknown>): unknown {
  let out = data
  for (const [path, value] of edits) out = setJSONPath(out, path, value)
  return out
}

/** Fold new producer data into the state, keeping the user's edits. */
export function rebase(state: State, data: unknown): State {
  const upstream = new Set(state.upstream)
  for (const path of state.edits.keys()) {
    if (!same(resolveJSONPath(state.base, path), resolveJSONPath(data, path))) upstream.add(path)
  }
  return { base: data, seed: withEdits(data, state.edits), edits: state.edits, upstream }
}

/** Record a form's value; the producer's own value is not an edit. */
export function edit(state: State, path: string, value: unknown): State {
  const had = state.edits.has(path)
  if (same(value, resolveJSONPath(state.base, path))) {
    if (!had) return state
    const edits = new Map(state.edits)
    edits.delete(path)
    const upstream = new Set(state.upstream)
    upstream.delete(path)
    return { ...state, edits, upstream }
  }
  if (had && same(value, state.edits.get(path))) return state
  return { ...state, edits: new Map(state.edits).set(path, value) }
}

export function initialState(data: unknown): State {
  return { base: data, seed: data, edits: new Map(), upstream: new Set() }
}

export function usePageData(data: unknown): PageData {
  const [state, setState] = useState(() => initialState(data))
  // New producer data: fold it in during render (React's "adjust state when
  // a prop changes" pattern), so no frame shows the stale draft.
  let current = state
  if (state.base !== data) {
    current = rebase(state, data)
    setState(current)
  }
  const write = useCallback((path: string, value: unknown) => {
    setState((s) => edit(s, path, value))
  }, [])
  const draft = useMemo(() => withEdits(current.base, current.edits), [current.base, current.edits])
  return {
    draft,
    seed: current.seed,
    upstream: current.upstream,
    write,
  }
}
