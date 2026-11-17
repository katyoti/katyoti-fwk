// SPDX-License-Identifier: Apache-2.0
// A card with height: fill takes the window's height below it and scrolls its
// last child inside itself: two such cards in a grid row
// each scroll their own content and the page does not. A static render cannot
// measure the window: the card gets the default fill height.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { FILL_HEIGHT_DEFAULT } from '../src/ui-builders/page/nodes/fill-height.ts'

const draw = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(UIPageView, { node: node as never, onAction: () => {} })))

const box = (id: string, height?: 'fill') => ({
  kind: 'card', id, ...(height ? { height } : {}), children: [
    { kind: 'heading', text: `Box ${id}`, level: 5 },
    { kind: 'vbox', children: Array.from({ length: 30 }, (_, i) => ({ kind: 'text', content: `${id} line ${i}` })) },
  ],
})

/** The markup of the card whose element id is card:<id>, to its end. */
const cardOf = (html: string, id: string) => html.slice(html.indexOf(`data-element="card:${id}"`) - 400)

describe('a card with height: fill', () => {
  const html = draw({ kind: 'grid', cols: 2, children: [box('running', 'fill'), box('next', 'fill')] })

  test('each card of the row is marked fill and takes a measured height', () => {
    for (const id of ['running', 'next']) {
      expect(html).toMatch(new RegExp(`<div[^>]*data-element="card:${id}"[^>]*>`))
      const tag = html.match(new RegExp(`<div[^>]*data-element="card:${id}"[^>]*>`))![0]
      expect(tag).toContain('data-fill=""')
      expect(tag).toContain(`height:${FILL_HEIGHT_DEFAULT}px`)
    }
  })

  test('its last child scrolls inside it, the heading stays above', () => {
    const running = cardOf(html, 'running')
    const scroll = running.indexOf('data-fill-scroll')
    expect(scroll).toBeGreaterThan(-1)
    expect(running.indexOf('Box running')).toBeLessThan(scroll)
    expect(running.indexOf('running line 0')).toBeGreaterThan(scroll)
    expect(html.split('data-fill-scroll').length - 1).toBe(2)
  })

  test('a card without it keeps its plain layout', () => {
    const plain = draw(box('plain'))
    const tag = plain.match(/<div[^>]*data-element="card:plain"[^>]*>/)![0]
    expect(tag).not.toContain('data-fill')
    expect(tag).toContain('height:100%')
    expect(plain).not.toContain('data-fill-scroll')
  })
})
