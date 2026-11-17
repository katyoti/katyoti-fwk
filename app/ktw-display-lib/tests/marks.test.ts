// SPDX-License-Identifier: Apache-2.0
// The page grammar draws a two-part chip (kind + value), a
// coloured letter monogram, a dashed callout tone, a data-bound tab badge
// with a tone, and data-bound stepper step labels.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { ATOMIC_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { boundValue, monogramLetters } from '../src/ui-builders/page/nodes/marks.ts'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data })))
const text = (html: string) => html.replace(/<style[^>]*>[\s\S]*?<\/style>/g, '').replace(/<[^>]+>/g, '')

describe('the page grammar marks', () => {
  test('a two-part chip shows the kind, muted, before the value', () => {
    const html = page({ kind: 'badge', text: 'Arbor', prefix: 'application' })
    expect(html).toContain('data-guide="badge.prefix"')
    expect(text(html)).toContain('applicationArbor')
    expect(html).toContain('<b>Arbor</b>')
    expect(page({ kind: 'badge', text: 'Arbor' })).not.toContain('badge.prefix')
  })

  test('a monogram draws one or two letters on a square of its colour', () => {
    expect(ATOMIC_KINDS).toContain('monogram')
    expect((UIPageNodeJSONSchema as any).definitions.Monogram.required).toEqual(['kind', 'text'])
    expect(monogramLetters('LeanIX')).toBe('LE')
    expect(monogramLetters('Confluence Cloud')).toBe('CC')
    expect(monogramLetters('dynatrace-entities')).toBe('DE')
    expect(monogramLetters('Cf')).toBe('CF')
    const html = page({ kind: 'monogram', text: 'Confluence Cloud', color: 'blue' })
    expect(html).toContain('data-guide="monogram"')
    expect(text(html)).toBe('CC')
    expect(html).toContain('blue')
  })

  test('a callout box takes its tone, its border dashed when asked', () => {
    const dashed = page({ kind: 'box', callout: 'yellow', dashed: true, children: [{ kind: 'text', content: 'Une remarque en attente' }] })
    expect(dashed).toContain('data-guide-name="yellow-dashed"')
    expect(dashed).toContain('1px dashed var(--mantine-color-yellow-5)')
    const solid = page({ kind: 'box', callout: 'blue', children: [] })
    expect(solid).toContain('1px solid var(--mantine-color-blue-5)')
    expect(page({ kind: 'box', children: [] })).not.toContain('data-guide="callout"')
  })

  test('a tab badge follows the page data, in its tone', () => {
    const tabs = { kind: 'tabs', tabs: [
      { id: 'sources', label: 'Sources', badgeField: '$.counts.sources', badgeColor: 'red', content: { kind: 'text', content: 'x' } },
      { id: 'etude', label: 'Étude', badge: 'nouveau', content: { kind: 'text', content: 'y' } },
    ] }
    const html = page(tabs, { counts: { sources: 12 } })
    expect(html).toMatch(/data-guide="tab.badge" data-guide-name="sources"[^>]*><span[^>]*>12</)
    expect(html).toContain('--mantine-color-red-light')
    expect(html).toContain('nouveau')
    expect(page(tabs, { counts: { sources: 3 } })).toMatch(/data-guide-name="sources"[^>]*><span[^>]*>3</)
    expect(page(tabs, {})).not.toContain('data-guide-name="sources"')
    expect(boundValue({ a: 0 }, '$.a', 'x')).toBe(0)
    expect(boundValue({}, '$.a', 'x')).toBe('x')
  })

  test('a stepper step label follows the page data once set', () => {
    const stepper = { kind: 'stepper', current: 'sources', steps: [
      { id: 'subject', label: 'Sujet' },
      { id: 'sources', label: 'Sources proposées', labelField: '$.labels.sources' },
    ] }
    expect(text(page(stepper, { labels: { sources: 'Sources proposées (12)' } }))).toContain('Sources proposées (12)')
    expect(text(page(stepper, {}))).toContain('Sources proposées')
    expect(text(page(stepper, {}))).not.toContain('(12)')
  })
})
