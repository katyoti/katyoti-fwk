// SPDX-License-Identifier: Apache-2.0
// A page table as the mock-up's « Contenu lu »: its head
// stays in view while the rows scroll (`stickyHeader`, in a box `maxHeight`
// high or in the page); an `icon` field draws its Tabler icon by name - a
// row's open mark prepended to its title; and a row its open action's `when`
// refuses neither opens nor shows a pointer.

import { beforeAll, describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { iconColor } from '../src/ui-builders/fields/icon/renderer.tsx'
import { buildUITable } from '../src/ui-builders/table/ui-table.builder.ts'
import { UITableView, rowClick } from '../src/ui-builders/table/ui-table.view.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { loadTablerIcons } from '../src/ui-core/components/icons/byName.tsx'

type Row = { id: string; title: string; open?: string; doc?: string }

const schema = (table: Record<string, unknown> = {}) => ({
  name: 'read', version: 1, root: 'Row',
  types: {
    Row: {
      type: 'object', label: 'Row', idField: ['id'], titleField: ['title'],
      fields: {
        id: { type: 'string', label: 'Id' },
        title: { type: 'string', label: 'Titre' },
        open: { type: 'string', label: '', optional: true, content: 'icon' },
        doc: { type: 'string', label: 'Doc', optional: true },
      },
      extensions: [{ type: 'ui-table', name: 'default', columns: [{ id: 'title', name: 'title', label: 'Titre', prepend: ['open'] }], ...table }],
    },
  },
})

const rows: Row[] = [
  { id: 'a', title: 'Lu', open: 'external-link', doc: 'm.a' },
  { id: 'b', title: 'Pas lu' },
]

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const view = (table: Record<string, unknown>) => {
  const t = buildUITable<Row>({ schema: schema(table) as never, tableRef: 'default' })
  return html(createElement(UITableView<Row>, { table: t as never, data: rows }))
}
const trs = (h: string) => (h.split('<tbody')[1] ?? '').split('<tr').slice(1)

beforeAll(async () => {
  wireFieldRenderers()
  await loadTablerIcons()
})

describe('a sticky table head', () => {
  test('stickyHeader marks the head sticky, with no scroll box of its own', () => {
    const h = view({ stickyHeader: true })
    expect(h).toMatch(/<thead[^>]*data-sticky="true"/)
    expect(h).not.toContain('mantine-ScrollArea')
  })

  test('with maxHeight the rows scroll under the head in a box that high', () => {
    const h = view({ stickyHeader: true, maxHeight: 320 })
    expect(h).toMatch(/<thead[^>]*data-sticky="true"/)
    expect(h).toContain('data-ktw-table-scroll')
    expect(h).toContain('max-height:calc(20rem')
  })

  test('a table without it keeps a plain head in its sideways scroll box', () => {
    const h = view({})
    expect(h).not.toContain('data-sticky')
    expect(h).toContain('mantine-ScrollArea')
  })
})

describe('an icon cell', () => {
  test('draws the Tabler icon its value names, prepended to the title', () => {
    const [a, b] = trs(view({}))
    expect(a).toContain('tabler-icon-external-link')
    expect(a.indexOf('tabler-icon-external-link')).toBeLessThan(a.indexOf('Lu<'))
    expect(b).not.toContain('tabler-icon')
  })

  test('an unknown name draws a dot; a colorMap colors it', () => {
    const t = buildUITable<Row>({ schema: schema() as never, tableRef: 'default' })
    const h = html(createElement(UITableView<Row>, { table: t as never, data: [{ id: 'x', title: 'X', open: 'no-such-icon' }] }))
    expect(h).toContain('tabler-icon-point-filled')
    expect(iconColor({ type: 'icon', colorMap: '{"m.a":"green"}', colorField: 'doc' }, { doc: 'm.a' }, 'eye')).toBe('var(--mantine-color-green-6)')
    expect(iconColor({ type: 'icon', colorMap: '{"eye":"#123456"}' }, {}, 'eye')).toBe('#123456')
    expect(iconColor({ type: 'icon' }, {}, 'eye')).toBeUndefined()
  })
})

describe("a row's open honours its when", () => {
  const opens = (r: Row) => !!r.doc

  test('a row the when refuses does not open; one it accepts does', () => {
    const on = { open: () => {}, opens }
    expect(rowClick(on, rows[0])).toBe('open')
    expect(rowClick(on, rows[1])).toBeUndefined()
    expect(rowClick({ open: () => {} }, rows[1])).toBe('open')
    expect(rowClick({ select: () => {}, open: () => {}, opens }, rows[1])).toBe('select')
  })

  test("a page table's open with when: doc points only the rows it opens", () => {
    const h = html(createElement(UIPageView, {
      node: { kind: 'table', schema: schema() as never, data: rows, open: { id: 'open', when: 'doc', navigate: { object: '${item.doc}' } } } as never,
      onAction: () => {},
    } as never))
    const [a, b] = trs(h)
    expect(a).toContain('cursor:pointer')
    expect(b).toContain('cursor:default')
  })
})
