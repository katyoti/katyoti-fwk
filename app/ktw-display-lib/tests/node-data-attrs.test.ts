// SPDX-License-Identifier: Apache-2.0
// A page node's own data-* keys reach its
// rendered element - spread first on the renderer's root, never on a wrapper,
// the renderer's own attributes winning on a clash.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { dataAttrs } from '../src/ui-builders/page/nodes/data-attrs.ts'

const page = (node: unknown, host = false) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(UIPageView, { node: node as never, ...(host ? { onAction: () => {} } : {}) })))

/** The node's root: the first element in the page's own display:contents
 *  box, the <style> tags around it left out. */
function rootTag(html: string): string {
  const rest = html.replace(/^(\s*<style[^>]*>[\s\S]*?<\/style>)+/, '').replace(/^<div style="display:contents">/, '')
    .replace(/^(\s*<style[^>]*>[\s\S]*?<\/style>)+/, '')
  const m = rest.match(/^<[a-zA-Z][^>]*>/)
  return m ? m[0] : ''
}

const action = { id: 'go', label: 'Go', uri: 'x:tools/go' }

describe("a page node's data-* keys reach its element", () => {
  test('dataAttrs keeps only the data-* keys, as strings', () => {
    expect(dataAttrs({ kind: 'text', dataField: 'a', 'data-view': 'v', 'data-n': 3, 'data-on': true, 'data-o': { a: 1 }, 'data-x': null }))
      .toEqual({ 'data-view': 'v', 'data-n': '3', 'data-on': 'true', 'data-o': '{"a":1}' })
    expect(dataAttrs(undefined)).toEqual({})
  })

  const kinds: [string, unknown, boolean?][] = [
    ['vbox', { kind: 'vbox', 'data-view': 'k', children: [{ kind: 'text', content: 'a' }] }],
    ['hbox', { kind: 'hbox', 'data-view': 'k', children: [{ kind: 'text', content: 'a' }] }],
    ['text', { kind: 'text', content: 'Hello', 'data-view': 'k' }],
    ['grid', { kind: 'grid', cols: 2, 'data-view': 'k', children: [] }],
    ['grid by ratio', { kind: 'grid', cols: 3, ratio: [6, 2, 4], 'data-view': 'k', children: [] }],
    ['button with a host', { kind: 'button', label: 'Go', action, 'data-view': 'k' }, true],
    ['button with no host', { kind: 'button', label: 'Go', action, 'data-view': 'k' }],
    ['badge', { kind: 'badge', text: 'held', 'data-view': 'k' }],
    ['badge with an action', { kind: 'badge', text: 'held', action, 'data-view': 'k' }, true],
  ]
  for (const [name, node, host] of kinds) {
    test(`${name}: on its root element`, () => {
      expect(rootTag(page(node, host))).toContain('data-view="k"')
    })
  }

  test('a nested node carries its own keys on its own element, no wrapper added', () => {
    const html = page({ kind: 'vbox', 'data-view': 'outer', children: [
      { kind: 'text', content: 'T', 'data-title': 'IN-062' },
      { kind: 'badge', text: 'held', 'data-held': 'IN-062' },
    ] })
    expect(rootTag(html)).toContain('data-view="outer"')
    expect(html).toMatch(/<p[^>]*data-title="IN-062"[^>]*>T<\/p>/)
    expect(html).toMatch(/data-held="IN-062"/)
    // The same tree without its keys draws the same elements: none added.
    const bare = page({ kind: 'vbox', children: [{ kind: 'text', content: 'T' }, { kind: 'badge', text: 'held' }] })
    expect(html.replace(/ data-(view|title|held)="[^"]*"/g, '')).toBe(bare)
  })

  test("on a clash the renderer's own attribute wins", () => {
    const tag = rootTag(page({ kind: 'grid', cols: 3, ratio: [1, 1], 'data-guide': 'mine', 'data-view': 'k', children: [] }))
    expect(tag).toContain('data-guide="grid.ratio"')
    expect(tag).not.toContain('data-guide="mine"')
    expect(tag).toContain('data-view="k"')
  })
})
