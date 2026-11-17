// SPDX-License-Identifier: Apache-2.0
// A generic grouped list any page uses - elements grouped by a
// field, a count per group in its header whose click shows only that group
// and a second click shows all, an overdue count, and a bulk selection that
// answers several elements of one group at once, each answer separate and
// only where the action applies; headings by a second field (counted by kind,
// drawn by holder), each kept under a filter.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import {
  bulkEvents, countLook, groupCounts, groupItems, itemKey, NO_SELECTION, OVERDUE_FILTER, sectionTick, selectedItems, selectGroup,
  toggleFilter, toggleSelected, visibleSections,
} from '../src/ui-builders/page/grouped-list.ts'

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const page = (node: unknown) => html(createElement(UIPageView, { node: node as never, onAction: (() => {}) as never }))

const tasks = [
  { id: 'ht-1', kind: 'approve', who: 'team', title: 'Approve the notes', late: true },
  { id: 'ht-2', kind: 'decide', who: 'mine', title: 'Pick a source' },
  { id: 'ht-3', kind: 'approve', who: 'mine', title: 'Approve the plan' },
  { id: 'ht-4', kind: 'provide', who: 'team', title: 'Give the key', late: 'since 2 days' },
  { id: 'ht-5', kind: 'approve', who: 'team', title: 'Approve the test', late: false },
]
const holders = [{ key: 'mine', label: 'Mine' }, { key: 'team', label: 'Holding list hl-cosmos' }]
const ks = (gs: { key: string; items: { key: string }[] }[]) => gs.map((g) => [g.key, g.items.map((it) => it.key)])
const named = [{ key: 'decide', label: 'Décision' }, { key: 'approve', label: 'Validation' }, { key: 'choose', label: 'Choix' }]
const report = { id: 'approve', label: 'Approve', uri: 'ktw-agents:todos/report', args: { id: '${item.id}', verdict: 'approved' } }

describe('grouping by a field', () => {
  test('groups the named first, in their order and labels, then the others as first seen', () => {
    const gs = groupItems(tasks, 'kind', named)
    expect(gs.map((g) => [g.key, g.label, g.items.map((it) => it.key)])).toEqual([
      ['decide', 'Décision', ['id:ht-2']],
      ['approve', 'Validation', ['id:ht-1', 'id:ht-3', 'id:ht-5']],
      ['choose', 'Choix', []],
      ['provide', 'provide', ['id:ht-4']],
    ])
    // An element without the field, or without an id, still has its place.
    const loose = groupItems([{ title: 'x' }], 'kind')
    expect(loose).toEqual([{ key: '', label: '-', items: [{ el: { title: 'x' }, index: 0, key: 'at:0', group: '' }] }])
  })

  test('an element without an id never shares its key with one whose id reads like a position', () => {
    const els = [{ id: '1', kind: 'a' }, { kind: 'a' }, { id: 'at:0', kind: 'a' }]
    const keys = groupItems(els, 'kind')[0].items.map((it) => it.key)
    expect(new Set(keys).size).toBe(3)
    expect(itemKey({ id: '1' }, 0)).not.toBe(itemKey({}, 1))
    // Ticking the id-less element does not tick the one whose id is « 1 ».
    const sel = toggleSelected(NO_SELECTION, 'a', itemKey(els[1], 1))
    expect(selectedItems(groupItems(els, 'kind'), sel)).toEqual([els[1]])
  })

  test('the header counts each group, then the overdue ones', () => {
    const gs = groupItems(tasks, 'kind', named)
    expect(groupCounts(gs, { overdueField: 'late' }, 'fr')).toEqual([
      { key: 'decide', label: 'Décision', count: 1 },
      { key: 'approve', label: 'Validation', count: 3 },
      { key: 'choose', label: 'Choix', count: 0 },
      { key: 'provide', label: 'provide', count: 1 },
      { key: OVERDUE_FILTER, label: 'en retard', count: 2 },
    ])
    expect(groupCounts(gs, { overdueField: 'late', overdueLabel: 'late!' }, 'en').at(-1)).toEqual({ key: OVERDUE_FILTER, label: 'late!', count: 2 })
    // No overdue field, or none overdue: no overdue count.
    expect(groupCounts(gs, {}, 'en').some((c) => c.key === OVERDUE_FILTER)).toBe(false)
    expect(groupCounts(groupItems([tasks[1]], 'kind'), { overdueField: 'late' }).some((c) => c.key === OVERDUE_FILTER)).toBe(false)
  })
})

describe('a click on a count shows only that group, a second shows all', () => {
  const gs = groupItems(tasks, 'kind', named)
  test('the filter toggles', () => {
    expect(toggleFilter(null, 'approve')).toBe('approve')
    expect(toggleFilter('approve', 'approve')).toBeNull()
    expect(toggleFilter('approve', 'decide')).toBe('decide')
  })
  test('what shows under each filter, the headings being the groups', () => {
    const keys = (f: string | null) => ks(visibleSections(gs, f, { overdueField: 'late' }))
    // All: the empty group is not drawn.
    expect(keys(null)).toEqual([['decide', ['id:ht-2']], ['approve', ['id:ht-1', 'id:ht-3', 'id:ht-5']], ['provide', ['id:ht-4']]])
    expect(keys('approve')).toEqual([['approve', ['id:ht-1', 'id:ht-3', 'id:ht-5']]])
    // A group's count at 0 clicked: its heading, with nothing under it.
    expect(keys('choose')).toEqual([['choose', []]])
    // Overdue: each group holding an element stays, the empty one for its note.
    expect(keys(OVERDUE_FILTER)).toEqual([['decide', []], ['approve', ['id:ht-1']], ['provide', ['id:ht-4']]])
  })
})

describe('headings by a second field, counts by the first', () => {
  const gs = groupItems(tasks, 'kind', named)
  const spec = { overdueField: 'late', sectionBy: 'who', sectionGroups: holders }
  test('the header counts by kind while the elements are drawn by holder', () => {
    expect(groupCounts(gs, spec, 'en').map((c) => [c.key, c.count])).toEqual([
      ['decide', 1], ['approve', 3], ['choose', 0], ['provide', 1], [OVERDUE_FILTER, 2],
    ])
    const all = visibleSections(gs, null, spec)
    expect(all.map((s) => s.label)).toEqual(['Mine', 'Holding list hl-cosmos'])
    // In the list's order under each heading.
    expect(ks(all)).toEqual([['mine', ['id:ht-2', 'id:ht-3']], ['team', ['id:ht-1', 'id:ht-4', 'id:ht-5']]])
  })
  test('under a filter every heading stays, an empty one for « Nothing of this kind. »', () => {
    expect(ks(visibleSections(gs, 'decide', spec))).toEqual([['mine', ['id:ht-2']], ['team', []]])
    expect(ks(visibleSections(gs, 'choose', spec))).toEqual([['mine', []], ['team', []]])
    expect(ks(visibleSections(gs, OVERDUE_FILTER, spec))).toEqual([['mine', []], ['team', ['id:ht-1', 'id:ht-4']]])
    // A named heading no element holds is drawn too; another value follows, labelled by itself.
    const odd = visibleSections(groupItems([{ id: 'x', kind: 'a', who: 'bot' }], 'kind'), null, spec)
    expect(odd.map((s) => [s.key, s.label, s.items.length])).toEqual([['mine', 'Mine', 0], ['team', 'Holding list hl-cosmos', 0], ['bot', 'bot', 1]])
  })
  test("a heading's tick selects in one kind only: the filtered one", () => {
    const team = visibleSections(gs, null, spec)[1]
    expect(sectionTick(team, null, spec)).toBeNull()
    expect(sectionTick(team, OVERDUE_FILTER, spec)).toBeNull()
    expect(sectionTick(team, 'approve', spec)).toBe('approve')
    expect(sectionTick(gs[1], null, { overdueField: 'late' })).toBe('approve')
    // selectGroup keeps to that kind whatever the heading holds.
    expect(selectGroup(NO_SELECTION, 'approve', team.items)).toEqual({ group: 'approve', keys: ['id:ht-1', 'id:ht-5'] })
  })
})

describe('bulk select answers several elements of one group, each answer separate', () => {
  const gs = groupItems(tasks, 'kind', named)
  test('a selection stays in one group: a tick elsewhere starts a new one', () => {
    let sel = toggleSelected(NO_SELECTION, 'approve', 'id:ht-1')
    sel = toggleSelected(sel, 'approve', 'id:ht-3')
    expect(sel).toEqual({ group: 'approve', keys: ['id:ht-1', 'id:ht-3'] })
    expect(toggleSelected(sel, 'decide', 'id:ht-2')).toEqual({ group: 'decide', keys: ['id:ht-2'] })
    expect(toggleSelected(toggleSelected(sel, 'approve', 'id:ht-1'), 'approve', 'id:ht-3')).toEqual(NO_SELECTION)
  })
  test('the tick of a group selects all of it, again none', () => {
    const approve = gs.find((g) => g.key === 'approve')!
    const all = selectGroup(NO_SELECTION, 'approve', approve.items)
    expect(all).toEqual({ group: 'approve', keys: ['id:ht-1', 'id:ht-3', 'id:ht-5'] })
    expect(selectGroup(all, 'approve', approve.items)).toEqual(NO_SELECTION)
  })
  test('one run of the action per element, each on its element', () => {
    const picked = selectedItems(gs, { group: 'approve', keys: ['id:ht-5', 'id:ht-1'] })
    expect(picked.map((x) => (x as { id: string }).id)).toEqual(['ht-1', 'ht-5'])
    const { events: evs, skipped } = bulkEvents(report as never, picked)
    expect(evs).toHaveLength(2)
    expect(skipped).toEqual([])
    expect(evs.map((e) => (e.item as { id: string }).id)).toEqual(['ht-1', 'ht-5'])
    expect(evs.every((e) => e.action === (report as never))).toBe(true)
    // A key no longer in its group (a refresh moved it) is not answered.
    expect(selectedItems(gs, { group: 'decide', keys: ['id:ht-1'] })).toEqual([])
  })
  test('an element where the action is hidden or disabled is skipped, as its own button would be', () => {
    const els = [
      { id: 'a', kind: 'approve', open: true },
      { id: 'b', kind: 'approve', open: false },
      { id: 'c', kind: 'approve', open: true, waiting: 'Held by Marie' },
      { id: 'd', kind: 'approve', open: true },
    ]
    const act = { ...report, when: 'open', disabledWhen: 'waiting' }
    const { events, skipped } = bulkEvents(act as never, els)
    expect(events.map((e) => (e.item as { id: string }).id)).toEqual(['a', 'd'])
    expect(skipped.map((x) => (x as { id: string }).id)).toEqual(['b', 'c'])
    // `when` with a value: only the elements holding it.
    const onlyTeam = bulkEvents({ ...report, when: 'who=team' } as never, tasks)
    expect(onlyTeam.events.map((e) => (e.item as { id: string }).id)).toEqual(['ht-1', 'ht-4', 'ht-5'])
  })
})

describe('the list draws it', () => {
  const node = { kind: 'list', id: 'todo', data: tasks, item: { kind: 'text', content: '', dataField: '$.item.title' }, groupBy: 'kind', groups: named, overdueField: 'late' }
  test('a header with a count per group and the overdue count, each group under its heading', () => {
    const out = page(node)
    expect(out).toContain('data-element="list:todo"')
    for (const [k, n] of [['decide', 1], ['approve', 3], ['choose', 0], ['provide', 1], [OVERDUE_FILTER, 2]]) {
      expect(out).toMatch(new RegExp(`data-group-filter="${k}" data-count="${n}"`))
    }
    expect((out.match(/data-group="/g) ?? []).length).toBe(3)
    expect(out.indexOf('data-group="decide"')).toBeLessThan(out.indexOf('data-group="approve"'))
    expect(out).toContain('Approve the plan')
    // No bulk actions: no ticks.
    expect(out).not.toContain('data-select=')
  })
  test('sections: false draws the elements in their order with no heading', () => {
    const out = page({ ...node, sections: false })
    expect(out).not.toContain('data-group="')
    expect(out.indexOf('Approve the notes')).toBeLessThan(out.indexOf('Approve the plan'))
  })
  test('bulk actions put a tick on each element and on each group', () => {
    const out = page({ ...node, bulk: [report] })
    expect((out.match(/data-select="/g) ?? []).length).toBe(5)
    expect((out.match(/data-select-group="/g) ?? []).length).toBe(3)
    expect(out).toContain('data-select="id:ht-1"')
  })
  test('the overdue count is drawn like the others, not as a red button', () => {
    const out = page(node)
    const btn = (k: string) => out.match(new RegExp(`<button[^>]*data-group-filter="${k}"[^>]*>`))![0]
    const strip = (b: string) => b.replace(/data-group-filter="[^"]*"/, '').replace(/data-count="[^"]*"/, '')
    expect(strip(btn(OVERDUE_FILTER))).toBe(strip(btn('approve')))
    // Active or not, the overdue count looks like any other count.
    expect(countLook(null, OVERDUE_FILTER)).toEqual(countLook(null, 'approve'))
    expect(countLook(OVERDUE_FILTER, OVERDUE_FILTER)).toEqual(countLook('approve', 'approve'))
    expect(countLook('approve', OVERDUE_FILTER)).toEqual(countLook(OVERDUE_FILTER, 'approve'))
  })
  test('headings by holder, counts by kind; every heading drawn with its note when empty', () => {
    const out = page({ ...node, sectionBy: 'who', sectionGroups: [...holders, { key: 'bots', label: 'Bots' }], bulk: [report] })
    for (const [k, n] of [['decide', 1], ['approve', 3], [OVERDUE_FILTER, 2]]) {
      expect(out).toMatch(new RegExp(`data-group-filter="${k}" data-count="${n}"`))
    }
    expect(out.match(/data-group="[^"]*"/g)).toEqual(['data-group="mine"', 'data-group="team"', 'data-group="bots"'])
    expect(out).toContain('Holding list hl-cosmos')
    expect(out).toContain('data-empty-section="bots"')
    expect(out).toContain('Nothing of this kind.')
    // Headings mixing kinds have no tick of their own; each element does.
    expect(out).not.toContain('data-select-group=')
    expect((out.match(/data-select="/g) ?? []).length).toBe(5)
  })
  test('a plain list is unchanged', () => {
    const out = page({ kind: 'list', data: tasks, item: node.item })
    expect(out).not.toContain('data-group-filter')
  })
  test('the grammar takes the grouped list', () => {
    const p = (UIPageNodeJSONSchema as any).definitions.List.properties
    for (const k of ['groupBy', 'groups', 'overdueField', 'overdueLabel', 'sectionBy', 'sectionGroups', 'sections', 'bulk']) expect(p[k]).toBeDefined()
    expect(p.bulk.items).toEqual({ $ref: '#/definitions/Action' })
  })
})

// « À faire » on the grouped list - the conversations under a
// heading of their own, drawn alone under their filter and hidden under a
// question's; the header in the page's own words (« kind »); no count on the
// headings; each element drawn as the node it carries.
describe('a heading for some groups alone, the header in the page words, elements drawing themselves', () => {
  const asks = [
    ...tasks,
    { id: 'dp-1', kind: 'conversation', who: 'chats', title: 'Chat · agent architecte' },
  ]
  const sections = [...holders, { key: 'chats', label: 'Conversations', only: ['conversation'] }]
  const gs = groupItems(asks, 'kind', named)
  const spec = { overdueField: 'late', sectionBy: 'who', sectionGroups: sections }
  test('with no filter every heading, the conversations last', () => {
    expect(ks(visibleSections(gs, null, spec))).toEqual([
      ['mine', ['id:ht-2', 'id:ht-3']], ['team', ['id:ht-1', 'id:ht-4', 'id:ht-5']], ['chats', ['id:dp-1']],
    ])
  })
  test("a question's filter or the overdue one hides the conversations' heading, the others kept", () => {
    expect(ks(visibleSections(gs, 'approve', spec))).toEqual([['mine', ['id:ht-3']], ['team', ['id:ht-1', 'id:ht-5']]])
    expect(ks(visibleSections(gs, 'choose', spec))).toEqual([['mine', []], ['team', []]])
    expect(ks(visibleSections(gs, OVERDUE_FILTER, spec))).toEqual([['mine', []], ['team', ['id:ht-1', 'id:ht-4']]])
  })
  test("the conversations' filter draws their heading alone", () => {
    expect(ks(visibleSections(gs, 'conversation', spec))).toEqual([['chats', ['id:dp-1']]])
  })
  const node = {
    kind: 'list', id: 'todo', data: asks.map((a) => ({ ...a, card: { kind: 'text', content: `card ${a.id}` } })), nodeField: 'card',
    groupBy: 'kind', groups: named, overdueField: 'late', sectionBy: 'who', sectionGroups: sections,
    filterLabel: 'Filter by kind', filterHint: 'Click a count to show only that kind.', sectionCounts: false,
  }
  test('the header says « kind », the headings carry no count, each element its own node', () => {
    const out = page(node)
    expect(out).toContain('aria-label="Filter by kind"')
    expect(out).toContain('Click a count to show only that kind.')
    expect(out).not.toContain('Filter by group')
    expect(out).toMatch(/data-group-filter="conversation" data-count="1"/)
    expect(out.match(/data-group="[^"]*"/g)).toEqual(['data-group="mine"', 'data-group="team"', 'data-group="chats"'])
    for (const a of asks) expect(out).toContain(`card ${a.id}`)
    // A heading's count badge is gone; its default stays.
    const head = (o: string) => o.slice(o.indexOf('data-group="team"'), o.indexOf('card ht-1'))
    expect(head(out)).not.toMatch(/mantine-Badge-root/)
    expect(head(page({ ...node, sectionCounts: undefined }))).toMatch(/mantine-Badge-root/)
  })
  test('an element without its node is drawn by the template, and a list may give no template', () => {
    const out = page({ ...node, data: [{ id: 'x', kind: 'approve', who: 'mine', title: 'bare' }], item: { kind: 'text', content: '', dataField: '$.item.title' } })
    expect(out).toContain('bare')
    expect(page({ kind: 'list', data: [{ id: 'y', card: { kind: 'text', content: 'own y' } }], nodeField: 'card' })).toContain('own y')
  })
  test('the grammar takes them', () => {
    const p = (UIPageNodeJSONSchema as any).definitions.List.properties
    for (const k of ['filterLabel', 'filterHint', 'sectionCounts', 'nodeField']) expect(p[k]).toBeDefined()
    expect(p.sectionGroups.items.properties.only).toBeDefined()
  })
})

// The previous « À faire » view's look, any page can use - the
// header's counts as KPI tiles (each group's icon, colour and hint, a first
// tile counting everything), each heading a card « <label> — <n> », and the
// elements' place named in a dimmed line when it changes.
describe('tiles, section cards and place lines', () => {
  const looked = [
    { key: 'approve', label: 'Validation', icon: 'circle-check', color: 'teal', hint: 'Validate or send back' },
    { key: 'decide', label: 'Décision', icon: 'arrows-split', color: 'grape', hint: 'Choose an option' },
  ]
  const ordered = [tasks[2], tasks[1], tasks[0], tasks[4], tasks[3]] // mine first within each kind
  const node = {
    kind: 'list', id: 'todo', data: ordered, groupBy: 'kind', groups: looked, overdueField: 'late',
    filterStyle: 'tiles', allLabel: 'Tout', sectionStyle: 'card', placeField: 'who', places: holders,
    item: { kind: 'text', content: '', dataField: '$.item.title' },
  }

  test('the header is tiles: « Tout » counting everything, then each group with its count and hint, the overdue one', () => {
    const out = page(node)
    expect(out).toContain('data-group-all="5"')
    expect(out).toContain('Tout')
    expect(out).toMatch(/data-group-filter="approve"[^>]*data-count="3"/)
    expect(out).toMatch(/data-group-filter="decide"[^>]*data-count="1"/)
    expect(out).toContain('Validate or send back')
    expect(out).toContain(`data-group-filter="${OVERDUE_FILTER}"`)
    // The tiles are the counts: no chip button among them.
    expect(out).not.toMatch(/<button[^>]*data-group-filter="approve"[^>]*mantine-Button-root/)
  })

  test('each group is a card titled « <label> — <n> », its hint under it, no count badge', () => {
    const out = page(node)
    expect(out).toContain('Validation — 3')
    expect(out).toContain('Décision — 1')
    const section = /data-group="approve"[\s\S]*?(?=data-group="|$)/.exec(out)?.[0] ?? ''
    expect(section).toContain('Validate or send back')
    expect(section).not.toContain('mantine-Badge-root')
  })

  test('each section names its heading (data-group-heading): the title alone, no count badge even when its cards carry badges', () => {
    const badged = { ...node, item: { kind: 'badge', text: '', dataField: '$.item.title' } }
    const out = page(badged)
    for (const key of ['approve', 'decide']) {
      const at = out.indexOf(`data-group-heading="${key}"`)
      const heading = at < 0 ? '' : out.slice(at, out.indexOf('</h5>', at))
      expect(heading).not.toBe('')
      expect(heading).not.toContain('mantine-Badge-root')
    }
    // The cards under it do carry badges: a check on the section's first child would see them.
    const section = /data-group="approve"[\s\S]*?(?=data-group="|$)/.exec(out)?.[0] ?? ''
    expect(section).toContain('mantine-Badge-root')
    // The default heading style names its heading too.
    expect(page({ ...badged, sectionStyle: undefined })).toContain('data-group-heading="approve"')
  })

  test('a place line names the holder each time it changes, « Mine » first', () => {
    const out = page(node)
    const section = /data-group="approve"[\s\S]*?(?=data-group="|$)/.exec(out)?.[0] ?? ''
    const mine = section.indexOf('data-place="Mine"'), team = section.indexOf('data-place="Holding list hl-cosmos"')
    expect(mine).toBeGreaterThan(-1)
    expect(team).toBeGreaterThan(mine)
    expect(section.split('data-place="Holding list hl-cosmos"').length).toBe(2) // once, not per element
  })

  test('without the options, the header stays chips and the headings labels', () => {
    const out = page({ ...node, filterStyle: undefined, allLabel: undefined, sectionStyle: undefined, placeField: undefined })
    expect(out).not.toContain('data-group-all')
    expect(out).not.toContain('Validation — 3')
    expect(out).not.toContain('data-place')
  })

  test('the grammar declares them', () => {
    const list = (UIPageNodeJSONSchema as any).definitions.List.properties
    expect(list.filterStyle.enum).toEqual(['chips', 'tiles'])
    expect(list.sectionStyle.enum).toEqual(['heading', 'card'])
    expect(list.placeField.type).toBe('string')
    expect(list.groups.items.properties.icon.type).toBe('string')
  })
})
