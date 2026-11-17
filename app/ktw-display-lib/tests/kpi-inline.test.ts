// SPDX-License-Identifier: Apache-2.0
// The inline KPI tile and the outlined button: the deck's
// « Needs you » tile drawn as request-rows-mockup.html - the label bold with
// the count on its line, no icon, a soft ring in its colour and no coloured
// top; its « Ouvrir À faire » button outlined in a soft line of its colour.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { KpiTile, kpiTileStyle, softLine } from '../src/ui-core/components/other/index.ts'
import { KPIRenderer } from '../src/ui-builders/page/nodes/atomic.tsx'
import { ButtonRenderer, outlineButtonStyle } from '../src/ui-builders/page/nodes/interactive.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

const html = (el: ReturnType<typeof createElement>) => renderToStaticMarkup(createElement(MantineProvider, { forceColorScheme: 'dark' }, el))

const between = (out: string) => out.slice(out.indexOf('Needs you'), out.indexOf('>3<'))

describe('an inline KPI tile', () => {
  const tile = (variant?: 'inline') => html(createElement(KpiTile, { label: 'Needs you', value: 3, description: '2 intakes', color: 'grape', icon: 'hand-stop', variant }))

  test('draws the label bold and the count on the same line, with no icon', () => {
    const out = tile('inline')
    expect(out).toContain('data-kpi-variant="inline"')
    // one row holds both: no other row opens between the label and the count
    expect(between(out)).not.toContain('mantine-Group-root')
    expect(out).not.toContain('<svg')
    expect(out).not.toContain('text-transform:uppercase')
  })

  test('the ordinary tile keeps its icon, its uppercase label and the count on a row of its own', () => {
    const out = tile()
    expect(out).not.toContain('data-kpi-variant')
    expect(between(out)).toContain('mantine-Group-root')
    expect(out).toContain('<svg')
  })

  test('is ringed with a soft line in its colour, with no coloured top', () => {
    const s = kpiTileStyle('grape', false, 'inline')
    expect(s.border).toBe(`1px solid ${softLine('grape')}`)
    expect(s.borderTop).toBeUndefined()
    expect(kpiTileStyle('grape').borderTop).toBe('3px solid var(--mantine-color-grape-6)')
  })

  test('the page node chooses it and the schema declares it', () => {
    const out = html(createElement(KPIRenderer, {
      node: { kind: 'kpi', label: 'Needs you', value: 3, color: 'grape', variant: 'inline', 'data-view': 'deck-needs-tile', children: [{ kind: 'text', content: 'open' }] } as never,
      render: (n: { kind: string; content?: string }, key?: string | number) => createElement('b', { key }, n.content),
    }))
    expect(out).toContain('data-kpi-variant="inline"')
    expect(out).toContain('data-view="deck-needs-tile"')
    expect(out).toContain('data-kpi-footer')
    const kpi = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, unknown> }> }).definitions.KPI
    expect(kpi.properties.variant).toEqual({ enum: ['inline'] })
  })
})

describe('an outlined button', () => {
  test('is lined softly in its colour, its label a light shade of it', () => {
    const s = outlineButtonStyle('grape')!
    expect(s['--button-bd']).toBe(`1px solid ${softLine('grape')}`)
    expect(s['--button-color']).toContain('var(--mantine-color-grape-3)')
    expect(outlineButtonStyle(undefined)).toBeUndefined()
  })

  test('the page button draws it, other variants untouched', () => {
    const btn = (variant: string) => html(createElement(ButtonRenderer, {
      node: { kind: 'button', label: 'Ouvrir À faire', variant, color: 'grape', 'data-view': 'deck-needs-open', action: { label: 'Ouvrir À faire', screen: 'todo' } } as never,
    }))
    expect(btn('outline')).toContain('--button-bd:1px solid color-mix(in srgb, var(--mantine-color-grape-6) 45%, transparent)')
    expect(btn('outline')).toContain('--button-color:light-dark(var(--mantine-color-grape-7), var(--mantine-color-grape-3))')
    expect(btn('light')).not.toContain('color-mix')
  })
})
