// SPDX-License-Identifier: Apache-2.0
// A section: on a progressive page the display library draws
// its placeholder, asks the host to load it when it comes into view (or at
// once, eager or open), and draws what the load answers in its place, by its
// id; with the host's flag off, or no loader, it draws as declared.

import { describe, expect, test } from 'bun:test'

import {
  placeholderShape, sectionContent, sectionLoadsNow, sectionMode, sectionWhen,
} from '../../../ktw-display-lib/src/ui-builders/page/nodes/section-load'

const section = { kind: 'card', id: 'lane:s1', load: { method: 'ktw-x:views/lane', args: { stream: 's1' } }, placeholder: { kind: 'cards', count: 3 } }

describe('a section', () => {
  test('loads only on a progressive page whose host can load it', () => {
    expect(sectionMode(section, true, true)).toBe('load')
    expect(sectionMode(section, false, true)).toBe('as-is')
    expect(sectionMode(section, true, false)).toBe('as-is')
    expect(sectionMode({ kind: 'card', id: 'x' }, true, true)).toBe('as-is')
  })

  test('loads when visible by default, at once when eager or open, at once where the surface cannot tell', () => {
    expect(sectionWhen(section as never)).toBe('visible')
    expect(sectionWhen({ when: 'eager' })).toBe('eager')
    expect(sectionLoadsNow('visible', false, true)).toBe(false)
    expect(sectionLoadsNow('visible', true, true)).toBe(true)
    expect(sectionLoadsNow('eager', false, true)).toBe(true)
    expect(sectionLoadsNow('open', false, true)).toBe(true)
    expect(sectionLoadsNow('visible', false, false)).toBe(true)
  })

  test('draws what its load answers: the inline tree, a page, or a node', () => {
    const tree = { kind: 'text', content: 'loaded' }
    expect(sectionContent({ tree, data: { n: 1 } })).toEqual({ node: tree as never, data: { n: 1 } })
    expect(sectionContent({ dslTree: tree, data: 2 })).toEqual({ node: tree as never, data: 2 })
    expect(sectionContent(tree)).toEqual({ node: tree as never })
    // a tree still named by URI is the host's to resolve
    expect(sectionContent({ tree: { uri: 'ktw-x:/page-template/lane', hash: 'h' } })).toBeUndefined()
    expect(sectionContent(undefined)).toBeUndefined()
    expect(sectionContent({ ok: true })).toBeUndefined()
  })

  test('its placeholder is skeleton, cards or rows, a few of them', () => {
    expect(placeholderShape({ kind: 'cards', count: 3 })).toEqual({ kind: 'cards', count: 3 })
    expect(placeholderShape(undefined)).toEqual({ kind: 'skeleton', count: 3 })
    expect(placeholderShape({ kind: 'rows', count: 400 })).toEqual({ kind: 'rows', count: 12 })
  })
})
