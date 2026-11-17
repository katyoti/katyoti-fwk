// SPDX-License-Identifier: Apache-2.0
// The generic pieces of the goal card
// (modules/research-delivery-overview/request-rows-mockup.html section 4) -
// a stacked gauge whose legend is drawn as swatches (the mock-up's .legend)
// and a card whose border is drawn dashed (the mock-up's .constraint).

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

const page = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, onAction: () => {} })))
const gauge = (legend: unknown) => ({
  kind: 'gauge', legend, segments: [
    { value: 17, color: 'green', label: 'done' },
    { value: 1, color: 'orange', label: 'running' },
    { value: 0, color: 'violet', label: 'waiting' },
  ],
})

describe('a stacked gauge with a swatch legend', () => {
  test('the grammar declares the pieces', () => {
    const d = (UIPageNodeJSONSchema as any).definitions
    expect(d.Gauge.properties.legend.anyOf).toEqual([{ type: 'boolean' }, { const: 'swatch' }])
    expect(d.Card.properties.dashed).toEqual(expect.objectContaining({ type: 'boolean' }))
  })

  test('each shown part is a square in its own colour beside its count and label', () => {
    const html = page(gauge('swatch'))
    expect(html).toContain('data-gauge-legend="swatch"')
    const legend = html.slice(html.indexOf('data-gauge-legend="swatch"'))
    expect(legend).toMatch(/data-swatch="done"><span style="[^"]*width:8px;height:8px;border-radius:2px;margin-right:4px[^"]*background:var\(--mantine-color-green-filled\)"><\/span>17 done<\/span>/)
    expect(legend).toMatch(/data-swatch="running"><span style="[^"]*background:var\(--mantine-color-orange-filled\)"><\/span>1 running<\/span>/)
    expect(legend).not.toContain('data-swatch="waiting"')
  })

  test('the legend wraps with a 10px gap in dimmed 11px text, with no chips', () => {
    const html = page(gauge('swatch'))
    expect(html).toMatch(/data-gauge-legend="swatch" style="display:flex;flex-wrap:wrap;gap:10px;font-size:11px;color:var\(--mantine-color-dimmed\)"/)
    expect(html).not.toContain('mantine-Badge-root')
  })

  test('legend true keeps the badges; no legend draws none', () => {
    const chips = page(gauge(true))
    expect(chips).toContain('mantine-Badge-root')
    expect(chips).not.toContain('data-gauge-legend')
    const none = page(gauge(undefined))
    expect(none).not.toContain('mantine-Badge-root')
    expect(none).not.toContain('data-gauge-legend')
  })
})

describe('a card with a dashed border', () => {
  const card = (extra: Record<string, unknown>) => ({ kind: 'card', children: [{ kind: 'text', content: 'constraint' }], ...extra })

  test('dashed draws its normal border dashed', () => {
    const html = page(card({ dashed: true }))
    expect(html).toContain('data-dashed=""')
    expect(html).toMatch(/data-with-border="true"[^>]*style="[^"]*border-style:dashed|style="[^"]*border-style:dashed[^"]*"[^>]*data-with-border="true"/)
  })

  test('a card without it keeps its solid border', () => {
    const html = page(card({}))
    expect(html).not.toContain('border-style:dashed')
    expect(html).not.toContain('data-dashed')
  })
})

describe('a card with a stripe', () => {
  const card = (extra: Record<string, unknown>) => ({ kind: 'card', children: [{ kind: 'text', content: 'a blocker' }], ...extra })

  test('the grammar declares it', () => {
    expect((UIPageNodeJSONSchema as any).definitions.Card.properties.stripe).toEqual(expect.objectContaining({ type: 'string' }))
  })

  test('stripe draws a band of its tone down its left edge', () => {
    const html = page(card({ stripe: 'yellow' }))
    expect(html).toContain('data-stripe="yellow"')
    expect(html).toMatch(/style="[^"]*border-left:6px solid var\(--mantine-color-yellow-6\)/)
  })

  test('a card without it has no band', () => {
    const html = page(card({}))
    expect(html).not.toContain('data-stripe')
    expect(html).not.toContain('border-left:6px')
  })
})
