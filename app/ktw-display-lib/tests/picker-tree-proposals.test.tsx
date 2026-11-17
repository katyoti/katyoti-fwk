// SPDX-License-Identifier: Apache-2.0
// A check-box tree declaring `proposals` draws mock-up v6's proposal rows
// (« Pages prises ») above its search: each row ticked through the tree's
// selection - on when taken, grey when a taken ancestor includes it -, its
// title then « cet élément seul » or « avec n sous-éléments », the path
// « / » joined « · » why, a pill at the right; « Tout cocher » skips the
// rows taken or included; the `choose` label above the search.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { browsedTree, PICKER_LABELS, PickerRenderer, tickAllTree, treeMark, treeRowNote } from '../src/ui-builders/page/nodes/picker.tsx'
import type { UIPickerItem, UIPickerNode } from '../src/ui-builders/page/ui-page.interactive.ts'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'

// P (taken with what sits under it, not proposed) > p2; p1, p3, p4 elsewhere.
const items: UIPickerItem[] = [
  { key: 'p1', title: 'Orchard', path: ['Space', 'Apps'], why: 'keyword « orchard »', below: 3, pill: { label: 'Example', color: 'teal', icon: 'link' } },
  { key: 'p2', title: 'Meadow', parent: 'P', path: ['Space', 'Pasture'], why: 'keyword « meadow »' },
  { key: 'p3', title: 'Brook', path: ['Space'], why: 'keyword « brook »', below: 2 },
  { key: 'p4', title: 'Grove', path: ['Space', 'Woods'], why: 'keyword « grove »', below: 1 },
]
const selected: UIPickerItem[] = [
  { key: 'P', title: 'Pasture', with_children: true },
  { key: 'p1', title: 'Orchard', with_children: true },
  { key: 'p3', title: 'Brook' },
]

const host = { callMethod: async () => ({ items: [] }) } as any
const draw = (node: UIPickerNode) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host }, createElement(PickerRenderer, { node }))))

/** row cuts one proposal row's markup out of the page. */
function row(html: string, key: string): string {
  const at = html.indexOf(`data-guide-name="${key}"`)
  expect(at).toBeGreaterThan(-1)
  const ends = [html.indexOf('data-guide="picker.tree-proposal"', at + 1), html.indexOf('data-guide="picker.choose"', at)].filter((i) => i > 0)
  return html.slice(html.lastIndexOf('<div', at), Math.min(...ends))
}

describe('a tree picker\'s proposal rows', () => {
  test('the marks come from the tree\'s selection, the proposal\'s parent known', () => {
    const tree = browsedTree([], {}, items)
    expect(treeMark(selected, 'p1', tree)).toBe('with')
    expect(treeMark(selected, 'p2', tree)).toBe('included')
    expect(treeMark(selected, 'p3', tree)).toBe('self')
    expect(treeMark(selected, 'p4', tree)).toBe('none')
  })

  test('the note: « cet élément seul » when alone, else the counted below label', () => {
    expect(treeRowNote('self', 2, PICKER_LABELS)).toBe('cet élément seul')
    expect(treeRowNote('with', 3, PICKER_LABELS)).toBe('avec 3 sous-éléments')
    expect(treeRowNote('none', 1, PICKER_LABELS)).toBe('avec 1 sous-élément')
    expect(treeRowNote('included', 0, PICKER_LABELS)).toBe('')
    expect(treeRowNote('with', 2, { ...PICKER_LABELS, belowMany: 'avec {n} sous-pages' })).toBe('avec 2 sous-pages')
  })

  test('« Tout cocher » ticks only the rows neither taken nor included', () => {
    const tree = browsedTree([], {}, items)
    const got = tickAllTree(selected, items, tree)
    expect(got.map((s) => `${s.key}${s.with_children ? '+' : ''}`).sort()).toEqual(['P+', 'p1+', 'p3', 'p4+'])
    // a proposal under another proposal is included once its parent is ticked
    const nested: UIPickerItem[] = [{ key: 'a', title: 'A', below: 1 }, { key: 'b', title: 'B', parent: 'a' }]
    expect(tickAllTree([], nested, browsedTree([], {}, nested)).map((s) => s.key)).toEqual(['a'])
  })

  test('the region draws above the search: marks, notes, path · why, pill, choose', () => {
    const html = draw({
      kind: 'picker', layout: 'tree', tickable: true, search: 'ktw-example:/method/search',
      proposals: { title: 'Proposed from your keywords', items, choose: 'Or choose in the tree of the space' },
      selected,
    })
    expect(html).toContain('data-guide="picker.tree-proposals"')
    expect(html).toContain('Proposed from your keywords')
    const p1 = row(html, 'p1'), p2 = row(html, 'p2'), p3 = row(html, 'p3'), p4 = row(html, 'p4')
    expect(p1).toContain('data-mark="with"')
    expect(p1).toContain('avec 3 sous-éléments')
    expect(p1).toContain('Space / Apps')
    expect(p1).toContain(' · ')
    expect(p1).toContain('keyword « orchard »')
    expect(p1).toContain('data-guide="picker.pill"')
    expect(p1).toContain('Example')
    expect(p2).toContain('data-mark="included"')
    expect(p2).toMatch(/checked=""/)
    expect(p2).not.toContain('data-guide="picker.pill"')
    expect(p3).toContain('data-mark="self"')
    expect(p3).toContain('cet élément seul')
    expect(p3).not.toContain('sous-éléments')
    expect(p4).toContain('data-mark="none"')
    expect(p4).toContain('avec 1 sous-élément')
    expect(p4).not.toMatch(/checked=""/)
    // « Tout cocher » while one row is open; choose sits above the search
    expect(html).toContain('data-guide="picker.tick-all"')
    const choose = html.indexOf('data-guide="picker.choose"')
    expect(choose).toBeGreaterThan(html.indexOf('data-guide="picker.tree-proposals"'))
    expect(html.indexOf('data-guide="picker.search"')).toBeGreaterThan(choose)
    expect(html).toContain('Or choose in the tree of the space')
  })

  test('no « Tout cocher » when every row is taken or included; none when no rows', () => {
    const all = draw({ kind: 'picker', layout: 'tree', proposals: { title: 'T', items: items.slice(0, 3) }, selected })
    expect(all).not.toContain('data-guide="picker.tick-all"')
    const none = draw({ kind: 'picker', layout: 'tree', proposals: { title: 'T', items: [], none: 'Nothing matches.' } })
    expect(none).toContain('data-guide="picker.proposals-none"')
    expect(none).toContain('Nothing matches.')
  })

  test('a flat tickable picker keeps no tree region', () => {
    const html = draw({ kind: 'picker', tickable: true, proposals: { title: 'T', items } })
    expect(html).not.toContain('data-guide="picker.tree-proposals"')
  })
})
