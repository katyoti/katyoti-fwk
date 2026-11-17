// SPDX-License-Identifier: Apache-2.0
// A ui-table column draws named fields on a second line under its own value
// (`below`), dimmed and smaller: a row reads on two lines - its title, then
// its id and release under it. An empty field takes no room, a column
// without `below` stays one line; normal and compact tables alike.

import { beforeAll, describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUITable } from '../src/ui-builders/table/ui-table.builder.ts'
import { UITableView } from '../src/ui-builders/table/ui-table.view.tsx'

type Row = { id: string; title: string; release?: string }

const schema = (column: Record<string, unknown>, table: Record<string, unknown> = {}) => ({
  name: 'rows', version: 1, root: 'Row',
  types: {
    Row: {
      type: 'object', label: 'Row', idField: ['id'], titleField: ['title'],
      fields: {
        id: { type: 'string', label: 'Id' },
        title: { type: 'string', label: 'Titre' },
        release: { type: 'string', label: 'Version', optional: true },
      },
      extensions: [{ type: 'ui-table', name: 'default', columns: [{ id: 'title', name: 'title', label: 'Titre', ...column }], ...table }],
    },
  },
})

const rows: Row[] = [
  { id: 'm.F-001', title: 'Premier', release: 'v1' },
  { id: 'm.F-002', title: 'Second' },
]

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const view = (column: Record<string, unknown>, table: Record<string, unknown> = {}) => {
  const t = buildUITable<Row>({ schema: schema(column, table) as never, tableRef: 'default' })
  return html(createElement(UITableView<Row>, { table: t as never, data: rows }))
}
/** The second line of each row that has one, in order. */
const belowLines = (h: string) => h.split('data-ktw-below=""').slice(1).map((p) => {
  // The text of the second-line element: up to the tag that closes it.
  const tags = /<(\/?)[a-z][^>]*?(\/?)>/g
  const start = p.indexOf('>') + 1
  tags.lastIndex = start
  let depth = 1
  let end = p.length
  for (let m = tags.exec(p); m; m = tags.exec(p)) {
    if (m[2]) continue
    depth += m[1] ? -1 : 1
    if (depth === 0) { end = m.index; break }
  }
  return p.slice(start, end).replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
})

beforeAll(() => {
  wireFieldRenderers()
})

for (const density of ['normal', 'compact'] as const) {
  describe(`a ${density} table with below`, () => {
    test('draws the id and the release on a second line under the title', () => {
      const h = view({ below: ['id', 'release'] }, { density })
      const lines = belowLines(h)
      expect(lines[0]).toStartWith('m.F-001')
      expect(lines[0]).toContain('v1')
      expect(lines[0]).not.toContain('Premier')
      expect(h.indexOf('Premier')).toBeLessThan(h.indexOf('data-ktw-below'))
    })

    test('an empty field takes no room', () => {
      const lines = belowLines(view({ below: ['id', 'release'] }, { density }))
      expect(lines).toHaveLength(2)
      expect(lines[1]).toContain('m.F-002')
      expect(lines[1]).not.toContain('v1')
    })

    test('a row whose below fields are all empty stays one line', () => {
      const h = view({ below: ['release'] }, { density })
      expect(belowLines(h)).toHaveLength(1)
    })

    test('a column without below stays one line', () => {
      const h = view({}, { density })
      expect(h).not.toContain('data-ktw-below')
      expect(h).toContain('Premier')
    })
  })
}
