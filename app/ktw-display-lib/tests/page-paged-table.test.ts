// SPDX-License-Identifier: Apache-2.0
// A table that pages from the server: on a progressive page a
// table whose data is {load, pageSize} asks its method a page at a time -
// {limit, offset | cursor} in, {items, page} out - and draws
// the rows loaded, the next page asked at its end or on « Afficher plus ».

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'
import { mergePage, pageRequest, pagedSource } from '../src/ui-builders/page/nodes/paged-load.tsx'
import { showMoreLabel } from '../src/ui-builders/page/nodes/window.tsx'

const src = { load: { method: 'ktw-x:rows/page', args: { module: 'm' } }, pageSize: 20 }

describe('the paging rules', () => {
  test('a paged source is data naming a load method; rows are not one', () => {
    expect(pagedSource(src)).toEqual(src)
    expect(pagedSource([{ a: 1 }])).toBeUndefined()
    expect(pagedSource({ a: 1 })).toBeUndefined()
    expect(pagedSource({ load: { method: '' } })).toBeUndefined()
  })

  test('the next page is asked with its args, a limit and where to start - the cursor when one was given', () => {
    expect(pageRequest(src, 0)).toEqual({ module: 'm', limit: 20, offset: 0 })
    expect(pageRequest(src, 40)).toEqual({ module: 'm', limit: 20, offset: 40 })
    expect(pageRequest(src, 40, '40')).toEqual({ module: 'm', limit: 20, cursor: '40' })
    expect(pageRequest({ load: { method: 'x' } }, 0)).toEqual({ limit: 50, offset: 0 })
  })

  test('each page adds its items; paging ends with the last page', () => {
    const rows = (from: number, n: number) => Array.from({ length: n }, (_, i) => ({ n: from + i }))
    let s = mergePage({ rows: [], done: false }, { items: rows(0, 20), page: { total: 45, hasMore: true, nextCursor: '20' } })
    expect(s.rows.length).toBe(20)
    expect(s.done).toBe(false)
    s = mergePage(s, { items: rows(20, 20), page: { total: 45, hasMore: true, nextCursor: '40' } })
    s = mergePage(s, { items: rows(40, 5), page: { total: 45, hasMore: false } })
    expect(s.rows.length).toBe(45)
    expect(s.done).toBe(true)
    // no hasMore: the total decides; an empty page ends it
    expect(mergePage({ rows: [], done: false }, { items: rows(0, 3), page: { total: 10 } }).done).toBe(false)
    expect(mergePage({ rows: rows(0, 3), done: false }, { items: [], page: { total: 10 } }).done).toBe(true)
  })

  test('« Afficher plus » says how many are left when the total is known', () => {
    expect(showMoreLabel('fr', 25)).toBe('Afficher plus (25)')
    expect(showMoreLabel('fr', undefined)).toBe('Afficher plus')
  })
})

describe('a paged table on a page', () => {
  wireFieldRenderers()
  const schema = { name: 'row', root: 'Row', types: { Row: { type: 'object', fields: { n: { type: 'number' } } } } }
  const node = { kind: 'table', id: 'big', schema, data: src }
  const page = (host: HostServices) => renderToStaticMarkup(
    createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: host },
        createElement(UIPageView, { node: node as never, onAction: (() => {}) as never }))))

  test('pages only on a progressive page whose host can call methods', () => {
    const call = async () => ({ items: [], page: {} })
    expect(page({ progressive: true, callMethod: call })).toContain('data-paged-more="big"')
    expect(page({ callMethod: call })).not.toContain('data-paged-more')
    expect(page({ progressive: true })).not.toContain('data-paged-more')
  })
})
