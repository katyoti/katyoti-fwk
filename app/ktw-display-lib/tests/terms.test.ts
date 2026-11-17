// SPDX-License-Identifier: Apache-2.0
// The terms a search was run with as editable chips - one
// removed, one added (once, never empty) - and « Chercher encore » running the
// node's action with `${terms}` the terms shown.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { addTerm, removeTerm, resolveAction, termsEvent } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown, data?: unknown, onAction: unknown = () => {}) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data, onAction: onAction as never })))

const action = { uri: 'ktw-example:studies/propose', args: { id: 'etude-erp', terms: '${terms}' } }
const node = { kind: 'terms', id: 'search', terms: ['Arbor', 'facturation', 'TVA'], action,
  note: 'Termes déduits du titre, du périmètre et du contexte.' }

describe('the search terms node', () => {
  test('is a declared interactive kind, its action required', () => {
    expect(INTERACTIVE_KINDS).toContain('terms')
    expect((UIPageNodeJSONSchema as any).definitions.Terms.required).toEqual(['kind', 'action'])
  })

  test('a term is added once, never empty; one is removed', () => {
    expect(addTerm(['Arbor'], '  facture  ')).toEqual(['Arbor', 'facture'])
    expect(addTerm(['Arbor'], 'arbor')).toEqual(['Arbor'])
    expect(addTerm(['Arbor'], '   ')).toEqual(['Arbor'])
    expect(addTerm([], 'marge   Belgique')).toEqual(['marge Belgique'])
    expect(removeTerm(['Arbor', 'TVA'], 'Arbor')).toEqual(['TVA'])
  })

  test('« Chercher encore » runs the action with the terms shown, as a list', () => {
    const e = termsEvent(action, undefined, ['Arbor', 'EAI'])
    expect(e.extra).toEqual({ terms: ['Arbor', 'EAI'] })
    const resolved = resolveAction(e.action, e.item, e.extra)
    expect(resolved.args).toEqual({ id: 'etude-erp', terms: ['Arbor', 'EAI'] })
  })

  test('draws its title, each term with « × », the box to add one, its note and « Chercher encore »', () => {
    const html = page(node)
    expect(html).toContain('Recherche faite avec')
    for (const t of ['Arbor', 'facturation', 'TVA']) expect(html).toContain(`data-guide-name="${t}"`)
    expect(html).toContain('aria-label="Retirer Arbor"')
    expect(html).toContain('Ajouter un terme')
    expect(html).toContain('Termes déduits du titre')
    expect(html).toContain('Chercher encore')
    const again = html.slice(html.indexOf('data-guide="terms.again"') - 200, html.indexOf('data-guide="terms.again"'))
    expect(again).not.toContain('disabled')
  })

  test('without a host or without terms, « Chercher encore » cannot run', () => {
    const noHost = renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never })))
    const at = noHost.indexOf('data-guide="terms.again"')
    expect(noHost.slice(at - 200, at)).toContain('disabled')
    const empty = page({ ...node, terms: [] })
    expect(empty).toContain('Aucun terme')
    const at2 = empty.indexOf('data-guide="terms.again"')
    expect(empty.slice(at2 - 200, at2)).toContain('disabled')
  })

  test('its terms bind from data as strings', () => {
    const bound = applyDataBinding({ kind: 'terms', dataField: '$.search.terms', action } as never, { search: { terms: ['Arbor', 3] } }) as any
    expect(bound.terms).toEqual(['Arbor', '3'])
  })
})

describe('the search terms node sends what is typed, runs once, keeps each chip apart', () => {
  test('« Chercher encore » runs with the term being typed, not yet confirmed', async () => {
    const { termsToRun } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    expect(termsToRun(['Arbor'], ' EAI ')).toEqual(['Arbor', 'EAI'])
    expect(termsToRun(['Arbor'], 'arbor')).toEqual(['Arbor'])
    expect(termsToRun([], '')).toEqual([])
  })
  test('bound terms are drawn once each, case and spaces aside', async () => {
    const { uniqueTerms } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    expect(uniqueTerms(['Arbor', 'arbor', ' TVA ', '', 3, null])).toEqual(['Arbor', 'TVA', '3'])
    const html = page({ ...node, terms: ['Arbor', 'ARBOR', 'TVA'] })
    expect(html.split('data-guide-name="Arbor"').length - 1).toBe(1)
    expect(html).not.toContain('data-guide-name="ARBOR"')
  })
  test('a disabled node shows its terms and never runs', () => {
    const html = page({ ...node, disabled: true })
    expect(html).toContain('data-guide-name="Arbor"')
    const at = html.indexOf('data-guide="terms.again"')
    expect(html.slice(at - 200, at)).toContain('disabled')
    expect((UIPageNodeJSONSchema as any).definitions.Terms.properties.disabled.type).toBe('boolean')
  })
})

describe('where a terms node stands', () => {
  test('a run waits until the producer sends other terms - never ended by a redraw', async () => {
    const { termsInit, termsReduce } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    let s = termsInit(['Arbor', 'TVA'])
    s = termsReduce(s, { type: 'run', terms: ['Arbor', 'TVA', 'EAI'] })
    expect(s.busy).toBe(true)
    expect(s.terms).toEqual(['Arbor', 'TVA', 'EAI'])
    // a redraw sends the same bound terms: nothing changes, the run still waits
    const redraw = termsReduce(s, { type: 'bound', terms: ['Arbor', 'TVA'] })
    expect(redraw).toBe(s)
    // a second run while it waits is refused
    expect(termsReduce(s, { type: 'run', terms: ['x'] })).toBe(s)
    expect(termsReduce(s, { type: 'edit', terms: ['x'] })).toBe(s)
    // the producer's answer ends it and its terms are shown
    const answered = termsReduce(s, { type: 'bound', terms: ['Arbor', 'TVA', 'EAI', 'facture'] })
    expect(answered.busy).toBe(false)
    expect(answered.terms).toEqual(['Arbor', 'TVA', 'EAI', 'facture'])
    expect(answered.replaced).toBe(false)
  })
  test('an answer with the same terms ends the run at its timeout only', async () => {
    const { termsInit, termsReduce } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    const s = termsReduce(termsInit(['Arbor']), { type: 'run', terms: ['Arbor'] })
    expect(termsReduce(s, { type: 'bound', terms: ['Arbor'] }).busy).toBe(true)
    expect(termsReduce(s, { type: 'timeout' }).busy).toBe(false)
  })
  test('terms the producer changes replace an edit not run, and say so', async () => {
    const { termsInit, termsReduce } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    const edited = termsReduce(termsInit(['Arbor']), { type: 'edit', terms: ['Arbor', 'TVA'] })
    expect(edited.edited).toBe(true)
    const replaced = termsReduce(edited, { type: 'bound', terms: ['Arbor', 'marge'] })
    expect(replaced.terms).toEqual(['Arbor', 'marge'])
    expect(replaced.replaced).toBe(true)
    const again = termsReduce(replaced, { type: 'edit', terms: ['marge'] })
    expect(again.replaced).toBe(false)
    expect(termsReduce(termsInit(['a']), { type: 'bound', terms: ['b'] }).replaced).toBe(false) // nothing edited
  })
  test('nothing to run is no run', async () => {
    const { termsInit, termsReduce } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    const s = termsInit([])
    expect(termsReduce(s, { type: 'run', terms: [] })).toBe(s)
  })
})

describe('a terms run the page runs itself never waits', () => {
  test('a run that does not wait sets its terms at once, a host run waits', async () => {
    const { termsInit, termsReduce } = await import('../src/ui-builders/page/ui-page.interactive.ts')
    const s = termsReduce(termsInit(['Arbor']), { type: 'run', terms: ['Arbor', 'EAI'], wait: false })
    expect(s.busy).toBe(false)
    expect(s.terms).toEqual(['Arbor', 'EAI'])
    expect(termsReduce(s, { type: 'run', terms: ['EAI'], wait: false }).terms).toEqual(['EAI']) // again at once
    expect(termsReduce(termsInit(['Arbor']), { type: 'run', terms: ['Arbor'] }).busy).toBe(true)
  })
  test("the node's select action is the page's own, a method is not", async () => {
    const { pageOwn } = await import('../src/ui-builders/page/page-context.tsx')
    expect(pageOwn({ select: { key: 'terms', value: '${terms}' } })).toBe(true)
    expect(pageOwn({ uri: 'x:studies/propose' })).toBe(false)
  })
})
