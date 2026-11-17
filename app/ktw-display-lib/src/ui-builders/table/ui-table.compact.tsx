// SPDX-License-Identifier: Apache-2.0
// The compact table (`density: compact`): dense aligned rows
// that select or open, each with its menu — the delivery mock-up's `.row`
// list, not a spreadsheet: no header (unless `header: true`), no zebra, 1px
// separators, a rounded border around the list, a hover, the selected row
// highlighted. Columns align on one CSS grid: a column's `width` is its
// track; the first column without one is the row's title (bold, cut with an
// ellipsis), any later one secondary text (dimmed); a last `auto` track
// holds the row's actions and menu.

import { useState, type CSSProperties, type KeyboardEvent, type ReactNode } from 'react'
import { Tooltip } from '@mantine/core'

import { color } from '../fields/_planning/palette.ts'
import type { UIColumn } from './ui-table.types.ts'
import { UNDER_CELL, UNDER_LINE, type UIColumnExtra } from './ui-table.utils.tsx'

/** A column's role in a compact row. */
export type CompactRole = 'fixed' | 'title' | 'secondary'

/** compactRoles: a column with a width is fixed; the first without one is
 *  the title, the others secondary. */
export function compactRoles(columns: Pick<UIColumn, 'id' | 'width'>[]): CompactRole[] {
  let titled = false
  return columns.map((c) => {
    if (c.width !== undefined && c.width !== null && c.width !== '') return 'fixed'
    if (!titled) { titled = true; return 'title' }
    return 'secondary'
  })
}

/** compactGrid is the rows' grid-template-columns: each width as its track
 *  (a number in pixels), the title `minmax(min(16em,100%),2.4fr)` - a title
 *  and its id stay readable before a secondary column gives way -, a
 *  secondary `minmax(0,1.2fr)`, and an `auto` track when the row has
 *  trailing cells. */
export function compactGrid(columns: Pick<UIColumn, 'id' | 'width'>[], trailing: boolean): string {
  const roles = compactRoles(columns)
  const tracks = columns.map((c, i) => {
    if (roles[i] === 'fixed') return typeof c.width === 'number' ? `${c.width}px` : String(c.width)
    return roles[i] === 'title' ? 'minmax(min(16em,100%),2.4fr)' : 'minmax(0,1.2fr)'
  })
  if (trailing) tracks.push('auto')
  return tracks.join(' ')
}

/** What a click on a row runs: its selection, else its open, else the
 *  processor's row click, else nothing. */
export function rowClickKind(on: { select?: unknown; open?: unknown; processor?: unknown }): 'select' | 'open' | 'processor' | undefined {
  if (on.select) return 'select'
  if (on.open) return 'open'
  if (on.processor) return 'processor'
  return undefined
}

const PLAIN = new Set(['text', 'json', undefined])

const ELLIPSIS: CSSProperties = { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }

function cellOf<T>(column: UIColumn<T>, role: CompactRole, row: T, index: number): ReactNode {
  // Plain text takes the row's typography (a Mantine Text would set its own
  // size); every other content draws through its column renderer.
  if (PLAIN.has(column.content)) {
    const v = (row as Record<string, unknown>)[column.id]
    const text = v === undefined || v === null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v)
    const style: CSSProperties = role === 'title'
      ? { ...ELLIPSIS, fontWeight: 600 }
      : role === 'secondary' ? { ...ELLIPSIS, fontSize: 12.5, color: color('muted') } : ELLIPSIS
    const before = (column.before ?? []).filter((e) => !extraEmpty(row, e.id))
    const after = (column.after ?? []).filter((e) => !extraEmpty(row, e.id))
    const hover = column.hover ? (row as Record<string, unknown>)[column.hover] : undefined
    const own = (s: CSSProperties) => typeof hover === 'string' && hover !== ''
      ? <Tooltip label={hover} multiline w={360} withArrow openDelay={300} style={{ whiteSpace: 'pre-wrap' }}><span style={s}>{text}</span></Tooltip>
      : <span style={s} title={text || undefined}>{text}</span>
    const under = (column.under ?? []).filter((e) => !extraEmpty(row, e.id))
    // Its prepended and appended fields sit inline around it (an empty one
    // takes no room); when the cell is short the text alone gives way - the
    // title's track keeps it room enough (compactGrid).
    const extra: CSSProperties = { flex: '0 0 auto' }
    const line = before.length === 0 && after.length === 0
      ? <div style={{ minWidth: 0, display: 'flex' }}>{own(style)}</div>
      : (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
          {before.map((e) => <span key={'b-' + e.id} style={extra}>{extraOf(e, row, index)}</span>)}
          {own({ ...style, flex: '0 1 auto' })}
          {after.map((e) => <span key={'a-' + e.id} style={extra}>{extraOf(e, row, index)}</span>)}
        </div>
      )
    if (under.length === 0) return line
    // Its `below` fields on a second line under it, dimmed and smaller.
    return (
      <div style={{ ...UNDER_CELL, display: 'flex', alignItems: 'stretch', width: '100%' }}>
        {line}
        <div data-ktw-below="" style={UNDER_LINE}>
          {under.map((e) => <span key={'u-' + e.id} style={extra}>{PLAIN.has(e.content) ? String((row as Record<string, unknown>)[e.id]) : e.renderCell(row, index)}</span>)}
        </div>
      </div>
    )
  }
  return <div style={{ minWidth: 0, width: '100%' }}>{column.renderCell(row, index)}</div>
}

/** extraOf draws a prepended or appended field: plain text dimmed, any
 *  other content by its column renderer. */
function extraOf<T>(e: UIColumnExtra<T>, row: T, index: number): ReactNode {
  // An empty field draws nothing - never a dash beside the cell's own value.
  if (extraEmpty(row, e.id)) return null
  if (PLAIN.has(e.content)) {
    const v = (row as Record<string, unknown>)[e.id]
    return <span style={{ fontSize: 12.5, color: color('muted') }}>{String(v)}</span>
  }
  return e.renderCell(row, index)
}

/** extraEmpty is whether a prepended or appended field has nothing to show:
 *  absent, null, an empty text or an empty list. */
export function extraEmpty<T>(row: T, id: string): boolean {
  const v = (row as Record<string, unknown> | undefined)?.[id]
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
}

export function CompactRow<T>({ row, index, columns, grid, selected, clickable, onClick, trailing, last }: {
  row: T
  index: number
  columns: UIColumn<T>[]
  grid: string
  selected: boolean
  clickable: boolean
  onClick: () => void
  trailing?: ReactNode
  last: boolean
}) {
  const [hover, setHover] = useState(false)
  const roles = compactRoles(columns)
  const style: CSSProperties = {
    display: 'grid', gridTemplateColumns: grid, gap: 10, alignItems: 'center',
    padding: '8px 10px', fontSize: 13.5, lineHeight: 1.45,
    borderBottom: last ? 0 : `1px solid ${color('line')}`,
    background: selected || hover ? color('soft') : 'var(--mantine-color-body)',
    boxShadow: selected ? `inset 3px 0 0 ${color('accent')}` : undefined,
    cursor: clickable ? 'pointer' : 'default',
  }
  const onKey = (e: KeyboardEvent) => {
    if (!clickable || (e.key !== 'Enter' && e.key !== ' ')) return
    e.preventDefault()
    onClick()
  }
  return (
    <div role={clickable ? 'button' : 'row'} tabIndex={clickable ? 0 : undefined} aria-pressed={clickable ? selected : undefined}
      data-row-selected={selected ? 'true' : undefined} data-ktw-row=""
      style={style} onClick={clickable ? onClick : undefined} onKeyDown={onKey}
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      {/* A cell clips what its track cannot hold: never over its neighbour. */}
      {columns.map((c, i) => <div key={c.id} style={{ minWidth: 0, overflow: 'hidden', textAlign: c.align ?? 'left' }}>{cellOf(c, roles[i], row, index)}</div>)}
      {trailing !== undefined && trailing !== null && (
        <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end', alignItems: 'center', whiteSpace: 'nowrap' }}
          onClick={(e) => e.stopPropagation()}>
          {trailing}
        </div>
      )}
    </div>
  )
}

/** compactListStyle is the list's frame: a rounded border, no gaps. */
export const compactListStyle: CSSProperties = {
  display: 'flex', flexDirection: 'column', border: `1px solid ${color('line')}`, borderRadius: 8, overflow: 'hidden',
}
