// SPDX-License-Identifier: Apache-2.0
// An opened item shows what its source's fetch answers
// - its card at the summary size, its link, its data drawn by
// its data-type's view, its markdown text with its images, the objects it is
// used by - where the host shows the source model (IN-045's switch); off,
// nothing. One item brings data and text (a LeanIX application), one text
// only (a Confluence page).

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { ItemDetailView, itemFetchArgs } from '../src/ui-builders/page/nodes/item-detail.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { ITEM_DETAIL_LABELS, ITEM_DETAIL_RESOLVE, itemDataRows, itemDetailParts, type UIItemOpened } from '../src/ui-builders/page/ui-page.interactive.ts'

wireFieldRenderers()

/** A LeanIX application: data typed by its kind's data-type, and text. */
const app: UIItemOpened = {
  item: { key: 'app-arbor', title: 'ERP Arbor', kind: 'application', path: ['Applications', 'ERP'], detail: 'ERP des magasins',
    attrs: { code: 'RDY' }, modified: '2026-10-30T10:00:00Z', uri: 'https://leanix.example/factsheet/app-arbor' },
  data: { name: 'ERP Arbor', code: 'RDY', lifecycle: 'active' },
  data_type: 'ktw-example:/data-type/leanix-application',
  text: '# ERP Arbor\n\nLe **ERP** des magasins.\n\n![Schéma](https://leanix.example/img/arbor.png)',
  used: [{ id: 'acme-da.ST-001', name: 'Étude Arbor', type: 'as-is-study' }],
}

/** A Confluence page: text only, its URI the source's own. */
const page: UIItemOpened = {
  item: { key: '4242', title: 'Règles TVA magasins', kind: 'page', uri: 'confluence://wiki.example/pages/4242' },
  text: 'Les règles de **TVA** en magasin.\n\n![Barème](https://wiki.example/download/bareme.png)',
}

/** The view the data-type's view resolves to. */
const view = { name: 'leanix-application', root: 'App', types: { App: { type: 'object', fields: {
  name: { type: 'string', label: 'Nom' }, code: { type: 'string', label: 'Code applicatif' }, lifecycle: { type: 'string', label: 'Cycle de vie' } } } } }

const wrap = (host: object, child: unknown) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host as never }, child as never)))
const onPage = (host: object, node: unknown, data?: unknown) => wrap(host, createElement(UIPageView, { node: node as never, data, onAction: (() => {}) as never }))
const detail = (opened: UIItemOpened, more: Record<string, unknown> = {}) => wrap({ sourceModel: true, navigate: () => {} },
  createElement(ItemDetailView as never, { node: { id: 'item' }, opened, render: renderVia, ...more }))

// ItemDetailView draws its view and markdown through the page dispatcher:
// a one-node page around each part.
function renderVia(node: unknown, key?: string | number) {
  return createElement(UIPageView, { key, node: node as never, onAction: (() => {}) as never })
}

describe('what an item\'s detail draws (itemDetailParts)', () => {
  test('data and text: its card, its web link, its data and its data-type, its text, where it is used', () => {
    const p = itemDetailParts(app)
    expect(p.item?.title).toBe('ERP Arbor')
    expect(p.href).toBe('https://leanix.example/factsheet/app-arbor')
    expect(p.uri).toBeUndefined()
    expect(p.data).toEqual(app.data!)
    expect(p.dataType).toBe('ktw-example:/data-type/leanix-application')
    expect(p.dataRows).toEqual([{ field: 'name', value: 'ERP Arbor' }, { field: 'code', value: 'RDY' }, { field: 'lifecycle', value: 'active' }])
    expect(p.text).toContain('Le **ERP** des magasins')
    expect(p.used).toEqual([{ id: 'acme-da.ST-001', name: 'Étude Arbor', type: 'as-is-study' }])
    expect(p.empty).toBe(false)
  })

  test('text only: no data, its source URI shown as text, no used list when the producer says none', () => {
    const p = itemDetailParts(page)
    expect(p.data).toBeUndefined()
    expect(p.dataType).toBeUndefined()
    expect(p.dataRows).toEqual([])
    expect(p.href).toBeUndefined()
    expect(p.uri).toBe('confluence://wiki.example/pages/4242')
    expect(p.text).toContain('**TVA**')
    expect(p.used).toBeUndefined()
    expect(p.empty).toBe(false)
  })

  test('an item bringing neither is empty; a data-type without data is none', () => {
    expect(itemDetailParts({ item: { key: 'k', title: 'K' }, data: {}, data_type: 'x:/data-type/y', text: '  ' })).toMatchObject({ empty: true, dataRows: [] })
    expect(itemDetailParts({ item: { key: 'k', title: 'K' }, data: {}, data_type: 'x:/data-type/y' }).dataType).toBeUndefined()
    expect(itemDetailParts(undefined).empty).toBe(true)
    expect(itemDataRows({ a: 1, b: null, c: '', d: { e: 2 } })).toEqual([{ field: 'a', value: '1' }, { field: 'd', value: '{"e":2}' }])
  })
})

describe('an opened item drawn', () => {
  test('data + text: the card, the link, the data by its data-type\'s view, the markdown with its image, where it is used', () => {
    const html = detail(app, { view })
    expect(html).toContain('data-guide="item-detail"')
    expect(html).toContain('data-guide="item-detail.item-card"')
    expect(html).toContain('ERP Arbor')
    expect(html).toContain('ERP des magasins')
    expect(html).toMatch(/data-guide="item-detail.link"[^>]*>Ouvrir à la source</)
    expect(html).toContain('href="https://leanix.example/factsheet/app-arbor"')
    // The data drawn by the resolved view: its labels, not raw field names.
    expect(html).toContain('data-type="ktw-example:/data-type/leanix-application"')
    for (const want of ['Nom', 'Code applicatif', 'Cycle de vie', 'RDY', 'active']) expect(html).toContain(want)
    expect(html).not.toContain('data-guide="item-detail.fields"')
    // The text as markdown, its image drawn.
    expect(html).toMatch(/<h1[^>]*>ERP Arbor<\/h1>/)
    expect(html).toContain('<strong>ERP</strong>')
    expect(html).toContain('<img src="https://leanix.example/img/arbor.png" alt="Schéma"')
    expect(html).toMatch(/data-guide="item-detail.use"[^>]*data-guide-name="acme-da.ST-001"[^>]*>Étude Arbor</)
  })

  test('data before its view is resolved: field by field', () => {
    const html = detail(app)
    expect(html).toContain('data-guide="item-detail.fields"')
    expect(html).toContain('lifecycle')
    expect(html).not.toContain('Code applicatif')
  })

  test('text only: no data part, the source URI as text, the markdown with its image', () => {
    const html = detail(page)
    expect(html).not.toContain('data-guide="item-detail.data"')
    expect(html).not.toContain('data-guide="item-detail.link"')
    expect(html).toMatch(/data-guide="item-detail.uri"[^>]*>confluence:\/\/wiki.example\/pages\/4242</)
    expect(html).toContain('<strong>TVA</strong>')
    expect(html).toContain('<img src="https://wiki.example/download/bareme.png" alt="Barème"')
    expect(html).not.toContain('data-guide="item-detail.used"')
  })

  test('an item used by nothing says so', () => {
    const html = detail({ ...page, used: [] })
    expect(html).toContain(ITEM_DETAIL_LABELS.unused)
  })
})

describe('the item-detail node on a page', () => {
  const node = { kind: 'item-detail', id: 'item', dataField: '$.opened' }

  test('bound to an opened item, drawn where the host shows the source model', () => {
    const html = onPage({ sourceModel: true, callMethod: async () => ({}) }, node, { opened: page })
    expect(html).toContain('data-guide="item-detail"')
    expect(html).toContain('Règles TVA magasins')
    expect(html).toContain('<strong>TVA</strong>')
  })

  test('off the source model, nothing: the former display shows', () => {
    const html = onPage({ callMethod: async () => ({}) }, node, { opened: page })
    expect(html).not.toContain('item-detail')
    expect(html).not.toContain('Règles TVA magasins')
  })

  test('with fetch, it opens the item itself: loading until its answer', () => {
    const html = onPage({ sourceModel: true, callMethod: async () => page },
      { kind: 'item-detail', id: 'item', fetch: { method: 'ktw-agents:sources/source-fetch', args: { id: '${source}', key: '${key}' } } },
      { source: 'acme-da.SRC-001', key: '4242' })
    expect(html).toContain('data-state="loading"')
    expect(html).toContain('Ouverture de l')
  })

  test('its fetch args are read in the page data, then in the pages around it', () => {
    const fetch = { method: 'ktw-agents:sources/source-fetch', args: { id: '${source}', key: '${item.key}', of: '${study}' } }
    expect(itemFetchArgs({ fetch: { ...fetch, args: { id: '${source}', key: '${key}' } } }, { source: 'acme-da.SRC-001', key: '4242' }))
      .toEqual({ id: 'acme-da.SRC-001', key: '4242' })
    expect(itemFetchArgs({ fetch: { method: fetch.method, args: { id: '${source}', of: '${study}' } } }, { source: 'S' }, [{ study: 'acme-da.ST-001' }]))
      .toEqual({ id: 'S', of: 'acme-da.ST-001' })
    expect(itemFetchArgs({}, { source: 'S' })).toBeUndefined()
  })

  test('the DSL declares it: an interactive kind with its schema', () => {
    expect(INTERACTIVE_KINDS as readonly string[]).toContain('item-detail')
    const d = (UIPageNodeJSONSchema as any).definitions.ItemDetail
    expect(d.properties.kind.const).toBe('item-detail')
    expect(d.properties.fetch.required).toEqual(['method'])
    expect(ITEM_DETAIL_RESOLVE).toBe('ktw-agents:object-pane/resolve-part')
  })
})

// A fetched page's pictures come inlined as data:image
// URIs - its markdown draws them, where the default URL transform emptied
// them (an img with no src); any other data: address is still dropped.
describe('an inlined image in an opened item text', () => {
  test('keeps its data:image address', async () => {
    const { MarkdownRenderer } = await import('../src/ui-builders/page/nodes/atomic.tsx')
    const png = 'data:image/png;base64,iVBORw0KGgo='
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(MarkdownRenderer, { node: { kind: 'markdown', content: `![Schéma](${png})\n\n[lien](data:text/html;base64,PHA+)` } as any })))
    expect(html).toContain(`src="${png}"`)
    expect(html).not.toContain('data:text/html')
  })
})
