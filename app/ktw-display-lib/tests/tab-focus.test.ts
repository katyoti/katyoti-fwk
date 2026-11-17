// SPDX-License-Identifier: Apache-2.0
// One action switches to a tab and brings an element of it
// into view, highlighted for a moment.

import { describe, expect, test } from 'bun:test'

import { askTab, findFocus, focusSelectors, highlight, routePageAction, tabShown } from '../src/ui-builders/page/page-context.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

/** A root answering querySelector from a table of selectors. */
const rootWith = (found: Record<string, string>) => ({
  querySelector: (s: string) => (found[s] ? ({ name: found[s] } as unknown as Element) : null),
})

/** An element recording what is done to it. */
function element() {
  const attrs: Record<string, string> = {}
  const scrolled: unknown[] = []
  return {
    attrs, scrolled,
    style: { outline: '', outlineOffset: '' } as Record<string, string>,
    setAttribute: (k: string, v: string) => { attrs[k] = v },
    removeAttribute: (k: string) => { delete attrs[k] },
    scrollIntoView: (o: unknown) => { scrolled.push(o) },
  }
}

describe('a tab action with a focus', () => {
  test('is declared on actions', () => {
    expect((UIPageNodeJSONSchema as any).definitions.Action.properties.focus).toBeDefined()
  })

  test('one action shows the tab and carries the focus to the page', () => {
    const shown: unknown[] = []
    const on = { show: (t: string, f?: string) => shown.push([t, f]), select: () => {}, host: () => { throw new Error('a tab is the page’s own') }, selection: {} }
    routePageAction({ action: { tab: 'sources', focus: 'picker:confluence' } }, on)
    routePageAction({ action: { tab: 'sources' } }, on)
    expect(shown).toEqual([['sources', 'picker:confluence'], ['sources', undefined]])
    const asked = askTab(askTab(undefined, 'sources'), 'sources', 'picker:confluence')
    expect(asked).toEqual({ id: 'sources', n: 2, focus: 'picker:confluence' })
    expect(tabShown(asked, ['etude', 'sources'])).toBe('sources')
  })

  test('the focus names an element by its element id, its row name, its section or its DOM id - first match wins', () => {
    expect(focusSelectors('sources')).toEqual(['[data-element="sources"]', '[data-element$=":sources"]', '[data-guide-name="sources"]', '[data-section="sources"]', '[id="sources"]'])
    expect(focusSelectors('a"b\\c')[0]).toBe('[data-element="abc"]')
    expect((findFocus(rootWith({ '[data-element$=":confluence"]': 'picker', '[data-guide-name="confluence"]': 'row' }), 'confluence') as any).name).toBe('picker')
    expect((findFocus(rootWith({ '[data-guide-name="05"]': 'section row' }), '05') as any).name).toBe('section row')
    expect(findFocus(rootWith({}), 'nothing')).toBeNull()
    expect(findFocus(null, 'x')).toBeNull()
  })

  test('the element is scrolled into view and highlighted, then left as it was', async () => {
    const el = element()
    const off = highlight(el as unknown as Element, 20)
    expect(el.scrolled).toEqual([{ behavior: 'smooth', block: 'center' }])
    expect(el.attrs['data-focused']).toBe('true')
    expect(el.style.outline).toContain('solid')
    await new Promise((r) => setTimeout(r, 40))
    expect(el.attrs['data-focused']).toBeUndefined()
    expect(el.style.outline).toBe('')
    off()
  })
})
