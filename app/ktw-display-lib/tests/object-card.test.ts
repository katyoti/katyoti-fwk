// SPDX-License-Identifier: Apache-2.0
// The one object card: filled from an object's CWM row - its
// title, id, state, the type's declared badges, one next action and a « ⋯ »
// menu -, bound from a list's element, its actions run on the row.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema, UI_KIND_CATALOG } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { objectCardRow } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data })))

const row = {
  id: 'm.G-001',
  title: 'Ship the release',
  type: 'goal',
  state: { id: 'lifecycle.state', label: 'accepted', color: 'green' },
  badges: [{ id: 'plan.priority', label: 'P1', color: 'orange' }],
  next: { id: 'refine', label: 'Affiner', uri: 'ktw-agents:requests/refine', args: { id: '${item.id}' } },
  menu: [
    { id: 'archive', label: 'Retirer', uri: 'ktw-agents:cwm/object-archive', args: { id: '${item.id}' } },
  ],
}

describe('the object card', () => {
  test('draws the row: title, id, state, badges, the next action and the menu', () => {
    const html = page({ kind: 'object-card', row })
    expect(html).toContain('data-element="card:m.G-001"')
    expect(html).toContain('Ship the release')
    expect(html).toContain('m.G-001')
    expect(html).toContain('data-element="card-state:m.G-001"')
    expect(html).toContain('accepted')
    expect(html).toContain('data-element="card-badge:m.G-001:plan.priority"')
    expect(html).toContain('P1')
    expect(html).toContain('data-element="card-next:m.G-001"')
    expect(html).toContain('Affiner')
    expect(html).toContain('data-element="card-menu:m.G-001"')
  })

  test('is drawn for each element of a list, bound to $.item', () => {
    const html = page({
      kind: 'list', layout: 'grid', dataField: 'rows',
      item: { kind: 'object-card', dataField: '$.item' },
    }, { rows: [row, { id: 'm.G-002', title: 'Second' }] })
    expect(html).toContain('data-element="card:m.G-001"')
    expect(html).toContain('data-element="card:m.G-002"')
    expect(html).toContain('Second')
  })

  test('takes no partial row: no id, no card; no title, the id titles it', () => {
    expect(objectCardRow({ title: 'x' })).toBeUndefined()
    expect(objectCardRow(['a'])).toBeUndefined()
    expect(objectCardRow({ id: 'a.F-1' })?.title).toBe('a.F-1')
    expect(page({ kind: 'object-card', row: { title: 'no id' } })).not.toContain('data-element="card:')
    const bound = applyDataBinding({ kind: 'object-card', dataField: 'r' } as never, { r: row }) as { row?: { id: string } }
    expect(bound.row?.id).toBe('m.G-001')
  })

  test('is a declared interactive kind of the DSL', () => {
    expect(INTERACTIVE_KINDS as readonly string[]).toContain('object-card')
    expect(UI_KIND_CATALOG.some((k) => k.kind === 'object-card')).toBe(true)
    expect(JSON.stringify(UIPageNodeJSONSchema)).toContain('#/definitions/ObjectCard')
  })
})
