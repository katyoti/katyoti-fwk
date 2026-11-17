// SPDX-License-Identifier: Apache-2.0
// A tree picker of several items is a check-box tree
// (mock-up v6 « Pages prises »): a ticked item taken with what sits under it
// or alone, an item under one taken with its children shown included, one
// with taken items below it mixed; unticking an included item takes its
// includer alone and keeps its other children taken.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import {
  browsedTree, setTreeMode, tickTree, treeMark, TreeHelp, TreeModeButtons, PICKER_LABELS,
} from '../src/ui-builders/page/nodes/picker.tsx'
import type { UIPickerItem } from '../src/ui-builders/page/ui-page.interactive.ts'

// space S > A (> A1 > A1a, A2), B
const top: UIPickerItem[] = [{ key: 'S', title: 'Espace', has_children: true }]
const childrenOf: Record<string, UIPickerItem[]> = {
  S: [{ key: 'A', title: 'Applications', has_children: true }, { key: 'B', title: 'Bureau' }],
  A: [{ key: 'A1', title: 'Arbor', has_children: true }, { key: 'A2', title: 'WCT' }],
  A1: [{ key: 'A1a', title: 'Flux d\'intégration' }],
}
const tree = browsedTree(top, childrenOf)
const keys = (sel: UIPickerItem[]) => sel.map((s) => `${s.key}${s.with_children ? '+' : ''}`).sort()

describe('the check-box tree', () => {
  test('ticking an item takes it with what sits under it, its parent known', () => {
    const got = tickTree([], 'A', tree)
    expect(keys(got.selection)).toEqual(['A+'])
    expect(got.selection[0].parent).toBe('S')
    expect(got.notice).toBeUndefined()
    expect(treeMark(got.selection, 'A', tree)).toBe('with')
    expect(treeMark(got.selection, 'A1', tree)).toBe('included')
    expect(treeMark(got.selection, 'A1a', tree)).toBe('included')
    expect(treeMark(got.selection, 'S', tree)).toBe('mixed')
    expect(treeMark(got.selection, 'B', tree)).toBe('none')
  })

  test('a leaf is taken alone', () => {
    expect(keys(tickTree([], 'B', tree).selection)).toEqual(['B'])
  })

  test('ticking an item takes in what was taken under it', () => {
    const one = tickTree([], 'A1a', tree).selection
    expect(keys(tickTree(one, 'A', tree).selection)).toEqual(['A+'])
  })

  test('« cet élément seul » takes the item alone, its children no longer included', () => {
    const sel = setTreeMode(tickTree([], 'A', tree).selection, 'A', 'self', tree)
    expect(keys(sel)).toEqual(['A'])
    expect(treeMark(sel, 'A', tree)).toBe('self')
    expect(treeMark(sel, 'A1', tree)).toBe('none')
    const back = setTreeMode(tickTree(sel, 'A2', tree).selection, 'A', 'with', tree)
    expect(keys(back)).toEqual(['A+'])
  })

  test('unticking an included item takes its includer alone, its other children kept', () => {
    const sel = tickTree([], 'A', tree).selection
    const got = tickTree(sel, 'A1a', tree)
    // A alone, A1 alone (the path down), A2 kept: A1a let go.
    expect(keys(got.selection)).toEqual(['A', 'A1', 'A2'])
    expect(got.selection.find((s) => s.key === 'A1')?.parent).toBe('A')
    expect(treeMark(got.selection, 'A1a', tree)).toBe('none')
    expect(got.notice).toEqual({ removed: 'Flux d\'intégration', parent: 'Applications' })
  })

  test('unticking a taken item lets it go', () => {
    expect(tickTree(tickTree([], 'A', tree).selection, 'A', tree).selection).toEqual([])
  })

  test('draws its help and its mode buttons', () => {
    const wrap = (el: ReturnType<typeof createElement>) => renderToStaticMarkup(createElement(MantineProvider, null, el))
    const help = wrap(createElement(TreeHelp, { labels: PICKER_LABELS }))
    expect(help).toContain('Comment se prennent les sous-éléments')
    const modes = wrap(createElement(TreeModeButtons, { mode: 'self', labels: { ...PICKER_LABELS, self: 'cette page seule' }, onMode: () => {} }))
    expect(modes).toContain('cette page seule')
    expect(modes).toContain('aria-pressed="true"')
  })
})
