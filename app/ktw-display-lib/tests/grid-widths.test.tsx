// SPDX-License-Identifier: Apache-2.0
// The generic pieces of the shared request row
// (modules/research-delivery-overview/request-rows-mockup.html section 3) -
// a grid sized by fixed pixel widths beside a share of the rest, with lines
// between its columns and a gap in pixels; a list card with its own padding
// that lightens under the pointer when it opens; an accordion drawn as rule
// lines, each fold keeping its data-* keys; a vbox centring its children.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { gridGap, widthColumns } from '../src/ui-builders/page/nodes/layout.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

const page = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, onAction: () => {} })))
const text = (content: string) => ({ kind: 'text', content })
const row = (widths: number[], extra: Record<string, unknown> = {}) => ({
  kind: 'grid', cols: widths.length, widths, gap: 0, dividers: [2, 3], ...extra,
  children: [{ kind: 'box', bg: 'orange.6', children: [] }, text('m.RQ-047'), text('the title'), text('4/6')],
})

describe('a grid by fixed widths', () => {
  test('the grammar declares the pieces', () => {
    const d = (UIPageNodeJSONSchema as any).definitions
    expect(d.Grid.properties.widths).toBeDefined()
    expect(d.Grid.properties.dividers).toBeDefined()
    expect(d.List.properties.cardPadding).toBeDefined()
    expect(d.Accordion.properties.look).toBeDefined()
    expect(d.PanelSpec.properties.color).toBeDefined()
    expect(d.VBox.properties.justify).toBeDefined()
  })

  test('a width in pixels is fixed, a 0 shares what is left - by its ratio when given', () => {
    expect(widthColumns([4, 150, 0, 210])).toBe('4px 150px minmax(0, 1fr) 210px')
    expect(widthColumns([4, 140, 0, 190])).toBe('4px 140px minmax(0, 1fr) 190px')
    expect(widthColumns([0, 120, 0], [2, 0, 1])).toBe('minmax(0, 2fr) 120px minmax(0, 1fr)')
  })

  test('a gap is pixels for a number, a spacing token otherwise', () => {
    expect(gridGap(0)).toBe('0px')
    expect(gridGap(6)).toBe('6px')
    expect(gridGap('sm')).toBe('var(--mantine-spacing-sm)')
    expect(gridGap(undefined)).toBe('var(--mantine-spacing-md)')
  })

  test('the row draws its four columns, cells touching, stretched, lined before the title and the steps only', () => {
    const html = page(row([4, 150, 0, 210], { 'data-request': 'm.RQ-047' }))
    expect(html).toContain('data-guide="grid.widths"')
    expect(html).toContain('data-request="m.RQ-047"')
    expect(html).toContain('grid-template-columns:4px 150px minmax(0, 1fr) 210px')
    expect(html).toContain('gap:0px')
    expect(html).toContain('align-items:stretch')
    expect(html.match(/data-guide="grid.divider"/g)?.length).toBe(2)
    // The line sits on the title's and the steps' cells, never the id's.
    expect(html).toMatch(/data-guide="grid.divider"[^>]*border-left:1px solid var\(--mantine-color-default-border\)[^>]*>(?:<[^>]+>)*the title/)
    expect(html).not.toMatch(/data-guide="grid.divider"[^>]*>(?:<[^>]+>)*sources\.RQ-047/)
  })

  test('the dense row takes its own widths; a grid by ratio alone is unchanged', () => {
    expect(page(row([4, 140, 0, 190]))).toContain('grid-template-columns:4px 140px minmax(0, 1fr) 190px')
    const ratio = page({ kind: 'grid', cols: 2, ratio: [2, 1], children: [text('a'), text('b')] })
    expect(ratio).toContain('data-guide="grid.ratio"')
    expect(ratio).not.toContain('grid.widths')
    expect(ratio).not.toContain('grid.divider')
  })

  test('a vbox centres its children along the height it is given', () => {
    expect(page({ kind: 'vbox', justify: 'center', children: [text('x')] })).toContain('--stack-justify:center')
  })
})

describe('a list card with no padding that lightens when it opens', () => {
  const list = (extra: Record<string, unknown>) => ({ kind: 'list', variant: 'cards', data: [{ id: 'a' }], item: text('cell'), ...extra })

  test('cardPadding 0 draws the card unpadded, the default stays sm', () => {
    expect(page(list({ cardPadding: 0 }))).toContain('--card-padding:0')
    expect(page(list({}))).toContain('--card-padding:var(--mantine-spacing-sm)')
  })

  test('a card that opens carries the hover look, one that does not carries none', () => {
    const opens = page(list({ open: { label: 'Open', navigate: { object: '${item.id}' } } }))
    expect(opens).toContain('ktw-item-opens')
    expect(opens).toContain('.ktw-item-opens:hover{border-color:')
    expect(page(list({}))).not.toContain('ktw-item-opens')
  })

  test('the hover draws the mock-up colours in the dark theme - border #4a4d55, background #2a2b31', () => {
    // request-rows-mockup.html line 34: .row:hover{border-color:#4a4d55;background:#2a2b31}
    const hover = page(list({ open: { label: 'Open', navigate: { object: '${item.id}' } } })).match(/\.ktw-item-opens:hover\{([^}]*)\}/)
    expect(hover).not.toBeNull()
    const rule = hover![1].toLowerCase()
    expect(rule).toMatch(/border-color:light-dark\([^,]+,\s*#4a4d55\)/)
    expect(rule).toMatch(/background:light-dark\([^,]+,\s*#2a2b31\)/)
  })
})

describe('an accordion drawn as rule lines', () => {
  const folds = {
    kind: 'accordion', look: 'rule', multiple: true, defaultOpen: ['running'], panels: [
      { id: 'running', label: 'Running', badge: 2, color: 'orange', 'data-group': 'running', content: text('row one') },
      { id: 'done', label: '59 done', content: text('a done chip') },
    ],
  }

  test('a fold with a colour is a caret, its tag, its count and a rule - no frame', () => {
    const html = page(folds)
    expect(html).toContain('data-guide="accordion.rule"')
    expect(html).not.toContain('mantine-Accordion')
    expect(html).toMatch(/data-guide="fold.head" data-guide-name="running"[^>]*>.*?▾/)
    expect(html).toMatch(/data-guide="fold.tag"[^>]*background:var\(--mantine-color-orange-light\)[^>]*>Running</)
    expect(html).toMatch(/data-guide="fold.badge" data-guide-name="running"[^>]*>2</)
    expect(html).toContain('data-guide="fold.rule"')
    expect(html).toContain('text-transform:uppercase')
  })

  test('a fold with no colour is the quiet line « ▸ 59 done », folded, with no tag and no rule', () => {
    const html = page(folds)
    const done = html.slice(html.indexOf('data-fold="done"'))
    expect(done).toMatch(/data-guide-name="done"[^>]*>.*?▸.*?<span>59 done<\/span>/)
    expect(done).not.toContain('fold.tag')
    expect(done).not.toContain('fold.rule')
    expect(done).toMatch(/<div hidden="">.*a done chip/)
  })

  test('each fold keeps its data-* keys, its open one shown', () => {
    const html = page(folds)
    expect(html).toMatch(/data-group="running" data-fold="running" data-open=""/)
    expect(html).toMatch(/data-fold="running"[^]*?<div>.*?row one/)
  })
})
