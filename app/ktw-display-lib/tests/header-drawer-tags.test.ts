// SPDX-License-Identifier: Apache-2.0
// An object's header (a stepper of its workflow among the
// page's nodes), an action opening a node or a panel in a drawer, a form
// field holding typed tags looked up through a method, free text allowed.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { UIPageNodeJSONSchema as UI_PAGE_JSON_SCHEMA } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { actionVerb, type UIPageActionEvent } from '../src/ui-builders/page/ui-page.interactive.ts'
import { routePageAction } from '../src/ui-builders/page/page-context.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { stepperActive } from '../src/ui-builders/page/nodes/stepper.tsx'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import TypedTagsRenderer, { addTag, asTags, removeTag, typeOf, TypedTagsInput } from '../src/ui-builders/fields/typed-tags/renderer.tsx'

const steps = [{ id: 'prepare', label: 'Préparer' }, { id: 'generate', label: 'Générer' }, { id: 'review', label: 'Relire' }, { id: 'publish', label: 'Publier' }]
const wrap = (el: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, el as never))

describe('the stepper node', () => {
  test('is a page kind the schema declares', () => {
    expect(INTERACTIVE_KINDS).toContain('stepper')
    expect(JSON.stringify(UI_PAGE_JSON_SCHEMA)).toContain('"Stepper"')
  })
  test('marks the steps before the current one done, all when completed', () => {
    expect(stepperActive({ steps, current: 'review' })).toBe(2)
    expect(stepperActive({ steps, current: 'nope' })).toBe(0)
    expect(stepperActive({ steps, current: 'review', completed: true })).toBe(4)
  })
  test('binds its current step and draws its steps', () => {
    const bound = applyDataBinding({ kind: 'stepper', steps, dataField: '$.state' } as never, { state: 'generate' })
    expect((bound as { current?: string }).current).toBe('generate')
    const html = wrap(createElement(UIPageView, { node: { kind: 'stepper', steps, current: 'generate' } as never }))
    for (const s of steps) expect(html).toContain(s.label)
  })
})

describe('a drawer action', () => {
  const node = { kind: 'text', content: 'in the drawer' }
  const route = (action: object) => {
    const seen: string[] = []
    routePageAction({ action } as UIPageActionEvent, {
      show: () => seen.push('tab'), select: () => seen.push('select'), host: () => seen.push('host'),
      selection: {}, drawer: () => seen.push('drawer'),
    })
    return seen
  }
  test('is one verb', () => {
    expect(actionVerb({ drawer: { node: node as never } })).toBe('drawer')
    expect(actionVerb({ drawer: { panel: 'cadrage' }, uri: 'x:y/z' })).toBeUndefined()
  })
  test('holding a node is the page\'s own; holding a panel goes to the host', () => {
    expect(route({ drawer: { node } })).toEqual(['drawer'])
    expect(route({ drawer: { panel: 'cadrage' } })).toEqual(['host'])
  })
})

describe('the typed-tags field', () => {
  const types = [{ name: 'application', label: 'Application', color: 'blue' }]
  test('reads tags and free text, adds once, removes', () => {
    const tags = asTags([{ label: 'Arbor', type: 'application', key: 'A1' }, 'facture électronique', 3, { label: '' }])
    expect(tags).toEqual([{ label: 'Arbor', type: 'application', key: 'A1' }, { label: 'facture électronique' }])
    expect(addTag(tags, { label: 'Arbor ERP', type: 'application', key: 'A1' })).toHaveLength(2)
    expect(addTag(tags, { label: 'FACTURE électronique' })).toHaveLength(2)
    expect(addTag(tags, { label: 'flux' })).toHaveLength(3)
    expect(removeTag(tags, 0)).toEqual([{ label: 'facture électronique' }])
  })
  test('names a tag by its declared type', () => {
    expect(typeOf(types, 'application')?.label).toBe('Application')
    expect(typeOf(types, 'domain')).toEqual({ name: 'domain' })
    expect(typeOf(types, undefined)).toBeUndefined()
  })
  test('is a form field, and draws its tags with their type', () => {
    expect(typeof TypedTagsRenderer.form).toBe('function')
    const html = wrap(createElement(HostServicesContext.Provider, { value: {} },
      createElement(TypedTagsInput, { content: { type: 'typed-tags', lookup: 'x:tags/find', types } as never,
        value: [{ label: 'Arbor', type: 'application', key: 'A1' }, { label: 'flux' }], onChange: () => {} })))
    expect(html).toContain('Application · Arbor')
    expect(html).toContain('flux')
    expect(html).toContain('typed-tags.input')
  })
})
