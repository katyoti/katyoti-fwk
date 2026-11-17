// SPDX-License-Identifier: Apache-2.0
// An item is drawn from its source's roles in one item card
// at sizes - a search row, a browse row, a selection row, a summary - with
// optional slots per item kind declared as data. The picker's rows and the
// multi-source search's items draw through it.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { ITEM_CARD_ROLES, SourceItem, itemCardParts, modifiedDay } from '../src/ui-builders/page/nodes/source-item.tsx'
import { SOURCE_SEARCH_LABELS, SourceGroupView } from '../src/ui-builders/page/nodes/source-search.tsx'
import type { UIItemCardSize, UIItemKinds, UIPickerItem } from '../src/ui-builders/page/ui-page.interactive.ts'

/** A LeanIX application as the sources' roles carry it. */
const arbor: UIPickerItem = {
  key: 'app-arbor', title: 'Arbor', kind: 'application',
  path: ['Logistique', 'Transport'], detail: 'Planification des tournées',
  why: '« Arbor » dans le nom', badges: ['ia'], attrs: { code: 'RDY', owner: 'DSI' },
  modified: '2026-10-03T08:12:00Z',
}

const kinds: UIItemKinds = {
  application: { label: 'Application', slots: [{ attr: 'code', label: 'Code' }, { attr: 'owner', label: 'Responsable', sizes: ['summary'] }] },
}

const badges = { ia: { label: 'IA', color: 'violet' } }

const sizes: UIItemCardSize[] = ['search', 'browse', 'selection', 'summary']

function draw(item: UIPickerItem, size: UIItemCardSize, k?: UIItemKinds, guide?: string): string {
  return renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(SourceItem, { item, size, kinds: k, badges, guide })))
}

describe('one item card at four sizes', () => {
  test('each size draws the roles it needs, read from the item', () => {
    const search = itemCardParts(arbor, 'search', { kinds, badges })
    expect(search).toEqual({
      title: 'Arbor', kind: 'Application', path: ['Logistique', 'Transport'], why: '« Arbor » dans le nom',
      badges: [{ label: 'IA', color: 'violet' }], slots: [{ attr: 'code', label: 'Code', value: 'RDY' }],
    })
    const browse = itemCardParts(arbor, 'browse', { kinds, badges })
    expect(browse.path).toBeUndefined()
    expect(browse.why).toBeUndefined()
    expect(browse.kind).toBe('Application')
    const selection = itemCardParts(arbor, 'selection', { kinds, badges })
    expect(selection.kind).toBeUndefined()
    expect(selection.path).toEqual(['Logistique', 'Transport'])
    expect(selection.why).toBe('« Arbor » dans le nom')
    const summary = itemCardParts(arbor, 'summary', { kinds, badges })
    expect(summary.detail).toBe('Planification des tournées')
    expect(summary.modified).toBe('2026-10-03T08:12:00Z')
    expect(summary.slots.map((s) => s.attr)).toEqual(['code', 'owner'])
  })

  test('the four sizes draw the same item, each its own lines', () => {
    const html = Object.fromEntries(sizes.map((s) => [s, draw(arbor, s, kinds)]))
    for (const s of sizes) {
      expect(html[s]).toContain(`data-size="${s}"`)
      expect(html[s]).toContain('Arbor')
      expect(html[s]).toContain('IA')
    }
    expect(html.search).toContain('Logistique › Transport')
    expect(html.browse).not.toContain('Logistique')
    expect(html.browse).not.toContain('dans le nom')
    expect(html.selection).not.toContain('item-card.kind')
    expect(html.summary).toContain('Planification des tournées')
    expect(html.summary).toContain('modifié le 03/10/2026')
    expect(html.search).not.toContain('modifié le')
  })

  test('a kind slot is drawn when the kind declares it, at the sizes it names', () => {
    expect(draw(arbor, 'search', kinds)).toContain('data-guide-name="code"')
    expect(draw(arbor, 'search', kinds)).not.toContain('data-guide-name="owner"')
    expect(draw(arbor, 'summary', kinds)).toContain('data-guide-name="owner"')
    // No declaration: no slot, the kind its own word.
    const bare = draw(arbor, 'search')
    expect(bare).not.toContain('item-card.slot')
    expect(bare).toContain('application')
    expect(bare).not.toContain('RDY')
    // A declared slot the item has no attribute for is not drawn.
    expect(itemCardParts({ ...arbor, attrs: {} }, 'summary', { kinds }).slots).toEqual([])
  })

  test('a state takes the why line (its tooltip), a nested row drops its path', () => {
    expect(itemCardParts({ ...arbor, state: 'failed' }, 'selection').why).toBeUndefined()
    expect(itemCardParts(arbor, 'search', { nested: true }).path).toBeUndefined()
    expect(modifiedDay('pas une date')).toBe('pas une date')
    expect(ITEM_CARD_ROLES.summary).toContain('modified')
  })

  test('the multi-source search draws its items through the card, the source kinds laid over the node', () => {
    const group = {
      answer: { connector: 'leanix', label: 'LeanIX', state: 'found', found: 1, items: [arbor],
        kinds: { application: { slots: [{ attr: 'owner', label: 'Responsable' }] } } },
      ticked: ['app-arbor'], order: 1,
    }
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(SourceGroupView, { group, labels: SOURCE_SEARCH_LABELS, kinds, onTick: () => {}, onAll: () => {} })))
    expect(html).toContain('source-search.item-card')
    expect(html).toContain('data-size="search"')
    expect(html).toContain('source-search.why')
    expect(html).toContain('data-guide-name="owner"')
    expect(html).not.toContain('data-guide-name="code"')
  })
})
