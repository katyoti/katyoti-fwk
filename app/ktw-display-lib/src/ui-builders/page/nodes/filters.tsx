// SPDX-License-Identifier: Apache-2.0
// Page filters: a chip group with counts, a search box and a
// switch, each setting one key of the page selection - so the sections reading
// `${selection.<key>}` re-read, and the nodes carrying a `filter` keep only
// the rows that pass it. The rows stay the producer's: a filter only hides.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Switch, TextInput } from '@mantine/core'

import { MONO } from '../../fields/_planning/marks.tsx'
import { color } from '../../fields/_planning/palette.ts'
import { useItem, useRunAction, useSelection } from '../page-context.tsx'
import type { UIChipOption, UIChipsNode, UISearchNode, UISwitchNode } from '../ui-page.interactive.ts'
import type { UIFilterClause } from '../ui-page.types.ts'

/** unset is a selection value that filters nothing: absent, null, empty. */
function unset(v: unknown): boolean {
  return v === undefined || v === null || v === ''
}

function text(v: unknown): string {
  return v === undefined || v === null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v)
}

function at(row: unknown, path: string): unknown {
  let v: unknown = row
  for (const k of path.split('.')) v = v && typeof v === 'object' ? (v as Record<string, unknown>)[k] : undefined
  return v
}

/** passes says whether a row passes one clause for a selection:
 *  - `equals` (the default): the row's `field` is the key's value; the key's
 *    value `empty` (when the clause names one) keeps the rows whose field is
 *    empty - a « No stream » chip;
 *  - `contains`: one of `fields` (or `field`) holds the key's text, ignoring case;
 *  - `unless`: a row whose `field` is set shows only while the key is on - a
 *    « Show held increments » switch.
 *  A clause whose key is unset keeps every row (`unless` hides then). */
export function passes(row: unknown, c: UIFilterClause, selection: Record<string, unknown>): boolean {
  const v = selection[c.key]
  const match = c.match ?? 'equals'
  if (match === 'unless') {
    const flagged = at(row, c.field ?? '')
    return !flagged || v === true
  }
  if (unset(v)) return true
  if (match === 'contains') {
    const q = text(v).trim().toLowerCase()
    if (!q) return true
    const fields = c.fields ?? (c.field ? [c.field] : [])
    return fields.some((f) => text(at(row, f)).toLowerCase().includes(q))
  }
  const got = at(row, c.field ?? '')
  if (c.empty !== undefined && text(v) === c.empty) return unset(got)
  return text(got) === text(v)
}

/** filterRows keeps the rows passing every clause. */
export function filterRows<T>(rows: T[], clauses: UIFilterClause[] | undefined, selection: Record<string, unknown>): T[] {
  if (!clauses || clauses.length === 0) return rows
  return rows.filter((r) => clauses.every((c) => passes(r, c, selection)))
}

/** withFilter is a node with its `data` rows filtered by its `filter`
 *  against the selection; any other node as it is. */
export function withFilter<N extends { filter?: UIFilterClause[]; data?: unknown }>(node: N, selection: Record<string, unknown>): N {
  if (!node.filter || !Array.isArray(node.data)) return node
  return { ...node, data: filterRows(node.data, node.filter, selection) }
}

/** chipCount is an option's count: its own `count`, else the rows of
 *  `countRows` whose `countField` is its value (every row for the `all`
 *  option, the empty-field rows for the `empty` one). */
export function chipCount(o: UIChipOption, node: Pick<UIChipsNode, 'countRows' | 'countField' | 'empty'>): number | undefined {
  if (typeof o.count === 'number') return o.count
  if (!Array.isArray(node.countRows) || !node.countField) return undefined
  const rows = node.countRows
  if (unset(o.value)) return rows.length
  if (node.empty !== undefined && o.value === node.empty) return rows.filter((r) => unset(at(r, node.countField!))).length
  return rows.filter((r) => text(at(r, node.countField!)) === o.value).length
}

/** chipOptions are a chip group's options, its `all` option first. */
export function chipOptions(node: Pick<UIChipsNode, 'options' | 'all'>): UIChipOption[] {
  const opts = node.options ?? []
  return node.all ? [{ value: '', label: node.all }, ...opts] : opts
}

export function ChipsRenderer({ node }: { node: UIChipsNode }): ReactNode {
  const { selection, select } = useSelection()
  const current = text(selection[node.key])
  return (
    <div {...dataAttrs(node)} role="group" aria-label={node.label ?? node.key} data-ktw-chips={node.key}
      style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
      {chipOptions(node).map((o) => {
        const on = current === o.value
        const n = chipCount(o, node)
        return (
          <button key={o.value || '(all)'} type="button" aria-pressed={on} data-value={o.value}
            onClick={() => select(node.key, o.value)}
            style={{
              font: 'inherit', fontSize: 12.5, padding: '3px 10px', borderRadius: 999, cursor: 'pointer',
              display: 'inline-flex', gap: 6, alignItems: 'center',
              border: `1px solid ${on ? color('accent') : color('line')}`,
              background: on ? color('soft') : color('surface'), color: on ? color('accent') : 'inherit',
              fontWeight: on ? 600 : 400,
            }}>
            {o.color && <i style={{ width: 9, height: 9, borderRadius: '50%', background: color(o.color) }} />}
            {o.label ?? o.value}
            {n !== undefined && <span style={{ fontFamily: MONO, fontSize: 11, color: color('muted') }}>{n}</span>}
          </button>
        )
      })}
    </div>
  )
}

/** The pause after the last keystroke before a search sets its key. */
export const SEARCH_DELAY_MS = 200

export function SearchRenderer({ node }: { node: UISearchNode }): ReactNode {
  const { selection, select } = useSelection()
  const [value, setValue] = useState(() => text(selection[node.key]))
  const timer = useRef<ReturnType<typeof setTimeout>>()
  useEffect(() => () => clearTimeout(timer.current), [])
  return (
    <TextInput {...dataAttrs(node)} size="xs" type="search" value={value} placeholder={node.placeholder}
      aria-label={node.label ?? node.placeholder ?? node.key} data-ktw-search={node.key}
      w={node.width ?? 220}
      onChange={(e) => {
        const v = e.currentTarget.value
        setValue(v)
        clearTimeout(timer.current)
        timer.current = setTimeout(() => select(node.key, v.trim()), SEARCH_DELAY_MS)
      }} />
  )
}

export function SwitchRenderer({ node }: { node: UISwitchNode }): ReactNode {
  const { selection, select } = useSelection()
  const item = useItem()
  const run = useRunAction()(node.action)
  if (node.action) {
    // An action switch: drawn at `checked`, a toggle runs its action with
    // `${value}` the new state; the producer's next answer redraws it.
    const on = node.checked === true
    return (
      <Switch {...dataAttrs(node)} size="xs" label={node.label} checked={on} disabled={!run} data-ktw-switch={node.key}
        data-element={`switch:${node.key}`} onClick={(e) => e.stopPropagation()}
        onChange={() => run?.(item, { value: on ? 'false' : 'true' })} />
    )
  }
  return (
    <Switch {...dataAttrs(node)} size="xs" label={node.label} checked={selection[node.key] === true} data-ktw-switch={node.key}
      onChange={(e) => select(node.key, e.currentTarget.checked)} />
  )
}
