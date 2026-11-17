// SPDX-License-Identifier: Apache-2.0
// A multi-source search panel draws one group per source as
// it answers - its state, its items each saying why with a tick, « Tout
// cocher », « Réessayer » when unreachable, « Configurer » when not
// configured, « Ajouter depuis <source>… » - and hands what is ticked to the
// page: selected [{connector, items}] and unreachable [{connector, why}].
// The search's per-source states come as
// sources.SearchSources emits them: the whole search each time a source moves.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { SOURCE_SEARCH_LABELS, SourceGroupView, sourceGroupAddable, sourceSearchFoldKey, sourceSearchKey } from '../src/ui-builders/page/nodes/source-search.tsx'
import {
  SOURCE_SEARCH_EMPTY, sourceGroupsShown, sourceSearchReduce, sourceSearchStale, sourceSearchValue, watchedValues,
  type SourceGroup, type SourceSearchState, type UISourceAnswer,
} from '../src/ui-builders/page/ui-page.interactive.ts'

const lx = (s: Partial<UISourceAnswer>): UISourceAnswer => ({ connector: 'leanix', label: 'LeanIX', state: 'searching', ...s })
const cf = (s: Partial<UISourceAnswer>): UISourceAnswer => ({ connector: 'confluence', label: 'Confluence', state: 'searching', ...s })
const dt = (s: Partial<UISourceAnswer>): UISourceAnswer => ({ connector: 'dynatrace', label: 'Dynatrace', state: 'searching', ...s })

const arbor = { key: 'app-arbor', title: 'Arbor', why: '« Arbor » dans le nom' }
const eai = { key: 'app-eai', title: 'EAI Flux', why: 'interface de Arbor' }
const tva = { key: 'page-tva', title: 'Règles TVA', why: '« TVA » et « magasin »' }

/** The stream a search emits: Confluence slow, Dynatrace following LeanIX
 *  then unreachable. */
const stream: UISourceAnswer[][] = [
  [cf({}), lx({}), dt({ state: 'waiting', waiting: 'leanix', waiting_label: 'LeanIX' })],
  [cf({}), lx({ state: 'found', found: 2, items: [arbor, eai] }), dt({ state: 'waiting', waiting: 'leanix', waiting_label: 'LeanIX' })],
  [cf({}), lx({ state: 'found', found: 2, items: [arbor, eai] }), dt({})],
  [cf({}), lx({ state: 'found', found: 2, items: [arbor, eai] }), dt({ state: 'unreachable', message: 'hors réseau ACME' })],
  [cf({ state: 'found', found: 1, items: [tva] }), lx({ state: 'found', found: 2, items: [arbor, eai] }), dt({ state: 'unreachable', message: 'hors réseau ACME' })],
]

function play(answers: UISourceAnswer[][], from: SourceSearchState = SOURCE_SEARCH_EMPTY, watched?: string): SourceSearchState[] {
  const out: SourceSearchState[] = []
  let st = from
  for (const sources of answers) {
    st = sourceSearchReduce(st, { type: 'answers', sources, watched })
    out.push(st)
  }
  return out
}

const shown = (st: SourceSearchState) => sourceGroupsShown(st).map((g) => `${g.answer.connector}:${g.answer.state}`)

const draw = (group: SourceGroup, props: Record<string, unknown> = {}) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(SourceGroupView, { group, labels: SOURCE_SEARCH_LABELS, onTick: () => {}, onAll: () => {}, ...props })))

describe('a multi-source search panel', () => {
  test('the groups appear in answer order with their states, those still searching after', () => {
    const states = play(stream)
    expect(shown(states[0])).toEqual(['confluence:searching', 'leanix:searching', 'dynatrace:waiting'])
    expect(shown(states[1])).toEqual(['leanix:found', 'confluence:searching', 'dynatrace:waiting'])
    expect(shown(states[2])).toEqual(['leanix:found', 'confluence:searching', 'dynatrace:searching'])
    expect(shown(states[3])).toEqual(['leanix:found', 'dynatrace:unreachable', 'confluence:searching'])
    expect(shown(states[4])).toEqual(['leanix:found', 'dynatrace:unreachable', 'confluence:found'])
  })

  test('a found item is ticked when proposed; the panel hands selected and unreachable to the page', () => {
    const end = play(stream)[4]
    expect(sourceSearchValue(end)).toEqual({
      selected: [
        { connector: 'leanix', items: [arbor, eai] },
        { connector: 'confluence', items: [tva] },
      ],
      unreachable: [{ connector: 'dynatrace', why: 'hors réseau ACME' }],
    })
  })

  test('an untick holds through the next emissions; « Tout cocher » ticks the group again', () => {
    let st = play(stream.slice(0, 2))[1]
    st = sourceSearchReduce(st, { type: 'tick', connector: 'leanix', key: 'app-eai', on: false })
    st = play(stream.slice(2), st)[2]
    expect(sourceSearchValue(st).selected[0]).toEqual({ connector: 'leanix', items: [arbor] })
    st = sourceSearchReduce(st, { type: 'all', connector: 'leanix', on: true })
    expect(sourceSearchValue(st).selected[0].items.map((i) => i.key)).toEqual(['app-arbor', 'app-eai'])
    st = sourceSearchReduce(st, { type: 'all', connector: 'leanix', on: false })
    expect(sourceSearchValue(st).selected.map((s) => s.connector)).toEqual(['confluence'])
  })

  test('« Réessayer » re-runs one source alone: the others keep their groups and ticks, the search is no new one', () => {
    const end = play(stream, SOURCE_SEARCH_EMPTY, '["Arbor"]')[4]
    let st = sourceSearchReduce(end, { type: 'retry', connector: 'dynatrace' })
    expect(st.retrying).toEqual(['dynatrace'])
    st = sourceSearchReduce(st, { type: 'answers', sources: [dt({})], watched: '["Arbor TVA"]' })
    expect(shown(st)).toEqual(['leanix:found', 'confluence:found', 'dynatrace:searching'])
    expect(st.ranWith).toBe('["Arbor"]')
    st = sourceSearchReduce(st, { type: 'answers', sources: [dt({ state: 'found', found: 1, items: [{ key: 'tag-arbor', title: 'arbor', why: 'tag CodeAppli' }] })] })
    expect(st.retrying).toEqual([])
    expect(sourceSearchValue(st).selected.map((s) => s.connector)).toEqual(['leanix', 'confluence', 'dynatrace'])
    expect(sourceSearchValue(st).unreachable).toEqual([])
  })

  test('the proposals say they are out of date when the values the search ran with change; a new search keeps the ticks', () => {
    const data = { value: { name: 'Arbor', perimeter_tags: ['facturation'] } }
    const ran = watchedValues(['value.name', 'value.perimeter_tags'], data)
    let st = play(stream, SOURCE_SEARCH_EMPTY, ran)[4]
    expect(sourceSearchStale(st, ran)).toBe(false)
    const changed = watchedValues(['value.name', 'value.perimeter_tags'], { value: { ...data.value, perimeter_tags: ['facturation', 'TVA'] } })
    expect(sourceSearchStale(st, changed)).toBe(true)
    st = sourceSearchReduce(st, { type: 'tick', connector: 'leanix', key: 'app-eai', on: false })
    // « Chercher encore »: a new search with the values as they are now.
    st = play([[cf({}), lx({}), dt({ state: 'waiting', waiting: 'leanix' })],
      [cf({}), lx({ state: 'found', found: 1, items: [eai] }), dt({ state: 'waiting', waiting: 'leanix' })]], st, changed)[1]
    expect(sourceSearchStale(st, changed)).toBe(false)
    // What was ticked is kept - Arbor though no longer proposed, Confluence's
    // while it searches again -, EAI stays unticked.
    expect(sourceSearchValue(st).selected).toEqual([
      { connector: 'leanix', items: [arbor] },
      { connector: 'confluence', items: [tva] },
    ])
    expect(watchedValues(undefined, data)).toBeUndefined()
  })

  test('a section read again with the same answer is a new search: the out-of-date notice goes', () => {
    // The panel as its renderer moves it: the answers fold in when their fold
    // key changes - their content, or the load of the section around it.
    const panel = (): { draw: (sources: UISourceAnswer[], load: number, watched?: string) => SourceSearchState } => {
      let st = SOURCE_SEARCH_EMPTY
      let at: string | undefined
      return {
        draw: (sources, load, watched) => {
          const key = sourceSearchFoldKey(sources, load)
          if (key !== at) { at = key; st = sourceSearchReduce(st, { type: 'answers', sources, watched }) }
          return st
        },
      }
    }
    const whole = stream[4]
    const ran = watchedValues(['value.name'], { value: { name: 'Arbor' } })
    const now = watchedValues(['value.name'], { value: { name: 'Arbor 2' } })
    const p = panel()
    expect(sourceSearchStale(p.draw(whole, 1, ran), ran)).toBe(false)
    // The title changes: a redraw, the same section load - out of date.
    expect(sourceSearchStale(p.draw(whole, 1, now), now)).toBe(true)
    // The section is read again with the new title; its answer is the same.
    const st = p.draw(whole, 2, now)
    expect(sourceSearchStale(st, now)).toBe(false)
    expect(st.ranWith).toBe(now)
    // Outside a section (load 0) the answers still fold in by content alone.
    expect(sourceSearchFoldKey(whole, 0)).toBe(sourceSearchFoldKey([...whole], 0))
    expect(sourceSearchFoldKey(whole, 1)).not.toBe(sourceSearchFoldKey(whole, 2))
  })

  test('draws a found group with its count, why and ticks, « Tout cocher », « Ajouter depuis <source>… »', () => {
    const st = sourceSearchReduce(play(stream)[4], { type: 'tick', connector: 'leanix', key: 'app-eai', on: false })
    const html = draw(sourceGroupsShown(st)[0], { onAdd: () => {} })
    expect(html).toContain('data-guide="source-search.group.leanix"')
    expect(html).toContain('1/2 retenus')
    expect(html).toContain('« Arbor » dans le nom')
    expect(html).toContain('Tout cocher')
    expect(html).toContain('Ajouter depuis LeanIX…')
    expect((html.match(/checked=""/g) ?? []).length).toBe(1)
    expect(draw(sourceGroupsShown(play(stream)[4])[0])).toContain('Tout décocher')
    expect(draw(sourceGroupsShown(play(stream)[4])[0])).not.toContain('Ajouter depuis')
  })

  test('draws searching, waiting, unreachable with « Réessayer » and not configured with « Configurer »', () => {
    const states = play(stream)
    expect(draw(sourceGroupsShown(states[0])[0])).toContain('Recherche dans Confluence…')
    expect(draw(sourceGroupsShown(states[0])[2])).toContain('attend LeanIX')
    const down = draw(sourceGroupsShown(states[4])[1], { onRetry: () => {} })
    expect(down).toContain('hors réseau ACME')
    expect(down).toContain('data-guide="source-search.retry"')
    expect(down).not.toContain('Tout cocher')
    const off = sourceSearchReduce(SOURCE_SEARCH_EMPTY, { type: 'answers', sources: [dt({ state: 'not-configured', configure: { label: 'Configurer Dynatrace', hint: 'La clé DYNATRACE_TOKEN' } })] })
    const html = draw(sourceGroupsShown(off)[0], { onConfigure: () => {} })
    expect(html).toContain('non configurée')
    expect(html).toContain('Configurer Dynatrace')
    expect(html).toContain('La clé DYNATRACE_TOKEN')
  })

  test('the renderer hands the value to the page selection and runs « Réessayer » with only that source', () => {
    expect(sourceSearchKey({ id: 'source-search' })).toBe('source-search')
    expect(sourceSearchKey({ key: 'sources', id: 'source-search' })).toBe('sources')
    const src = readFileSync(join(import.meta.dir, '..', 'src', 'ui-builders', 'page', 'nodes', 'source-search.tsx'), 'utf8')
    expect(src).toContain('select(key, value)')
    expect(src).toContain('retry(g.answer, { only: g.answer.connector })')
    expect(src).toContain("move({ type: 'answers', sources: node.data?.sources, watched })")
    // It folds on the answers and the load of the section around it.
    expect(src).toContain('sourceSearchFoldKey(node.data?.sources, useSectionLoad())')
    expect(src).toContain('}, [fold])')
    const section = readFileSync(join(import.meta.dir, '..', 'src', 'ui-builders', 'page', 'nodes', 'section-load.tsx'), 'utf8')
    expect(section).toContain('<SectionLoadContext.Provider value={got.loads ?? 0}>')
    expect(section).toContain('loads: (was.loads ?? 0) + 1')
  })
})

describe('« Ajouter depuis <source>… » with no carrier', () => {
  const picked = { key: 'page-caisse', title: 'Caisse magasin' }
  const connectors = [
    { name: 'confluence', label: 'Confluence', search: 'ktw-example:confluence/source-search' },
    { name: 'leanix', label: 'LeanIX', browse: 'ktw-example:leanix/source-browse' },
    { name: 'dynatrace', label: 'Dynatrace' },
  ]

  test('with no carrier the picks join their group ticked, never twice, and stay ticked through a later answer', () => {
    let st = play(stream)[4]
    st = sourceSearchReduce(st, { type: 'tick', connector: 'confluence', key: 'page-tva', on: false })
    st = sourceSearchReduce(st, { type: 'add', connector: 'confluence', items: [picked, tva] })
    const conf = sourceGroupsShown(st).find((g) => g.answer.connector === 'confluence')!
    expect(conf.answer.items!.map((i) => i.key)).toEqual(['page-tva', 'page-caisse'])
    expect(conf.ticked).toEqual(['page-tva', 'page-caisse'])
    // Added again: nothing twice.
    st = sourceSearchReduce(st, { type: 'add', connector: 'confluence', items: [picked] })
    expect(sourceGroupsShown(st).find((g) => g.answer.connector === 'confluence')!.answer.items!.length).toBe(2)
    // The other groups are untouched; the page's selection carries the picks under their connector.
    expect(sourceSearchValue(st).selected).toEqual([
      { connector: 'leanix', items: [arbor, eai] },
      { connector: 'confluence', items: [tva, picked] },
    ])
    // A later answer of the search leaves the pick out: it stays, ticked.
    st = sourceSearchReduce(st, { type: 'answers', sources: [cf({ state: 'found', found: 1, items: [tva] })] })
    expect(sourceSearchValue(st).selected.find((s) => s.connector === 'confluence')).toEqual({ connector: 'confluence', items: [tva, picked] })
    // An unreachable item is never added.
    const same = sourceSearchReduce(st, { type: 'add', connector: 'confluence', items: [{ key: 'gone', title: 'Gone', unreachable: true }] })
    expect(same).toBe(st)
  })

  test('a proposed item picked again « avec les sous-éléments » is held with its sub-pages, through a later answer too', () => {
    let st = play(stream)[4]
    st = sourceSearchReduce(st, { type: 'add', connector: 'confluence', items: [{ ...tva, with_children: true }] })
    const conf = () => sourceGroupsShown(st).find((g) => g.answer.connector === 'confluence')!
    expect(conf().answer.items!.filter((i) => i.key === 'page-tva').length).toBe(1)
    expect(conf().ticked).toContain('page-tva')
    expect(sourceSearchValue(st).selected.find((s) => s.connector === 'confluence')).toEqual({ connector: 'confluence', items: [{ ...tva, with_children: true }] })
    // The search proposes it again: still with its sub-pages.
    st = sourceSearchReduce(st, { type: 'answers', sources: [cf({ state: 'found', found: 1, items: [tva] })] })
    expect(sourceSearchValue(st).selected.find((s) => s.connector === 'confluence')).toEqual({ connector: 'confluence', items: [{ ...tva, with_children: true }] })
    // Picked again without them: the page alone.
    st = sourceSearchReduce(st, { type: 'add', connector: 'confluence', items: [tva] })
    expect(sourceSearchValue(st).selected.find((s) => s.connector === 'confluence')).toEqual({ connector: 'confluence', items: [tva] })
  })

  test('with no carrier the button shows on a group found or with nothing new whose connector has a search or a browse', () => {
    const end = play(stream)[4]
    const group = (c: string) => sourceGroupsShown(end).find((g) => g.answer.connector === c)!
    expect(sourceGroupAddable(group('confluence'), connectors)?.name).toBe('confluence')
    expect(sourceGroupAddable(group('leanix'), connectors)?.name).toBe('leanix')
    // Unreachable: its picks would be lost.
    expect(sourceGroupAddable(group('dynatrace'), connectors)).toBeUndefined()
    const at = (state: string) => ({ answer: cf({ state }), order: 1, ticked: [] })
    expect(sourceGroupAddable(at('nothing-new'), connectors)?.name).toBe('confluence')
    for (const s of ['searching', 'waiting', 'unreachable', 'not-configured']) expect(sourceGroupAddable(at(s), connectors)).toBeUndefined()
    // A connector with neither search nor browse, one not listed, no list yet.
    expect(sourceGroupAddable({ answer: dt({ state: 'found', items: [] }), order: 1, ticked: [] }, connectors)).toBeUndefined()
    expect(sourceGroupAddable({ answer: { connector: 'jira', state: 'found' }, order: 1, ticked: [] }, connectors)).toBeUndefined()
    expect(sourceGroupAddable(at('found'), null)).toBeUndefined()
  })

  test('with no carrier the group is drawn with the button and the renderer wires the picker modal to the add event', () => {
    const end = play(stream)[4]
    const conf = sourceGroupsShown(end).find((g) => g.answer.connector === 'confluence')!
    expect(draw(conf, { onAdd: () => {} })).toContain('Ajouter depuis Confluence…')
    const src = readFileSync(join(import.meta.dir, '..', 'src', 'ui-builders', 'page', 'nodes', 'source-search.tsx'), 'utf8')
    expect(src).toContain("useConnectors(!add && node.pickers ? { connectors: node.pickers } : undefined)")
    expect(src).toContain("move({ type: 'add', connector: adding.connector.name, items })")
    expect(src).toContain('<Picker node={connectorNode(connector)} selection={picked} onSelection={setPicked} />')
    expect(SOURCE_SEARCH_LABELS.addConfirm).toBe('Ajouter')
    expect(SOURCE_SEARCH_LABELS.cancel).toBe('Annuler')
  })
})
