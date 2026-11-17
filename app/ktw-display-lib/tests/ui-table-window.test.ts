// SPDX-License-Identifier: Apache-2.0
// A table draws only what is seen (the window of a progressive page):
// its first pageSize rows and « Afficher plus » adding the next page, on every
// host - a chat artifact or a hostless panel too - with no in-memory pager
// (no Mantine Pagination, no page-size picker); filter and sort act over every
// row before the window takes its pages; pagination false draws every row (a
// server-paged table loads its own pages).

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUITable } from '../src/ui-builders/table/ui-table.builder.ts'
import { UITableView, allRows } from '../src/ui-builders/table/ui-table.view.tsx'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'
import { windowSize } from '../src/ui-builders/page/nodes/window.tsx'

type Row = { id: string; n: number }

const schema = {
  name: 'rows', version: 1, root: 'Rows',
  types: {
    Rows: { type: 'array', items: 'Row' },
    Row: { type: 'object', label: 'Row', idField: ['id'], titleField: ['id'], fields: { id: { type: 'string', label: 'Id' }, n: { type: 'number', label: 'N' } } },
  },
}

const rows: Row[] = Array.from({ length: 25 }, (_, i) => ({ id: `r${i}`, n: i }))

const render = (extra: Record<string, unknown>, host?: HostServices) => {
  wireFieldRenderers()
  const table = { ...buildUITable<Row>({ schema: schema as never }), ...extra }
  const view = createElement(UITableView<Row>, { table: table as never, data: rows })
  return renderToStaticMarkup(createElement(MantineProvider, null,
    host ? createElement(HostServicesContext.Provider, { value: host }, view) : view))
}

const bodyRows = (html: string) => (html.split('<tbody')[1] ?? '').split('<tr').length - 1

describe('a table draws its first page and « Afficher plus »', () => {
  test('hostless: the first 10 rows and a button for the 15 left, no pager', () => {
    const html = render({})
    expect(bodyRows(html)).toBe(10)
    expect(html).toContain('data-show-more=')
    expect(html).toMatch(/Show more \(15\)|Afficher plus \(15\)/)
    expect(html).not.toContain('mantine-Pagination')
    expect(html).not.toContain('mantine-Select')
  })

  test('the same on a progressive host, with the page size the table names', () => {
    const html = render({ pageSize: 20 }, { progressive: true })
    expect(bodyRows(html)).toBe(20)
    expect(html).toMatch(/Show more \(5\)|Afficher plus \(5\)/)
    expect(html).not.toContain('mantine-Pagination')
  })

  test('a table that holds no more than its page draws no button', () => {
    const html = render({ pageSize: 50 })
    expect(bodyRows(html)).toBe(25)
    expect(html).not.toContain('data-show-more')
  })

  test('pagination false draws every row, no button', () => {
    const html = render({ pagination: false })
    expect(bodyRows(html)).toBe(25)
    expect(html).not.toContain('data-show-more')
    expect(html).not.toContain('mantine-Pagination')
  })
})

describe('the window takes the pages of every row, filtered and sorted', () => {
  test('« Afficher plus » adds the next page: 10, 20, then all 25', () => {
    expect(windowSize(25, true, 10, 1)).toBe(10)
    expect(windowSize(25, true, 10, 2)).toBe(20)
    expect(windowSize(25, true, 10, 3)).toBe(25)
  })

  test('sort acts over every row: the first page sorted descending starts at the last row', () => {
    const desc = (a: Row, b: Row) => b.n - a.n
    const all = allRows(rows, undefined, desc)
    expect(all.length).toBe(25)
    expect(all.slice(0, windowSize(all.length, true, 10, 1)).map((r) => r.n)).toEqual([24, 23, 22, 21, 20, 19, 18, 17, 16, 15])
  })

  test('filter acts over every row: matches beyond the first page are found', () => {
    const odd = (rs: Row[]) => rs.filter((r) => r.n % 2 === 1 && r.n > 12)
    const all = allRows(rows, odd)
    expect(all.map((r) => r.n)).toEqual([13, 15, 17, 19, 21, 23])
    expect(rows).toHaveLength(25)
  })
})
