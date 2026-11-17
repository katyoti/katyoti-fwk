// SPDX-License-Identifier: Apache-2.0
// A page text clamps to a number of lines, draws in the
// monospace font on request and, expandable, shows a toggle « Afficher plus »
// / « Afficher moins » only when it overflows its lines.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import i18next from 'i18next'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { ClampedText, TextToggle, measureOverflow, textOverflows } from '../src/ui-builders/page/nodes/atomic.tsx'
import { showLessLabel, showMoreLabel } from '../src/ui-builders/page/nodes/window.tsx'

const wrap = (el: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, el as never))
const page = (node: unknown, data?: unknown) => wrap(createElement(UIPageView, { node: node as never, data, onAction: () => {} } as never))
const long = 'A very long request title that holds more than two lines once it is drawn in a narrow column of the deck'

describe('a clamped, mono, expandable text', () => {
  test('the grammar declares the three props', () => {
    const t = (UIPageNodeJSONSchema as any).definitions.Text.properties
    expect(t.lineClamp).toEqual(expect.objectContaining({ type: 'integer', minimum: 1 }))
    expect(t.mono).toEqual(expect.objectContaining({ type: 'boolean' }))
    expect(t.expandable).toEqual(expect.objectContaining({ type: 'boolean' }))
  })

  test('lineClamp clamps a literal text to its lines', () => {
    const html = page({ kind: 'text', id: 't1', content: long, lineClamp: 2 })
    expect(html).toContain('data-line-clamp="true"')
    expect(html).toContain('--text-line-clamp:2')
    expect(html).toContain(long)
  })

  test('lineClamp clamps a text bound by dataField the same', () => {
    const html = page({ kind: 'vbox', children: [{ kind: 'text', content: '', dataField: '$.title', lineClamp: 3 }] }, { title: long })
    expect(html).toContain('--text-line-clamp:3')
    expect(html).toContain(long)
  })

  test('no lineClamp draws the text whole', () => {
    const html = page({ kind: 'text', content: long })
    expect(html).not.toContain('data-line-clamp')
    expect(html).not.toContain('--text-line-clamp')
  })

  test('mono draws the monospace font, case kept', () => {
    const html = page({ kind: 'text', content: 'm.RQ-433', mono: true })
    expect(html).toContain('font-family:var(--mantine-font-family-monospace)')
    expect(html).toContain('m.RQ-433')
    expect(html).not.toContain('text-transform')
    expect(page({ kind: 'text', content: 'x' })).not.toContain('font-family-monospace')
  })

  test('an expandable text that is not seen to overflow draws no toggle', () => {
    const html = page({ kind: 'text', id: 'note', content: 'short', lineClamp: 3, expandable: true })
    expect(html).toContain('--text-line-clamp:3')
    expect(html).not.toContain('text:note:toggle')
    expect(html).not.toContain(showMoreLabel('fr'))
    expect(html).not.toContain(showMoreLabel('en'))
  })

  test('the toggle shows only on overflow', () => {
    expect(textOverflows({ scrollHeight: 60, clientHeight: 40 })).toBe(true)
    expect(textOverflows({ scrollHeight: 40, clientHeight: 40 })).toBe(false)
    expect(textOverflows({ scrollHeight: 41, clientHeight: 40 })).toBe(false)
    expect(textOverflows(null)).toBe(false)
  })

  test('the toggle has its own element id and says « Afficher plus » folded, « Afficher moins » open', async () => {
    await i18next.init({ lng: 'fr', resources: {} })
    await i18next.changeLanguage('fr')
    const folded = wrap(createElement(TextToggle, { id: 'note', open: false, onToggle: () => {} }))
    expect(folded).toContain('data-element="text:note:toggle"')
    expect(folded).toContain('aria-expanded="false"')
    expect(folded).toContain('Afficher plus')
    const opened = wrap(createElement(TextToggle, { id: 'note', open: true, onToggle: () => {} }))
    expect(opened).toContain('aria-expanded="true"')
    expect(opened).toContain('Afficher moins')
    expect(wrap(createElement(TextToggle, { open: false, onToggle: () => {} }))).not.toContain('data-element')
    expect(showLessLabel('en')).toBe('Show less')
    expect(showMoreLabel('en')).toBe('Show more')
  })

  // In the real GUI the toggle never showed. The measure
  // saw the overflow, the toggle's wrapper then moved the text under a new
  // parent - a new <p>, the measured one removed; the observer, still on the
  // removed one, measured it 0 high and took the toggle away again.
  test('the text keeps its place in the tree when the toggle shows', () => {
    const text = createElement('p', { 'data-probe': 'clamped' }, long)
    const before = (open: boolean, overflows: boolean) => {
      const html = wrap(createElement(ClampedText, { text, id: 'note', open, overflows, onToggle: () => {} }))
      return html.slice(0, html.indexOf('<p data-probe="clamped"'))
    }
    const folded = before(false, false)
    expect(folded).not.toBe('')
    expect(before(false, true)).toBe(folded)
    expect(before(true, false)).toBe(folded)
    const shown = wrap(createElement(ClampedText, { text, id: 'note', open: false, overflows: true, onToggle: () => {} }))
    expect(shown).toContain('text:note:toggle')
    expect(wrap(createElement(ClampedText, { text, id: 'note', open: false, overflows: false, onToggle: () => {} }))).not.toContain('text:note:toggle')
  })

  test('a text out of the document says nothing of the overflow', () => {
    expect(measureOverflow({ scrollHeight: 0, clientHeight: 0, isConnected: false })).toBeUndefined()
    expect(measureOverflow({ scrollHeight: 60, clientHeight: 40, isConnected: false })).toBeUndefined()
    expect(measureOverflow(null)).toBeUndefined()
    expect(measureOverflow({ scrollHeight: 60, clientHeight: 40, isConnected: true })).toBe(true)
    expect(measureOverflow({ scrollHeight: 40, clientHeight: 40, isConnected: true })).toBe(false)
  })

  // A release page's tree row opens on click; the
  // note's toggle sits in it - its click unfolds the note and stops there,
  // the row does not open.
  test("the toggle's click unfolds the text and does not reach the row", () => {
    let toggled = 0
    let stopped = 0
    const el = TextToggle({ id: 'note', open: false, onToggle: () => { toggled++ } }) as any
    el.props.onClick({ stopPropagation: () => { stopped++ } })
    expect(toggled).toBe(1)
    expect(stopped).toBe(1)
  })
})
