// SPDX-License-Identifier: Apache-2.0
// A create page's controls: one-choice cards setting a key of
// the page selection - a radio group, a badge in its colour, a disabled card
// with its pill and why -, a labelled text field bound to a key, prefilled
// and shown only while another key holds a value, and a button disabled until
// the keys it requires are set.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { choiceSets, isSet, missingKeys, shownWhen } from '../src/ui-builders/page/nodes/choice-cards.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { ALL_KINDS } from '../src/ui-builders/page/ui-page.types.ts'

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const page = (node: unknown, data: unknown = {}) =>
  html(createElement(UIPageView, { node: node as never, data, onAction: () => {} }))

const cards = {
  kind: 'choice-cards', id: 'connector', key: 'connector', label: 'Connecteur',
  options: [
    { value: 'leanix-application', label: 'LeanIX', color: 'teal', badge: 'LX', description: 'Applications de LeanIX', sets: { name: 'LeanIX' } },
    { value: 'confluence', label: 'Confluence', color: 'blue', description: 'Pages Confluence', sets: { name: 'Confluence' } },
    { value: 'documents', label: 'Documents', disabled: true, pill: 'déjà dans l’étude', tooltip: 'Une seule par étude' },
  ],
}

describe('choice cards', () => {
  test('a radio group, one card per option with its badge; the chosen one checked', () => {
    const out = page(cards, { selection: { connector: 'confluence' } })
    expect(out).toContain('role="radiogroup"')
    expect(out).toContain('data-element="choice:connector"')
    expect(out).toMatch(/role="radio" aria-checked="true"[^>]*data-value="confluence"/)
    expect(out).toMatch(/role="radio" aria-checked="false"[^>]*data-value="leanix-application"/)
    expect(out).toContain('>LX<')
    expect(out).toContain('>CO<')
    expect(out).toContain('data-element="choice:connector:confluence"')
  })

  test('a chosen card sets the other keys it names, each only while the person has not changed it', () => {
    const [leanix, confluence] = cards.options
    expect(choiceSets(leanix, undefined, {})).toEqual({ name: 'LeanIX' })
    expect(choiceSets(confluence, leanix, { name: 'LeanIX' })).toEqual({ name: 'Confluence' })
    expect(choiceSets(confluence, leanix, { name: 'Mon wiki' })).toEqual({})
    expect(choiceSets(cards.options[2], undefined, {})).toEqual({})
  })

  test('a disabled card cannot be chosen: disabled, its pill drawn, never checked', () => {
    const out = page(cards, { selection: { connector: 'documents' } })
    expect(out).toMatch(/role="radio" aria-checked="false" aria-disabled="true" disabled=""[^>]*data-value="documents"/)
    expect(out).toContain('déjà dans l’étude')
    expect(out).not.toContain('aria-checked="true"')
  })

  test('bound by dataField, the options come from the data', () => {
    const out = page({ kind: 'choice-cards', key: 'c', dataField: 'connectors' }, { connectors: [{ value: 'x', label: 'Xylo' }] })
    expect(out).toContain('data-value="x"')
    expect(out).toContain('Xylo')
  })
})

describe('a text-field', () => {
  const name = { kind: 'text-field', key: 'name', label: 'Nom', value: 'LeanIX', required: true }
  const space = { kind: 'text-field', key: 'space', label: 'Espace', when: 'connector=confluence' }

  test('labelled, its starting value shown while the selection holds none', () => {
    const out = page(name)
    expect(out).toContain('Nom')
    expect(out).toContain('value="LeanIX"')
    expect(out).toContain('data-ktw-field="name"')
    expect(out).toContain('data-element="field:name"')
    expect(page(name, { selection: { name: 'Mon LeanIX' } })).toContain('value="Mon LeanIX"')
  })

  test('drawn only while its when holds', () => {
    expect(page(space, { selection: { connector: 'leanix-application' } })).not.toContain('data-ktw-field="space"')
    expect(page(space, { selection: { connector: 'confluence' } })).toContain('data-ktw-field="space"')
    expect(shownWhen(undefined, {})).toBe(true)
    expect(shownWhen('connector', {})).toBe(false)
    expect(shownWhen('connector', { connector: 'x' })).toBe(true)
  })

  test('any node with a when is drawn only while it holds; one without is always drawn', () => {
    const note = { kind: 'vbox', children: [
      { kind: 'text', id: 'note-lx', content: 'Elle propose des applications', when: 'connector=leanix-application' },
      { kind: 'text', id: 'note-cf', content: 'Elle propose des pages', when: 'connector=confluence' },
      { kind: 'text', id: 'always', content: 'Toujours là' },
    ] }
    const lx = page(note, { selection: { connector: 'leanix-application' } })
    expect(lx).toContain('Elle propose des applications')
    expect(lx).not.toContain('Elle propose des pages')
    expect(lx).toContain('Toujours là')
    const none = page(note)
    expect(none).not.toContain('Elle propose')
    expect(none).toContain('Toujours là')
  })

  test('given rows, a multi-line text area showing what is typed; without, a single-line input', () => {
    const remark = { kind: 'text-field', key: 'remark', label: 'Remarque', rows: 4 }
    const out = page(remark, { selection: { remark: 'Plus court' } })
    expect(out).toMatch(/<textarea[^>]*data-element="field:remark"/)
    expect(out).toContain('Plus court</textarea>')
    expect(page(name)).not.toContain('<textarea')
    expect(page(name)).toMatch(/<input[^>]*data-element="field:name"/)
  })

  test('with its values given, a drop-down of them showing the one chosen', () => {
    const spaces = { kind: 'text-field', key: 'scope', label: 'Espace', value: 'ARCHI', options: [{ value: 'ARCHI', label: 'ARCHI · Architecture SI' }, { value: 'PROJ' }] }
    const out = page(spaces)
    expect(out).toContain('data-element="field:scope"')
    expect(out).toContain('value="ARCHI · Architecture SI"')
    expect(out).toContain('readonly=""')
  })
})

describe('a text-field and a button requiring its keys', () => {
  const create = { kind: 'button', id: 'create', label: 'Créer', requires: ['connector', 'name'], requiresHint: 'Choisissez un connecteur et un nom',
    action: { uri: 'ktw-agents:sources/use', args: { connector: '${selection.connector}', name: '${selection.name}' } } }

  test('disabled until every required key is set, then enabled', () => {
    expect(page(create, { selection: { connector: 'confluence' } })).toMatch(/disabled=""[^>]*data-missing="name"/)
    const on = page(create, { selection: { connector: 'confluence', name: 'Wiki' } })
    expect(on).toContain('data-element="button:create"')
    expect(on).not.toContain('data-missing')
    expect(on).not.toMatch(/disabled=""[^>]*data-element="button:create"/)
  })

  test('a blank value is not set', () => {
    expect(isSet('  ')).toBe(false)
    expect(isSet(false)).toBe(false)
    expect(missingKeys(['a', 'b'], { a: 'x', b: ' ' })).toEqual(['b'])
    expect(missingKeys(undefined, {})).toEqual([])
  })

  test('a button draws its icon before its label, or after it with iconPosition right', () => {
    const next = { kind: 'button', label: 'Suivante', icon: 'chevron-right', action: { uri: 'u' } }
    const before = page(next)
    expect(before).toContain('data-position="left"')
    expect(before).not.toContain('data-position="right"')
    const after = page({ ...next, iconPosition: 'right' })
    expect(after).toContain('data-position="right"')
    expect(after).not.toContain('data-position="left"')
    expect(after.indexOf('Suivante')).toBeLessThan(after.indexOf('data-position="right"'))
  })

  test('choice-cards and text-field are page kinds', () => {
    for (const k of ['choice-cards', 'text-field']) expect(ALL_KINDS).toContain(k as never)
  })
})
