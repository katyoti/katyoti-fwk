// SPDX-License-Identifier: Apache-2.0
// A board: a list of items beside the selected item's panel -
// each row its icon, label, count, chips and a switch, a count in the head, a
// click selecting the row (the first while none is), the panel drawn beside.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { childrenOf } from '../src/ui-builders/page/ui-page.types.ts'
import { boardRows, boardSelected, boardToggleEvent } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data, onAction: (() => {}) as never })))

const rows = [
  { key: 'leanix', label: 'LeanIX', icon: 'building', count: 3, chips: ['joignable', { text: '2 en attente', color: 'yellow' }], on: true },
  { key: 'confluence', label: 'Confluence', count: '12 pages', on: false },
]
const use = { uri: 'ktw-agents:sources/use', args: { connector: '${item.key}', on: '${value}' } }
const board = {
  kind: 'board', id: 'sources', title: 'Sources', select: 'connector', dataField: '$.connectors', toggle: use,
  panel: { kind: 'text', content: 'the panel of the selected source' },
}

describe('the board rows and selection', () => {
  test('keep the rows with a key; select the selection value it holds, else the first', () => {
    expect(boardRows([...rows, { label: 'no key' }, null, 'x'])).toHaveLength(2)
    expect(boardRows(undefined)).toEqual([])
    expect(boardSelected(rows, { connector: 'confluence' }, 'connector')).toBe('confluence')
    expect(boardSelected(rows, {}, 'connector')).toBe('leanix')
    expect(boardSelected(rows, { connector: 'gone' }, 'connector')).toBe('leanix')
    expect(boardSelected([], {}, 'connector')).toBeUndefined()
  })

  test('flipping a switch sends the toggle action on the row with the new state', () => {
    expect(boardToggleEvent(use, rows[0])).toEqual({ action: use, item: rows[0], extra: { value: 'false' } })
    expect(boardToggleEvent(use, rows[1]).extra).toEqual({ value: 'true' })
  })
})

describe('a board draws', () => {
  test('each row with its label, count and chips, the head count, a switch and the panel beside', () => {
    const html = page(board, { connectors: rows })
    for (const want of ['LeanIX', 'Confluence', '12 pages', 'joignable', '2 en attente', 'Sources', 'the panel of the selected source']) {
      expect(html).toContain(want)
    }
    expect(html).toContain('data-element="board:sources"')
    expect((html.match(/data-guide="board.row"/g) ?? []).length).toBe(2)
    expect((html.match(/type="checkbox"/g) ?? []).length).toBe(2)
    // The first row is the one selected while none is.
    expect(html).toMatch(/aria-current="true"[^>]*data-guide-name="leanix"/)
  })

  test('no switch without toggle, its empty text without rows and no panel', () => {
    const plain = page({ ...board, toggle: undefined }, { connectors: rows })
    expect(plain).not.toContain('type="checkbox"')
    const none = page({ ...board, empty: 'Aucune source.' }, { connectors: [] })
    expect(none).toContain('Aucune source.')
    expect(none).not.toContain('the panel of the selected source')
  })

  test('binds its rows, holds its panel as a child, and is in the page grammar', () => {
    expect((applyDataBinding(board as never, { connectors: rows }) as { data: unknown[] }).data).toHaveLength(2)
    expect(childrenOf(board as never)).toEqual([board.panel])
    const kinds = (UIPageNodeJSONSchema as { oneOf: Array<{ $ref: string }> }).oneOf.map((r) => r.$ref)
    expect(kinds).toContain('#/definitions/Board')
  })
})
