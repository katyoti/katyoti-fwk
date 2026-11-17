// SPDX-License-Identifier: Apache-2.0
// A board with `layout: cards` lays its rows out as a grid of
// cards where the host shows the source model (IN-045's switch) - each card
// its identity, state, meta line, the first items it picks (« and N more »),
// a remark saying what to do, « Open… », its actions and the switch -, and an
// opened card draws the panel in place of the grid with a way back. Off, the
// former list beside the panel.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { BoardCardsView } from '../src/ui-builders/page/nodes/board.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { boardCardItems, boardMoreText } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (host: object, node: unknown, data?: unknown) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host as never },
    createElement(UIPageView, { node: node as never, data, onAction: (() => {}) as never }))))

const rows = [
  { key: 'confluence', label: 'Confluence', icon: 'brand-confluence', color: 'blue', state: { text: '2 erreurs', color: 'red' },
    meta: '12 pages lues sur 14 · il y a 1 h', on: true,
    items: ['ERP Arbor (et 6 sous-pages)', 'Arbor — Interfaces facturation', 'Processus de facturation magasins', 'Règles TVA magasins', 'Paramétrage', 'Historique', 'Étude 2025'],
    note: '2 pages refusées : demandez l\'accès ou retirez-les.', noteColor: 'red' },
  { key: 'documents', label: 'Documents', state: 'à jour', meta: '2 fichiers', items: ['Contrat.pdf'], on: false },
]
const board = {
  kind: 'board', id: 'sources-board', title: 'Sources', select: 'connector', dataField: '$.connectors', layout: 'cards',
  openLabel: 'Choisir…', backLabel: '← Toutes les sources', moreLabel: 'et {n} autres',
  toggle: { uri: 'ktw-agents:sources/use', args: { connector: '${item.key}', on: '${value}' } },
  actions: [
    { label: 'Actualiser', uri: 'ktw-agents:sources/source-sync', args: { id: '${item.source}' } },
    { label: 'Retirer', uri: 'ktw-agents:sources/remove', args: { id: '${item.source}' }, dangerous: true },
  ],
  panel: { kind: 'text', content: 'the panel of the opened source' },
}
const call = async () => ({})

describe('a card lists its items', () => {
  test('the first five, then how many it leaves out', () => {
    expect(boardCardItems(rows[0].items)).toEqual({ shown: rows[0].items.slice(0, 5), more: 2 })
    expect(boardCardItems(['a', '', 3, 'b'], 1)).toEqual({ shown: ['a'], more: 1 })
    expect(boardCardItems(undefined)).toEqual({ shown: [], more: 0 })
    expect(boardMoreText(2, 'et {n} autres')).toBe('et 2 autres')
    expect(boardMoreText(3)).toBe('and 3 more')
  })
})

describe('a cards board', () => {
  test('draws one card per row where the host shows the source model', () => {
    const html = page({ callMethod: call, sourceModel: true }, board, { connectors: rows })
    expect((html.match(/data-guide="board.card"/g) ?? []).length).toBe(2)
    for (const want of ['Confluence', '2 erreurs', '12 pages lues sur 14 · il y a 1 h', 'ERP Arbor (et 6 sous-pages)', 'Paramétrage',
      'et 2 autres', '2 pages refusées', 'Documents', 'à jour', 'Contrat.pdf', 'Choisir…', 'Actualiser', 'Retirer']) {
      expect(html).toContain(want)
    }
    // Five items listed of seven: the sixth is left out.
    expect(html).not.toContain('Historique')
    expect((html.match(/data-guide="board.open"/g) ?? []).length).toBe(2)
    expect((html.match(/data-guide-label="Actualiser"/g) ?? []).length).toBe(2)
    expect((html.match(/type="checkbox"/g) ?? []).length).toBe(2)
    expect((html.match(/data-guide="board.card-note"/g) ?? []).length).toBe(1)
    // The grid holds the cards; the panel waits for a card to open.
    expect(html).not.toContain('the panel of the opened source')
    expect(html).not.toContain('data-guide="board.row"')
  })

  test('an opened card draws the panel in place of the grid, with the way back', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(BoardCardsView, {
      node: { ...board, data: rows } as never, render: (n: { content?: string }) => createElement('p', null, n.content),
      opened: 'confluence', onOpen: () => {}, onBack: () => {},
    })))
    expect(html).toContain('the panel of the opened source')
    expect(html).toContain('← Toutes les sources')
    expect(html).toContain('data-opened="confluence"')
    expect(html).not.toContain('data-guide="board.card"')
  })

  test('an opened key no row holds keeps the grid', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(BoardCardsView, {
      node: { ...board, data: rows } as never, render: () => null, opened: 'gone', onOpen: () => {}, onBack: () => {},
    })))
    expect((html.match(/data-guide="board.card"/g) ?? []).length).toBe(2)
  })

  test('off, the former list beside the panel', () => {
    const html = page({ callMethod: call }, board, { connectors: rows })
    expect(html).not.toContain('data-guide="board.card"')
    expect((html.match(/data-guide="board.row"/g) ?? []).length).toBe(2)
    expect(html).toContain('the panel of the opened source')
  })

  // A row's own label on its open button - « Déposer… » on
  // Documents, the board's « Choisir… » on the others.
  test('a row may name its own open button', () => {
    const own = [rows[0], { ...rows[1], openLabel: 'Déposer…' }]
    const html = page({ callMethod: call, sourceModel: true }, board, { connectors: own })
    expect((html.match(/Choisir…/g) ?? []).length).toBe(1)
    expect((html.match(/Déposer…/g) ?? []).length).toBe(1)
    expect(html.indexOf('Choisir…')).toBeLessThan(html.indexOf('Déposer…'))
  })

  test('is in the page grammar', () => {
    const def = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, { enum?: string[] }> }> }).definitions.Board
    expect(def.properties.layout.enum).toEqual(['list', 'cards'])
    for (const f of ['itemsMax', 'moreLabel', 'openLabel', 'backLabel', 'actions']) expect(def.properties[f]).toBeDefined()
  })
})
