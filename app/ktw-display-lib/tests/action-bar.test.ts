// SPDX-License-Identifier: Apache-2.0
// A sticky bottom action bar whose summary is computed live
// from the page's working data - the items ticked, the sources, the sections,
// an estimate - and whose buttons run with the summary's values.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { barCount, barSummary, resolveAction } from '../src/ui-builders/page/ui-page.interactive.ts'
import { actionDisabled, actionLabel } from '../src/ui-builders/page/nodes/interactive.tsx'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data, onAction: (() => {}) as never, hasHost: true } as never)))

const study = { kind: 'action-bar', id: 'go', showWhen: 'items', parts: [
  { id: 'items', count: '$.selected', label: 'éléments retenus', strong: true, required: 'Retenez au moins un élément.' },
  { id: 'sources', count: '$.sources', label: '{n} sources' },
  { id: 'sections', count: '$.sections', where: { ticked: true }, label: 'sections et schémas', strong: true },
  { id: 'minutes', count: '$.sections', where: { ticked: true }, per: 1.7, min: 6, label: 'environ {n} min' },
], actions: [
  { id: 'abandon', label: 'Abandonner', uri: 'ktw-example:studies/abandon', args: { id: 'etude-erp' } },
  { id: 'run', label: "Faire l'étude", uri: 'ktw-example:studies/run', args: { id: 'etude-erp', sections: '${item.sections}' }, disabledWhen: 'missing' },
] }

const data = {
  selected: [{ key: 'a' }, { key: 'b' }, { key: 'c' }],
  sources: 3,
  sections: [{ id: '00', ticked: true }, { id: '01', ticked: true }, { id: '02', ticked: false }, { id: '03', ticked: true }, { id: '04', ticked: true }, { id: '05', ticked: true }],
}

describe('the action bar', () => {
  test('is a declared interactive kind of the page grammar, its parts required', () => {
    expect(INTERACTIVE_KINDS).toContain('action-bar')
    expect((UIPageNodeJSONSchema as any).definitions.ActionBar.required).toEqual(['kind', 'parts'])
  })

  test('a part counts an array, filtered by where, or takes a number', () => {
    expect(barCount(data, { count: '$.selected' })).toBe(3)
    expect(barCount(data, { count: '$.sections', where: { ticked: true } })).toBe(5)
    expect(barCount(data, { count: '$.sources' })).toBe(3)
    expect(barCount(data, { count: '$.nothing' })).toBe(0)
  })

  test('the summary is computed from the data: counts, an estimate at least its minimum, the first missing reason', () => {
    const { values, shown } = barSummary(study as never, data)
    expect(values).toEqual({ items: 3, sources: 3, sections: 5, minutes: 9 })
    expect(shown.map((p) => `${p.before}${p.value}${p.after}`)).toEqual(['3 éléments retenus', '3 sources', '5 sections et schémas', 'environ 9 min'])
    const few = barSummary(study as never, { ...data, sections: [{ ticked: true }] }).values
    expect(few.minutes).toBe(6)
    const none = barSummary(study as never, { ...data, selected: [] }).values
    expect(none.missing).toBe('Retenez au moins un élément.')
  })

  test('the bar follows the data it reads: other ticks, another summary', () => {
    const html = page({ kind: 'vbox', children: [study] }, data)
    expect(html).toContain('data-element="action-bar:go"')
    expect(html).toContain('position:sticky')
    expect(html.replace(/<[^>]+>/g, '')).toContain('3 éléments retenus · 3 sources · 5 sections et schémas · environ 9 min')
    const more = page({ kind: 'vbox', children: [study] }, { ...data, selected: [...data.selected, { key: 'd' }] })
    expect(more.replace(/<[^>]+>/g, '')).toContain('4 éléments retenus')
  })

  test('it shows only once its showWhen part is not 0', () => {
    expect(page({ kind: 'vbox', children: [study] }, { ...data, selected: [] })).not.toContain('action-bar:go')
  })

  test('its buttons run with the summary as their element, the main one waiting while a required part is 0', () => {
    const { values } = barSummary(study as never, data)
    const run = study.actions[1]
    expect(resolveAction(run as never, values).args).toEqual({ id: 'etude-erp', sections: 5 })
    expect(actionDisabled(run as never, values)).toBeUndefined()
    const none = barSummary(study as never, { ...data, selected: [] }).values
    expect(actionDisabled(run as never, none)).toBe('Retenez au moins un élément.')
    expect(actionLabel('Faire ${item.sections} sections', values)).toBe('Faire 5 sections')
  })
})
