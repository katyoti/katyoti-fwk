// SPDX-License-Identifier: Apache-2.0
// Behind IN-045's switch (the host's sourceModel), the picker
// searches on Enter or the field's search icon - never while typing -, keeps
// its field editable while a search runs, nests search hits in the tree by
// their path and browses as an expandable tree. Off, the former picker.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { browseRows, browseTree, matchRows, openedAtOnce, PickerRenderer, searchesWhileTyping } from '../src/ui-builders/page/nodes/picker.tsx'

const render = (host: object, node: object) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))

describe('the source model picker', () => {
  test('never searches while typing where the host shows the source model', () => {
    expect(searchesWhileTyping({ live: true }, true)).toBe(false)
    expect(searchesWhileTyping({}, true)).toBe(false)
    // Off, the former live board keeps searching as it is typed in.
    expect(searchesWhileTyping({ live: true }, false)).toBe(true)
    expect(searchesWhileTyping({}, false)).toBe(false)
  })

  test('draws the field with its search icon, the field enabled', () => {
    const call = async () => ({ items: [] })
    const node = { kind: 'picker', id: 'p', search: 'x:s/search', live: true }
    const on = render({ callMethod: call, sourceModel: true }, node)
    expect(on).toContain('data-guide="picker.search-icon"')
    expect(on).toContain('aria-label="Chercher"')
    expect(on).not.toMatch(/data-guide="picker.search"[^>]*disabled/)
    const off = render({ callMethod: call }, node)
    expect(off).not.toContain('picker.search-icon')
  })

  test('nests search hits by their path, a hit holding the hits under it', () => {
    const rows = matchRows([
      { key: 'p2', title: 'Flux', path: ['AUP', 'ERP Arbor'] },
      { key: 'p1', title: 'ERP Arbor', path: ['AUP'] },
      { key: 'p3', title: 'Contrats', path: ['AUP', 'ERP Arbor'] },
      { key: 'x', title: 'Ailleurs' },
    ])
    expect(rows.map((r) => (r.kind === 'item' ? `${r.depth}:${r.item.key}` : `${r.depth}:place:${r.title}`))).toEqual([
      '0:place:AUP', '1:p1', '2:p2', '2:p3', '0:x',
    ])
  })

  test('keeps two hits of one title at one place apart', () => {
    const rows = matchRows([{ key: 'a', title: 'Flux', path: ['S'] }, { key: 'b', title: 'Flux', path: ['S'] }])
    expect(rows.filter((r) => r.kind === 'item').map((r) => (r as any).item.key)).toEqual(['a', 'b'])
  })

  test('browses an expandable tree, each child knowing its parent', () => {
    const top = [{ key: 'S', title: 'Space', has_children: true }, { key: 'T', title: 'Other', has_children: true }]
    const childrenOf = { S: [{ key: 'P', title: 'Page', has_children: true }], P: [{ key: 'C', title: 'Child' }] }
    const shut = browseRows(top, childrenOf, new Set())
    expect(shut.map((r) => r.item.key)).toEqual(['S', 'T'])
    const open = browseRows(top, childrenOf, new Set(['S', 'P']))
    expect(open.map((r) => `${r.depth}:${r.item.key}:${r.item.parent ?? ''}`)).toEqual(['0:S:', '1:P:S', '2:C:P', '0:T:'])
    // An item folded hides what sits under it, even expanded below.
    expect(browseRows(top, childrenOf, new Set(['P'])).map((r) => r.item.key)).toEqual(['S', 'T'])
  })

  test('opens the default scope the browse flags on its first draw, its children listed without a click', async () => {
    const asked: string[] = []
    const browse = async (parent?: string) => {
      asked.push(parent ?? '/')
      if (!parent) return [{ key: 'space:DEV', title: 'Développement', has_children: true, open: true }, { key: 'space:OPS', title: 'Exploitation', has_children: true }]
      if (parent === 'space:DEV') return [{ key: '101', title: 'Architecture actuelle' }]
      throw new Error(`unexpected browse of ${parent}`)
    }
    const got = await browseTree(browse)
    expect(asked).toEqual(['/', 'space:DEV'])
    expect(browseRows(got.top, got.childrenOf, got.expanded).map((r) => `${r.depth}:${r.item.key}`)).toEqual(['0:space:DEV', '1:101', '0:space:OPS'])
    // A flagged space whose browse fails stays closed, the tree still drawn.
    const failing = await browseTree(async (parent) => {
      if (parent) throw new Error('refused')
      return [{ key: 'space:DEV', title: 'Développement', has_children: true, open: true }]
    })
    expect([...failing.expanded]).toEqual([])
    expect(failing.top.map((m) => m.key)).toEqual(['space:DEV'])
    expect(failing.failed['space:DEV']).toContain('refused')
  })

  test('draws the top before the default scope\'s pages come, a slow space never holding the tree back', async () => {
    let release!: () => void
    const pages = new Promise<void>((r) => { release = r })
    const seen: string[][] = []
    const done = browseTree(async (parent) => {
      if (!parent) return [{ key: 'space:DEV', title: 'Développement', has_children: true, open: true }, { key: 'space:OPS', title: 'Exploitation', has_children: true }]
      await pages
      return [{ key: '101', title: 'Architecture actuelle' }]
    }, (top, opening) => seen.push([...top.map((m) => m.key), '|', ...opening]))
    await new Promise((r) => setTimeout(r, 10))
    // The space's pages still pending: the top is already given, DEV opening.
    expect(seen).toEqual([['space:DEV', 'space:OPS', '|', 'space:DEV']])
    release()
    const got = await done
    expect([...got.expanded]).toEqual(['space:DEV'])
  })

  test('draws the default space opened at once, its pages still coming, and closes it when its browse fails', () => {
    // On the top's arrival the flagged space is already expanded - the
    // first draw shows it opened, loading, however slowly its pages come.
    const first = openedAtOnce(new Set(), ['space:DEV'])
    expect([...first]).toEqual(['space:DEV'])
    expect(browseRows([{ key: 'space:DEV', title: 'Développement', has_children: true, open: true }], {}, first).map((r) => r.item.key)).toEqual(['space:DEV'])
    // Its browse failed: closed again, the others kept.
    expect([...openedAtOnce(new Set(['space:OPS', 'space:DEV']), [], ['space:DEV'])]).toEqual(['space:OPS'])
  })
})
