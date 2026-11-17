// SPDX-License-Identifier: Apache-2.0
// Page filters: chips with counts, a search box and a switch
// each set a key of the page selection; a node's `filter` keeps the rows
// passing it - « All streams » / a stream / « No stream », a search over the
// title and the id, held rows shown only while the switch is on.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { chipCount, chipOptions, filterRows, passes, withFilter } from '../src/ui-builders/page/nodes/filters.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { ALL_KINDS } from '../src/ui-builders/page/ui-page.types.ts'

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))

const rows = [
  { id: 'IN-001', title: 'Sources and search', stream: 'SM-002', held: '' },
  { id: 'IN-005', title: 'Dossier readiness', stream: 'SM-002', held: 'waits for the template' },
  { id: 'IN-043', title: 'See where everything stands', stream: 'SM-001', held: '' },
  { id: 'IN-090', title: 'Loose idea', stream: '', held: '' },
]
const byStream = { key: 'stream', field: 'stream', empty: 'none' }
const bySearch = { key: 'q', fields: ['title', 'id'], match: 'contains' as const }
const byHeld = { key: 'show_held', field: 'held', match: 'unless' as const }

describe('a filter clause', () => {
  test('equals keeps the rows of the key value; unset keeps all; its empty value keeps the empty rows', () => {
    expect(filterRows(rows, [byStream], {}).length).toBe(4)
    expect(filterRows(rows, [byStream], { stream: '' }).length).toBe(4)
    expect(filterRows(rows, [byStream], { stream: 'SM-002' }).map((r) => r.id)).toEqual(['IN-001', 'IN-005'])
    expect(filterRows(rows, [byStream], { stream: 'none' }).map((r) => r.id)).toEqual(['IN-090'])
  })

  test('contains looks in every field, ignoring case', () => {
    expect(filterRows(rows, [bySearch], { q: 'DOSSIER' }).map((r) => r.id)).toEqual(['IN-005'])
    expect(filterRows(rows, [bySearch], { q: 'in-04' }).map((r) => r.id)).toEqual(['IN-043'])
    expect(filterRows(rows, [bySearch], { q: '  ' }).length).toBe(4)
  })

  test('unless hides a flagged row while the key is off', () => {
    expect(filterRows(rows, [byHeld], {}).map((r) => r.id)).not.toContain('IN-005')
    expect(filterRows(rows, [byHeld], { show_held: true }).map((r) => r.id)).toContain('IN-005')
    expect(passes(rows[0], byHeld, {})).toBe(true)
  })

  test('every clause must pass; a node without rows is left as it is', () => {
    expect(filterRows(rows, [byStream, bySearch], { stream: 'SM-002', q: 'sources' }).map((r) => r.id)).toEqual(['IN-001'])
    const n = { kind: 'text', content: 'x', filter: [byStream] }
    expect(withFilter(n as never, { stream: 'SM-001' })).toBe(n as never)
    expect((withFilter({ kind: 'list', data: rows, filter: [byStream] } as never, { stream: 'SM-001' }) as { data: unknown[] }).data.length).toBe(1)
  })
})

describe('the chips', () => {
  const node = { kind: 'chips' as const, key: 'stream', all: 'All streams', empty: 'none', countField: 'stream', countRows: rows,
    options: [{ value: 'SM-001', label: 'KaTyoti' }, { value: 'SM-002', label: 'ArchiFactory' }, { value: 'none', label: 'No stream' }] }

  test('the all chip first, each chip counting its rows unless it gives its count', () => {
    expect(chipOptions(node).map((o) => o.value)).toEqual(['', 'SM-001', 'SM-002', 'none'])
    expect(chipOptions(node).map((o) => chipCount(o, node))).toEqual([4, 1, 2, 1])
    expect(chipCount({ value: 'SM-001', count: 9 }, node)).toBe(9)
    expect(chipCount({ value: 'SM-001' }, { countField: undefined })).toBeUndefined()
  })

  test('drawn as pressed buttons, the current value pressed, with their counts', () => {
    const out = html(createElement(UIPageView, { node: node as never, data: { selection: { stream: 'SM-002' } } }))
    expect(out).toContain('data-ktw-chips="stream"')
    expect(out).toMatch(/aria-pressed="true"[^>]*data-value="SM-002"/)
    expect(out).toMatch(/aria-pressed="false"[^>]*data-value=""/)
    expect(out).toContain('All streams')
    expect(out).toContain('No stream')
  })

  test('a search box and a switch draw on their keys', () => {
    const out = html(createElement(UIPageView, {
      node: { kind: 'hbox', children: [{ kind: 'search', key: 'q', placeholder: 'Find an increment' }, { kind: 'switch', key: 'show_held', label: 'Show held increments' }] } as never,
      data: { selection: { show_held: true } },
    }))
    expect(out).toContain('data-ktw-search="q"')
    expect(out).toContain('placeholder="Find an increment"')
    expect(out).toContain('Show held increments')
    expect(out).toMatch(/checked=""[^>]*data-ktw-switch="show_held"|data-ktw-switch="show_held"[^>]*checked=""/)
  })

  test('chips, search and switch are page kinds', () => {
    for (const k of ['chips', 'search', 'switch']) expect(ALL_KINDS).toContain(k as never)
  })
})

describe('a node reading the selection', () => {
  test('a list draws only the rows its filter keeps for the page selection', () => {
    const page = (selection: Record<string, unknown>) => html(createElement(UIPageView, {
      node: { kind: 'list', data: rows, filter: [byStream, bySearch, byHeld], item: { kind: 'text', dataField: '$.item.id' } } as never,
      data: { selection },
    }))
    const shown = (out: string) => ['IN-001', 'IN-005', 'IN-043', 'IN-090'].filter((id) => out.includes(id))
    expect(shown(page({}))).toEqual(['IN-001', 'IN-043', 'IN-090'])
    expect(shown(page({ stream: 'SM-002', show_held: true }))).toEqual(['IN-001', 'IN-005'])
    expect(shown(page({ q: 'loose' }))).toEqual(['IN-090'])
  })
})
