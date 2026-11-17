// SPDX-License-Identifier: Apache-2.0
// Drawing only what is seen: on a progressive page (the
// host's flag) a list, a tree, a lane and a grid cell draw a page
// of their items and « Afficher plus » for the rest; a closed tab or accordion
// item is not mounted. With the flag off everything draws, as before.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { DEFAULT_PAGE_SIZE, panelMounted, showMoreLabel, windowSize } from '../src/ui-builders/page/nodes/window.tsx'

const page = (node: unknown, progressive: boolean) => renderToStaticMarkup(
  createElement(MantineProvider, null,
    createElement(HostServicesContext.Provider, { value: progressive ? { progressive: true } : {} },
      createElement(UIPageView, { node: node as never, onAction: (() => {}) as never }))))

const count = (html: string, mark: string) => html.split(mark).length - 1

const many = Array.from({ length: 10000 }, (_, i) => ({ id: `i${i}`, title: `item ${i}` }))

describe('the window rules', () => {
  test('a page of items on a progressive page, all of them otherwise', () => {
    expect(windowSize(10000, false, 20, 1)).toBe(10000)
    expect(windowSize(10000, true, 20, 1)).toBe(20)
    expect(windowSize(10000, true, 20, 3)).toBe(60)
    expect(windowSize(10, true, 20, 1)).toBe(10)
    expect(windowSize(10000, true, undefined, 1)).toBe(DEFAULT_PAGE_SIZE)
    expect(windowSize(10000, true, 0, 0)).toBe(DEFAULT_PAGE_SIZE)
  })
  test('« Afficher plus » in French, with what is left', () => {
    expect(showMoreLabel('fr', 9980)).toBe('Afficher plus (9980)')
    expect(showMoreLabel('fr-FR', 3)).toBe('Afficher plus (3)')
    expect(showMoreLabel('en', 3)).toBe('Show more (3)')
  })
  test('a tab or an accordion item mounts only once open on a progressive page', () => {
    expect(panelMounted(true, false)).toBe(false)
    expect(panelMounted(true, true)).toBe(true)
    expect(panelMounted(false, false)).toBe(true)
  })
})

describe('a progressive page draws only what is seen', () => {
  const list = { kind: 'list', id: 'big', data: many, pageSize: 20, item: { kind: 'text', content: 'ROW' } }

  test('a 10 000-item list draws one page and « Afficher plus », all of them with the flag off', () => {
    const on = page(list, true)
    expect(count(on, '>ROW<')).toBe(20)
    expect(on).toContain('data-show-more="big"')
    const off = page(list, false)
    expect(count(off, '>ROW<')).toBe(10000)
    expect(off).not.toContain('data-show-more')
  })

  test('a lane draws a page of its cards', () => {
    const lanes = { kind: 'lanes', pageSize: 5, lanes: [{ id: 'a', title: 'A' }, { id: 'b', title: 'B' }],
      data: [...many.slice(0, 30).map((c) => ({ ...c, lane: 'a' })), ...many.slice(30, 33).map((c) => ({ ...c, lane: 'b' }))],
      item: { kind: 'text', content: 'CARD' } }
    const on = page(lanes, true)
    expect(count(on, '>CARD<')).toBe(5 + 3)
    expect(on).toContain('data-show-more="a"')
    expect(on).not.toContain('data-show-more="b"')
    expect(count(page(lanes, false), '>CARD<')).toBe(33)
  })

  test('a tree draws a page of its rows', () => {
    const tree = { kind: 'tree', id: 't', pageSize: 10, expanded: true, data: many.slice(0, 40), item: { kind: 'text', content: 'NODE' } }
    expect(count(page(tree, true), '>NODE<')).toBe(10)
    expect(count(page(tree, false), '>NODE<')).toBe(40)
  })

  test('a closed tab and a closed accordion item are not mounted', () => {
    const tabs = { kind: 'tabs', tabs: [
      { id: 'one', label: 'One', content: { kind: 'text', content: 'TAB-ONE' } },
      { id: 'two', label: 'Two', content: { kind: 'text', content: 'TAB-TWO' } },
    ] }
    expect(page(tabs, true)).toContain('TAB-ONE')
    expect(page(tabs, true)).not.toContain('TAB-TWO')
    expect(page(tabs, false)).toContain('TAB-TWO')
    const acc = { kind: 'accordion', defaultOpen: ['p1'], panels: [
      { id: 'p1', label: 'P1', content: { kind: 'text', content: 'ACC-ONE' } },
      { id: 'p2', label: 'P2', content: { kind: 'text', content: 'ACC-TWO' } },
    ] }
    expect(page(acc, true)).toContain('ACC-ONE')
    expect(page(acc, true)).not.toContain('ACC-TWO')
    expect(page(acc, false)).toContain('ACC-TWO')
  })
})
