// SPDX-License-Identifier: Apache-2.0
// A grid board: cards in cells of rows by one field and
// columns by another, counts on both; a move to another column of its row
// runs the action with the target cell, a prompted column asks its text
// first, a drop in another row is refused, Alt+Left/Right picks the column
// beside.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { gridCell, gridCounts, gridKeyTarget, gridMove, type UIGridBoardNode } from '../src/ui-builders/page/ui-page.interactive.ts'

const assess = { uri: 'ktw-agents:requests/assess', args: { id: '${item.id}', priority: '${column}' } }
const hold = { uri: 'ktw-agents:increments/hold', args: { id: '${item.id}', reason: '${value}' } }
const cards = [
  { id: 'IN-1', title: 'Sources everywhere', stream: 'SM-001', priority: 'P0' },
  { id: 'IN-2', title: 'Project groups', stream: 'SM-001', priority: 'P2' },
  { id: 'IN-3', title: 'Study in three minutes', stream: 'SM-002', priority: 'P2' },
]
const node: UIGridBoardNode = {
  kind: 'grid-board', id: 'open', dataField: '$.cards', rowField: 'stream', columnField: 'priority',
  rows: [{ id: 'SM-001', title: 'KaTyoti', meta: 'periodic every 14d' }, { id: 'SM-002', title: 'ArchiFactory' }],
  columns: [{ id: 'P0', title: 'P0' }, { id: 'P1', title: 'P1' }, { id: 'P2', title: 'P2' }, { id: 'held', title: 'On hold', prompt: 'Why is it on hold?', move: hold }],
  move: assess, crossRow: 'An increment stays in its stream.', data: cards,
}

describe('the grid board rules', () => {
  test('a cell holds the cards of its row and column; counts per row and column', () => {
    expect(gridCell(node, 'SM-001', 'P2').map((c) => c.id)).toEqual(['IN-2'])
    expect(gridCell(node, 'SM-002', 'P0')).toEqual([])
    expect(gridCounts(node)).toEqual({ rows: { 'SM-001': 2, 'SM-002': 1 }, columns: { P0: 1, P2: 2 } })
  })

  test('a move within the row runs with the target cell; another row is refused; the same cell does nothing', () => {
    expect(gridMove(node, cards[1], 'SM-001', 'P1')).toEqual({ run: { action: assess, item: cards[1], extra: { row: 'SM-001', column: 'P1' } } })
    expect(gridMove(node, cards[1], 'SM-002', 'P1')).toEqual({ refused: 'An increment stays in its stream.' })
    expect(gridMove(node, cards[1], 'SM-001', 'P2')).toEqual({ none: true })
  })

  test('a prompted column asks its question first, then runs its own move with the answer; an empty answer runs nothing', () => {
    expect(gridMove(node, cards[0], 'SM-001', 'held')).toEqual({ prompt: 'Why is it on hold?', column: 'held' })
    expect(gridMove(node, cards[0], 'SM-001', 'held', ' waiting for IN-036 ')).toEqual({ run: { action: hold, item: cards[0], extra: { row: 'SM-001', column: 'held', value: 'waiting for IN-036' } } })
    expect(gridMove(node, cards[0], 'SM-001', 'held', '  ')).toEqual({ none: true })
  })

  test('Alt+Left / Alt+Right pick the column beside, nothing past an edge', () => {
    expect(gridKeyTarget(node, cards[1], -1)).toBe('P1')
    expect(gridKeyTarget(node, cards[1], 1)).toBe('held')
    expect(gridKeyTarget(node, cards[0], -1)).toBeUndefined()
  })
})

describe('a grid board draws', () => {
  const page = (n: unknown, data?: unknown) => renderToStaticMarkup(
    createElement(MantineProvider, null, createElement(UIPageView, { node: n as never, data, onAction: (() => {}) as never })))

  test('its rows with their meta, its columns, a cell per row and column, the cards in their cells', () => {
    const html = page({ ...node, data: undefined }, { cards })
    for (const want of ['KaTyoti', 'periodic every 14d', 'ArchiFactory', 'On hold', 'Sources everywhere', 'Project groups']) expect(html).toContain(want)
    expect((html.match(/data-guide="grid.cell"/g) ?? []).length).toBe(8)
    expect((html.match(/data-guide="grid.card"/g) ?? []).length).toBe(3)
    expect(html).toContain('data-element="grid-board:open"')
    expect(html).toMatch(/data-guide-name="SM-001\|P2"[\s\S]*Project groups/)
  })

  test('binds its cards, holds its card template as a child, and is in the page grammar', () => {
    expect((applyDataBinding({ ...node, data: undefined } as never, { cards }) as { data: unknown[] }).data).toHaveLength(3)
    const kinds = (UIPageNodeJSONSchema as { oneOf: Array<{ $ref: string }> }).oneOf.map((r) => r.$ref)
    expect(kinds).toContain('#/definitions/GridBoard')
  })
})
