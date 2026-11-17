// SPDX-License-Identifier: Apache-2.0
// Cell + header renderer composition utilities — `wrapColumnRenderers`
// + `wrapHeaderRenderers` let a single column show MULTIPLE field values
// in one cell (e.g. avatar + name in one cell).
//
// Ported from _ktw_workspace/.../cpm-ui/ui-table/ui-table.utils.tsx during
// gui-table Subtask 2 (2026-05-04). No semantic changes.

import type { CSSProperties } from 'react'
import { Group, Stack } from '@mantine/core'
import { color } from '../fields/_planning/palette.ts'
import type { JsonObject } from '../../types/json-types.ts'
import type { UIColumnRenderer, UIHeaderRenderer } from '../fields/index.ts'

export function wrapColumnRenderers<TData extends JsonObject = JsonObject>(
  columns: UIColumnRenderer<TData>[],
  vertical?: boolean,
): UIColumnRenderer<TData> {
  const Cmp = vertical ? Stack : Group
  return (item: TData, index: number) => (
    <Cmp align="center" gap={2}>
      {columns.map((column) => column(item, index))}
    </Cmp>
  )
}

export function wrapHeaderRenderers(
  columns: UIHeaderRenderer[],
  vertical?: boolean,
): UIHeaderRenderer {
  const Cmp = vertical ? Stack : Group
  return () => (
    <Cmp align="center" gap={2}>
      {columns.map((column) => column())}
    </Cmp>
  )
}

/** A field drawn beside a column's own value (`prepend` / `append`): its
 *  name, its content-type name, its column renderer. */
export type UIColumnExtra<TData = any> = {
  id: string
  content?: string
  renderCell: UIColumnRenderer<TData>
}

/** emptyExtra is whether a prepended or appended field has nothing to show:
 *  it is then left out - never a dash beside the cell's own value. */
function emptyExtra(item: unknown, id: string): boolean {
  const v = (item as Record<string, unknown> | undefined)?.[id]
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
}

/** UNDER_LINE styles the second line of a cell (a column's `below`): dimmed
 *  and smaller, a Mantine Text inside it included (`--text-fz`). */
export const UNDER_LINE = {
  display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', minWidth: 0,
  fontSize: 12, lineHeight: 1.35, color: color('muted'), '--text-fz': '12px', '--text-lh': '1.35',
} as CSSProperties

/** UNDER_CELL stacks a cell's own line over its second line. */
export const UNDER_CELL: CSSProperties = { display: 'inline-flex', flexDirection: 'column', alignItems: 'flex-start', gap: 2, minWidth: 0 }

/** joinCell draws a column's own cell between its prepended and appended
 *  fields, inline, and its `below` fields on a second line under it; an
 *  empty field takes no room, and with none below the cell stays one line.
 *  The cell has no handler of its own: the row keeps its one hover and open. */
export function joinCell<TData extends JsonObject = JsonObject>(
  before: UIColumnExtra<TData>[],
  own: UIColumnRenderer<TData>,
  after: UIColumnExtra<TData>[],
  under: UIColumnExtra<TData>[] = [],
): UIColumnRenderer<TData> {
  const line: UIColumnRenderer<TData> = before.length === 0 && after.length === 0 ? own : (item: TData, index: number) => (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', minWidth: 0 }}>
      {before.filter((e) => !emptyExtra(item, e.id)).map((e) => <span key={'b-' + e.id}>{e.renderCell(item, index)}</span>)}
      <span style={{ minWidth: 0 }}>{own(item, index)}</span>
      {after.filter((e) => !emptyExtra(item, e.id)).map((e) => <span key={'a-' + e.id}>{e.renderCell(item, index)}</span>)}
    </span>
  )
  if (under.length === 0) return line
  return (item: TData, index: number) => {
    const shown = under.filter((e) => !emptyExtra(item, e.id))
    if (shown.length === 0) return line(item, index)
    return (
      <span style={UNDER_CELL}>
        <span style={{ minWidth: 0 }}>{line(item, index)}</span>
        <span data-ktw-below="" style={UNDER_LINE}>
          {shown.map((e) => <span key={'u-' + e.id}>{e.renderCell(item, index)}</span>)}
        </span>
      </span>
    )
  }
}
