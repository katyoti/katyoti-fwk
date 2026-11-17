// SPDX-License-Identifier: Apache-2.0
// A KPI tile can be marked selected: a page whose tiles are filters (« À
// faire »: a tile per action) marks the one chosen — a 2px ring in the
// tile's colour (theme variables: it holds in light and dark), the
// clickable tile the current one for assistive tech; the others unmarked.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { KpiTile, kpiTileStyle } from '../src/ui-core/components/other/index.ts'
import { KPIRenderer, kpiPressed } from '../src/ui-builders/page/nodes/atomic.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

describe('a selected KPI tile', () => {
  test('is ringed in its colour, under its coloured top', () => {
    const s = kpiTileStyle('grape', true)
    expect(s.border).toBe('2px solid var(--mantine-color-grape-6)')
    expect(s.borderTop).toBe('3px solid var(--mantine-color-grape-6)')
  })

  test('a tile with no colour is ringed in the primary colour', () => {
    expect(kpiTileStyle(undefined, true).border).toBe('2px solid var(--mantine-primary-color-filled)')
  })

  test('a tile not chosen has no ring', () => {
    expect(kpiTileStyle('grape', false).border).toBeUndefined()
    expect(kpiTileStyle('grape').border).toBeUndefined()
  })

  test('the chosen clickable tile is the current one; the others are not', () => {
    expect(kpiPressed({ selected: true })).toEqual({ 'aria-current': 'true' })
    expect(kpiPressed({ selected: false })).toEqual({})
    expect(kpiPressed({})).toEqual({})
  })

  test('the tile draws the mark', () => {
    const html = (selected: boolean) => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(KpiTile, { label: 'Décider', value: 17, color: 'grape', selected })))
    expect(html(true)).toContain('data-selected="true"')
    expect(html(true)).toContain('2px solid var(--mantine-color-grape-6)')
    expect(html(false)).not.toContain('data-selected')
    expect(html(false)).not.toContain('2px solid')
  })

  test('the page schema declares it', () => {
    const kpi = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, unknown> }> }).definitions.KPI
    expect(kpi.properties.selected).toEqual({ type: 'boolean' })
  })
})

// A tile's children sit inside it, under its caption
// (the deck's « Needs you » tile and its « Ouvrir À faire » button).
describe('a KPI tile with children', () => {
  const html = () => renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(KPIRenderer, {
      node: { kind: 'kpi', label: 'Needs you', value: 3, color: 'grape', 'data-view': 'deck-needs-tile', children: [{ kind: 'text', content: 'open' }] } as never,
      render: (n: { kind: string; content?: string }, key?: string | number) => createElement('b', { key, 'data-child': n.kind }, n.content),
    })))

  test('draws them in the tile, under the caption', () => {
    const out = html()
    expect(out).toContain('data-kpi-footer')
    expect(out).toContain('<b data-child="text">open</b>')
    expect(out).toContain('data-view="deck-needs-tile"')
    expect(out.indexOf('data-view="deck-needs-tile"')).toBeLessThan(out.indexOf('data-kpi-footer'))
  })

  test('the page schema declares them', () => {
    const kpi = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, unknown> }> }).definitions.KPI
    expect(kpi.properties.children).toEqual({ type: 'array', items: { $ref: '#' } })
  })
})
