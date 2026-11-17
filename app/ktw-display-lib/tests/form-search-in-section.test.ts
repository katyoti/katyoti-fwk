// SPDX-License-Identifier: Apache-2.0
// The one form of a study searches its sources in a
// loaded section - (1) its « Chercher les sources », a form ui-action with no
// uri and a select, sets the page selection from the form's value; (2) the
// source-search panel drawn in the loaded section watches the form of the
// page holding the section, so a perimeter changed after the search says the
// proposals are out of date; (3) the section read again (« Chercher
// encore ») keeps the panel's unticks and is the new search the notice
// compares with.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { setRendererLookup, type FieldRendererFactory } from '../src/ui-builders/fields/index.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { formActionProcessor } from '../src/ui-builders/page/nodes/schema-bound.tsx'
import { enclosingDrafts, enclosingPage, routePageAction, type PageContext } from '../src/ui-builders/page/page-context.tsx'
import {
  SOURCE_SEARCH_EMPTY, sourceSearchReduce, sourceSearchStale, sourceSearchValue, watchedValues,
  type UIPageAction, type UISourceAnswer,
} from '../src/ui-builders/page/ui-page.interactive.ts'
import type { Schema } from '../src/schema/index.ts'

const stubFactory: FieldRendererFactory = { view: () => () => null, form: () => () => null, header: () => () => null, column: () => () => null }
setRendererLookup(() => stubFactory)

const STUDY_NEW = 'ktw-example:example/study-new'

// The form as the study lays it: « Chercher les sources » sets the
// page selection's `search` from the form's value; no method runs.
const schema = {
  name: 'study-new',
  root: 'Study',
  types: {
    Study: {
      type: 'object',
      fields: { name: { type: 'string' }, perimeter_tags: { type: 'array', items: 'string' } },
      extensions: [{
        type: 'ui-form', name: 'default', actions: [
          { type: 'ui-action', id: 'cancel', scope: 'item', label: 'Annuler' },
          { type: 'ui-action', id: 'search-sources', scope: 'item', label: 'Chercher les sources',
            select: { key: 'search', value: { name: '${value.name}', perimeter_tags: '${value.perimeter_tags}' } } },
          { type: 'ui-action', id: 'submit', scope: 'item', label: 'Create', uri: STUDY_NEW },
          { type: 'ui-action', id: 'nothing', scope: 'item', label: 'No verb' },
        ],
      }],
    },
  },
} as unknown as Schema

/** A page as UIPageView runs a form's action: routed where it belongs, a
 *  select setting the selection, any other handed to the host. */
function page() {
  const selection: Record<string, unknown> = {}
  const host: UIPageAction[] = []
  const runOf = (action: UIPageAction | undefined) => action
    ? (item?: unknown, extra?: Record<string, unknown>) => routePageAction({ action, item, extra }, {
      show: () => {}, select: (k, v) => { selection[k] = v }, host: (e) => host.push(e.action), selection,
    })
    : undefined
  return { selection, host, runOf }
}

const lx = (s: Partial<UISourceAnswer>): UISourceAnswer => ({ connector: 'leanix', label: 'LeanIX', state: 'found', ...s })
const cf = (s: Partial<UISourceAnswer>): UISourceAnswer => ({ connector: 'confluence', label: 'Confluence', state: 'found', ...s })
const arbor = { key: 'app-arbor', title: 'Arbor', why: '« Arbor » dans le nom' }
const eai = { key: 'app-eai', title: 'EAI Flux', why: 'interface de Arbor' }
const tva = { key: 'page-tva', title: 'Règles TVA', why: '« TVA »' }
const fresh = { key: 'app-new', title: 'New', why: 'TVA' }

describe('the one form searches its sources in a loaded section', () => {
  test('a form ui-action with no uri and a select sets the page selection from the form value', async () => {
    const { selection, host, runOf } = page()
    const form = buildUIForms({ schema })['default']
    const processor = formActionProcessor(form.definition.actions, runOf)
    expect(processor).toBeDefined()
    const value = { name: 'Arbor', perimeter_tags: ['facturation', 'TVA'] }
    await processor!({ action: form.actions.find((a) => a.id === 'search-sources')!, item: value })
    expect(selection.search).toEqual({ name: 'Arbor', perimeter_tags: ['facturation', 'TVA'] })
    expect(host.length).toBe(0)
    // The form's own cancel and an action naming nothing run nothing.
    await processor!({ action: form.actions.find((a) => a.id === 'nothing')!, item: value })
    await processor!({ action: { id: 'cancel', scope: 'item' } as never, item: value })
    expect(Object.keys(selection)).toEqual(['search'])
    expect(host.length).toBe(0)
  })

  test('a panel in a loaded section watches the form of the page holding the section', () => {
    const watch = ['value.name', 'value.perimeter_tags']
    // The page holding the form and the section, the section's own page
    // inside it: its data is the section's answer, no form value in it.
    const form: PageContext = { draft: { value: { name: 'Arbor', perimeter_tags: ['facturation'] } } }
    const section: PageContext = { draft: { selection: {} }, enclosing: enclosingPage(form) }
    expect(enclosingPage({})).toBeUndefined()
    expect(enclosingDrafts(section)).toEqual([form.draft])
    const ran = watchedValues(watch, section.draft, enclosingDrafts(section))
    expect(ran).toBe('["Arbor",["facturation"]]')
    // The section's answer: every source answered at once - a search.
    const st = sourceSearchReduce(SOURCE_SEARCH_EMPTY, { type: 'answers', sources: [lx({ items: [arbor, eai] }), cf({ items: [tva] })], watched: ran })
    expect(st.ranWith).toBe(ran)
    expect(sourceSearchStale(st, ran)).toBe(false)
    // The perimeter changed in the form: the proposals are out of date.
    const changed: PageContext = { draft: { value: { name: 'Arbor', perimeter_tags: ['facturation', 'TVA'] } } }
    const now = watchedValues(watch, section.draft, enclosingDrafts({ ...section, enclosing: changed }))
    expect(sourceSearchStale(st, now)).toBe(true)
    // A path the section's own data holds is read there first.
    expect(watchedValues(['value.name'], { value: { name: 'own' } }, [form.draft])).toBe('["own"]')
  })

  test('a section read again keeps the panel unticks and is the search the notice compares with', () => {
    const ran = '["Arbor",["facturation"]]'
    let st = sourceSearchReduce(SOURCE_SEARCH_EMPTY, { type: 'answers', sources: [lx({ items: [arbor, eai] }), cf({ items: [tva] })], watched: ran })
    st = sourceSearchReduce(st, { type: 'tick', connector: 'leanix', key: 'app-eai', on: false })
    st = sourceSearchReduce(st, { type: 'tick', connector: 'confluence', key: 'page-tva', on: false })
    const now = '["Arbor",["facturation","TVA"]]'
    expect(sourceSearchStale(st, now)).toBe(true)
    // « Chercher encore »: the section read again answers every source again.
    st = sourceSearchReduce(st, { type: 'answers', sources: [lx({ items: [arbor, eai, fresh] }), cf({ items: [tva] })], watched: now })
    expect(sourceSearchValue(st).selected).toEqual([{ connector: 'leanix', items: [arbor, fresh] }])
    expect(st.ranWith).toBe(now)
    expect(sourceSearchStale(st, now)).toBe(false)
    // A « Réessayer » read again is no new search: the values it compares with stay.
    st = sourceSearchReduce(st, { type: 'retry', connector: 'confluence' })
    st = sourceSearchReduce(st, { type: 'answers', sources: [lx({ items: [arbor, eai] }), cf({ items: [tva] })], watched: '["other"]' })
    expect(st.ranWith).toBe(now)
    expect(st.retrying).toEqual([])
  })

  test('the page gives its pages the page around them; the panel reads its watch through them', () => {
    const src = (f: string) => readFileSync(join(import.meta.dir, '..', 'src', 'ui-builders', 'page', ...f.split('/')), 'utf8')
    expect(src('ui-page.view.tsx')).toContain('const enclosing = enclosingPage(useContext(DataContext))')
    expect((src('ui-page.view.tsx').match(/hasHost: !!onAction, enclosing \}/g) ?? []).length).toBe(3)
    expect(src('nodes/source-search.tsx')).toContain('watchedValues(node.watch, page.draft, enclosingDrafts(page))')
  })
})

describe('a page-own select in a loaded section reads the form around it', () => {
  // « Chercher encore » / « Réessayer »: a select run from the section's own
  // page, whose data is the section's answer; the form's value is in the
  // working copy of the page holding the section.
  const research: UIPageAction = { id: 'again', select: { key: 'search', value: { name: '${value.name}', perimeter_tags: '${value.perimeter_tags}', only: '${only}' } } }
  const form: PageContext = { draft: { value: { name: 'Paie', perimeter_tags: ['paie', 'RH'] } } }
  const section: PageContext = { draft: { selection: {}, value: {} }, enclosing: enclosingPage(form) }
  const drafts = [section.draft, ...enclosingDrafts(section)]

  test('a name the event lacks is read in its page, then the pages around it, nearest first', () => {
    const { selection, host } = page()
    routePageAction({ action: research, drafts }, { show: () => {}, select: (k, v) => { selection[k] = v }, host: (e) => host.push(e.action), selection })
    expect(selection.search).toEqual({ name: 'Paie', perimeter_tags: ['paie', 'RH'], only: '' })
    expect(host.length).toBe(0)
    // The section's own copy holding the name wins over the form around it.
    routePageAction({ action: research, drafts: [{ value: { name: 'own' } }, form.draft] }, { show: () => {}, select: (k, v) => { selection[k] = v }, host: () => {}, selection })
    expect((selection.search as Record<string, unknown>).name).toBe('own')
    expect((selection.search as Record<string, unknown>).perimeter_tags).toEqual(['paie', 'RH'])
  })

  test('an extra or the selection of the same name still wins', () => {
    const { selection } = page()
    selection.value = { perimeter_tags: ['from-selection'] }
    const act: UIPageAction = { id: 'retry', select: { key: 'search', value: { name: '${value.name}', tags: '${selection.value.perimeter_tags}', only: '${only}' } } }
    routePageAction({ action: act, extra: { value: { name: 'from-extra' }, only: 'confluence' }, drafts }, { show: () => {}, select: (k, v) => { selection[k] = v }, host: () => {}, selection })
    expect(selection.search).toEqual({ name: 'from-extra', tags: ['from-selection'], only: 'confluence' })
  })

  test('the working copies never reach the host', () => {
    const got: Record<string, unknown>[] = []
    routePageAction({ action: { id: 'open', uri: STUDY_NEW }, drafts }, { show: () => {}, select: () => {}, host: (e) => got.push(e as unknown as Record<string, unknown>), selection: {} })
    expect(got.length).toBe(1)
    expect('drafts' in got[0]).toBe(false)
  })

  test('a node runs a select with its page and the pages around it', () => {
    const src = readFileSync(join(import.meta.dir, '..', 'src', 'ui-builders', 'page', 'page-context.tsx'), 'utf8')
    expect(src).toContain('drafts: [page.draft, ...enclosingDrafts(page)]')
    expect(src).toContain('resolveActionArgs({ value: select.value }, e.item, scope, e.drafts)')
  })
})
