// SPDX-License-Identifier: Apache-2.0
// One generic picker node, driven only by the method URIs it is
// given - its selection (added once, never an unreachable item, removed with
// what sits under it, shown as a list or a tree), its declared badges, and its
// rendering with and without a host that calls methods.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import {
  addToSelection, badgeOf, PickerRenderer, removeFromSelection, selectionRows,
} from '../src/ui-builders/page/nodes/picker.tsx'

const space = { key: 'S', title: 'Space AUP' }
const page = { key: 'P1', title: 'ERP Arbor', parent: 'S' }
const child = { key: 'P2', title: 'Flux', parent: 'P1' }
const dead = { key: 'X', title: 'Gone', unreachable: true }

describe('the picker node', () => {
  test('is a declared interactive kind, its search optional', () => {
    expect(INTERACTIVE_KINDS).toContain('picker')
    const def = (UIPageNodeJSONSchema as any).definitions.Picker
    expect(def.required).toEqual(['kind'])
  })

  test('adds an item once and never an unreachable one', () => {
    let sel = addToSelection([], space)
    sel = addToSelection(sel, space)
    sel = addToSelection(sel, dead)
    expect(sel.map((s) => s.key)).toEqual(['S'])
  })

  test('removes an item with what sits under it', () => {
    const sel = [space, page, child, { key: 'Q', title: 'Other' }]
    expect(removeFromSelection(sel, 'P1').map((s) => s.key)).toEqual(['S', 'Q'])
    expect(removeFromSelection(sel, 'S').map((s) => s.key)).toEqual(['Q'])
  })

  test('shows the selection as a list, or as a tree by parent', () => {
    const sel = [child, space, page]
    expect(selectionRows(sel, 'list').map((r) => r.item.key)).toEqual(['P2', 'S', 'P1'])
    expect(selectionRows(sel, 'tree').map((r) => [r.item.key, r.depth])).toEqual([['S', 0], ['P1', 1], ['P2', 2]])
  })

  test('reads a declared badge, its name when undeclared', () => {
    expect(badgeOf({ ia: { label: 'IA', color: 'violet' } }, 'ia')).toEqual({ label: 'IA', color: 'violet' })
    expect(badgeOf(undefined, 'new')).toEqual({ label: 'new' })
  })

  const render = (host: object, node: object) => renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))

  test('without a host calling methods, shows the selection only', () => {
    const html = render({}, { kind: 'picker', search: 'x:search', selected: [{ ...page, badges: ['ia'], state: 'failed', why: 'denied' }], badges: { ia: { label: 'IA' } } })
    expect(html).toContain('La recherche n’est pas disponible ici')
    expect(html).toContain('ERP Arbor')
    expect(html).toContain('IA')
    expect(html).toContain('en échec') // the state in the picker's words
    expect(html).toContain('Retirer')
    expect(html).not.toContain('picker.search')
  })

  test('with a host, offers the search and the apply button', () => {
    const html = render({ callMethod: async () => ({ items: [] }) }, { kind: 'picker', search: 'x:search', apply: 'x:apply', labels: { apply: 'Utiliser' } })
    expect(html).toContain('data-guide="picker.search"')
    expect(html).toContain('Utiliser')
    expect(html).toContain('Rien de sélectionné')
  })
})

describe('the picker calls its methods with its args', () => {
  test('args, object and the query make the search input', async () => {
    const { searchInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(searchInput({ args: { connector: 'apps' }, object: 'm' }, '  arbor ')).toEqual({ connector: 'apps', object: 'm', query: 'arbor' })
    expect(searchInput({}, 'x')).toEqual({ query: 'x' })
  })
})

describe('the picker browses', () => {
  test('a level is asked with the args, the object and its parent', async () => {
    const { browseInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(browseInput({ args: { id: 'm.apps' }, object: 'm' }, 'space:ARCHI')).toEqual({ id: 'm.apps', object: 'm', parent: 'space:ARCHI' })
    expect(browseInput({ args: { id: 'm.apps' } })).toEqual({ id: 'm.apps' })
  })
  test('offers « Parcourir » where the host calls methods and the node browses', async () => {
    const { renderToStaticMarkup } = await import('react-dom/server')
    const { createElement } = await import('react')
    const { MantineProvider } = await import('@mantine/core')
    const { HostServicesContext } = await import('../src/ui-core/host/index.tsx')
    const { PickerRenderer } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const node = { kind: 'picker', search: 'x:s/search', browse: 'x:s/browse' } as any
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({}) } } as any, createElement(PickerRenderer, { node }))))
    expect(html).toContain('Parcourir')
    const bare = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({}) } } as any, createElement(PickerRenderer, { node: { kind: 'picker', search: 'x:s/search' } as any }))))
    expect(bare).not.toContain('Parcourir')
  })
})

describe('the picker says where its source stands', () => {
  const labels = { notConfigured: 'not configured', notReachable: 'unreachable' }
  test('a banner when the source is not configured or not reachable, none when it answers', async () => {
    const { statusBanner } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(statusBanner({ configured: false, message: 'APPS_TOKEN is not set' }, labels)).toEqual({ tone: 'yellow', text: 'not configured - APPS_TOKEN is not set' })
    expect(statusBanner({ configured: true, reachable: false, message: 'timeout' }, labels)?.tone).toBe('red')
    expect(statusBanner({ configured: true, reachable: true }, labels)).toBeUndefined()
    expect(statusBanner(undefined, labels)).toBeUndefined()
  })
  test('an item picked with what sits under it, only when it has some', async () => {
    const { withChildrenOf } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(withChildrenOf({ key: 'p', title: 'P', has_children: true }, true).with_children).toBe(true)
    expect(withChildrenOf({ key: 'q', title: 'Q' }, true).with_children).toBeUndefined()
    expect(withChildrenOf({ key: 'p', title: 'P', has_children: true }, false).with_children).toBeUndefined()
  })
  test('draws its derived note; a tree has no global « avec les sous-éléments » box', async () => {
    const { renderToStaticMarkup } = await import('react-dom/server')
    const { createElement } = await import('react')
    const { MantineProvider } = await import('@mantine/core')
    const { HostServicesContext } = await import('../src/ui-core/host/index.tsx')
    const { PickerRenderer } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const host = { callMethod: async () => ({}) } as any
    const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(HostServicesContext.Provider, { value: host },
      createElement(PickerRenderer, { node: { kind: 'picker', search: 'x:s', layout: 'tree', derived: 'its interfaces' } as any }))))
    expect(html).toContain('its interfaces')
    expect(html).not.toContain('picker.with-children')
    const list = renderToStaticMarkup(createElement(MantineProvider, null, createElement(HostServicesContext.Provider, { value: host },
      createElement(PickerRenderer, { node: { kind: 'picker', search: 'x:s' } as any }))))
    expect(list).not.toContain('avec les sous-éléments')
  })
})

describe('the picker as the mock-up draws it', () => {
  test('a live search from two letters, the items carrying a badge', async () => {
    const { liveQuery, withBadge } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(liveQuery('r')).toBe(false)
    expect(liveQuery(' ro ')).toBe(true)
    const sel = [{ key: 'a', title: 'A', badges: ['ai'] }, { key: 'b', title: 'B' }]
    expect(withBadge(sel, 'ai').map((s) => s.key)).toEqual(['a'])
  })
  test('two columns, its hint, « dont 1 IA » and « Retirer les IA »', async () => {
    const { renderToStaticMarkup } = await import('react-dom/server')
    const { createElement } = await import('react')
    const { MantineProvider } = await import('@mantine/core')
    const { HostServicesContext } = await import('../src/ui-core/host/index.tsx')
    const { PickerRenderer } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const node = { kind: 'picker', search: 'x:s', columns: true, live: true, hint: 'Tapez au moins deux lettres', removeBadge: 'ai',
      badges: { ai: { label: 'IA', color: 'violet' } }, selected: [{ key: 'p1', title: 'Étude', badges: ['ai'] }, { key: 'p2', title: 'Notes' }] } as any
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({}) } } as any, createElement(PickerRenderer, { node }))))
    expect(html).toContain('Rechercher et ajouter')
    expect(html).toContain('Tapez au moins deux lettres')
    expect(html).toContain('dont 1 IA')
    expect(html).toContain('Retirer les IA')
  })
})

describe('the picker of proposed items', () => {
  const proposed = [
    { key: 'p1', title: 'ERP Arbor', group: 'Confluence', why: '« Arbor » et « facturation »' },
    { key: 'p2', title: 'Notes', group: 'Confluence', why: 'cité par 2 pages retenues', badges: ['ai'] },
    { key: 'a1', title: 'Arbor', group: 'LeanIX', why: 'application du périmètre' },
  ]
  const html = (node: object, host: object = { callMethod: async () => ({}) }) => renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))

  test('an item is unticked in place - kept, never handed back - and ticked again', async () => {
    const { toggleItem, tickedItems } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const off = toggleItem(proposed, 'p2')
    expect(off.map((s) => s.key)).toEqual(['p1', 'p2', 'a1'])
    expect(off[1].off).toBe(true)
    expect(tickedItems(off).map((s) => s.key)).toEqual(['p1', 'a1'])
    const on = toggleItem(off, 'p2')
    expect('off' in on[1]).toBe(false)
    expect(tickedItems(on).length).toBe(3)
  })

  test('« Tout cocher » ticks or unticks one group only, each counted', async () => {
    const { tickGroup, pickerGroups } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const none = tickGroup(proposed, 'Confluence', false)
    expect(pickerGroups(none).map((g) => [g.group, g.ticked, g.items.length])).toEqual([['Confluence', 0, 2], ['LeanIX', 1, 1]])
    const all = tickGroup(none, 'Confluence', true)
    expect(pickerGroups(all).map((g) => g.ticked)).toEqual([2, 1])
    expect(pickerGroups([{ key: 'x', title: 'X' }]).map((g) => g.group)).toEqual([''])
  })

  test('draws each item ticked or struck through with its why line, a count and « Tout cocher » per group', () => {
    const out = html({ kind: 'picker', search: 'x:s', tickable: true, badges: { ai: { label: 'IA' } },
      selected: [proposed[0], { ...proposed[1], off: true }, proposed[2]] })
    expect(out).toContain('« Arbor » et « facturation »')
    expect(out).toContain('cité par 2 pages retenues')
    expect(out).toContain('1/2 retenus')
    expect(out).toContain('1/1 retenus')
    expect(out).toContain('Tout cocher') // Confluence, one unticked
    expect(out).toContain('Tout décocher') // LeanIX, all ticked
    expect(out).toContain('Sélection (2)') // the ticked ones
    const struck = out.slice(out.indexOf('data-guide-name="p2"'))
    expect(struck.slice(0, struck.indexOf('>Notes<'))).toContain('line-through')
    const kept = out.slice(out.indexOf('data-guide-name="p1"'))
    expect(kept.slice(0, kept.indexOf('>ERP Arbor<'))).not.toContain('line-through')
    expect(out).not.toContain('Retirer</') // ticked in place, not removed
  })

  test('a plain picker shows the why line too, and keeps « Retirer »', () => {
    const out = html({ kind: 'picker', search: 'x:s', selected: [proposed[0]] })
    expect(out).toContain('data-guide="picker.why"')
    expect(out).toContain('Retirer')
    expect(out).not.toContain('retenus')
  })

  test('an item already held is a checked, disabled box where it is listed', async () => {
    const { PickerAddControl, PICKER_LABELS } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const draw = (held: boolean) => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(PickerAddControl, { held, off: false, labels: PICKER_LABELS, onAdd: () => {} })))
    const heldBox = draw(true)
    expect(heldBox).toContain('picker.held-box')
    expect(heldBox).toContain('checked')
    expect(heldBox).toContain('disabled')
    expect(heldBox).toContain('déjà retenue')
    expect(draw(false)).toContain('aria-label="Ajouter"')
    expect(draw(false)).not.toContain('picker.held-box')
  })

  test('an unreachable source offers « Réessayer », a source not configured does not', async () => {
    const { PickerBanner, PICKER_LABELS, statusBanner } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const draw = (status: object, onRetry?: () => void) => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(PickerBanner, { banner: statusBanner(status as any, PICKER_LABELS)!, status: status as any, labels: PICKER_LABELS, onRetry })))
    const down = { configured: true, reachable: false, message: 'délai dépassé après 15 s' }
    expect(draw(down, () => {})).toContain('Réessayer')
    expect(draw(down, () => {})).toContain('délai dépassé après 15 s')
    expect(draw(down)).not.toContain('Réessayer') // nothing to ask again
    expect(draw({ configured: false }, () => {})).not.toContain('Réessayer')
  })

  test('the node declares tickable and held', () => {
    const def = (UIPageNodeJSONSchema as any).definitions.Picker
    expect(def.properties.tickable.type).toBe('boolean')
    expect(def.properties.held.items.type).toBe('string')
    expect(def.properties.selected.items.properties.off.type).toBe('boolean')
    expect(def.properties.selected.items.properties.group.type).toBe('string')
  })
})

describe('the tickable picker as a tree, its states and its badge', () => {
  const tree = [
    { key: 'R', title: 'ERP Arbor', with_children: true },
    { key: 'F', title: 'Interfaces facturation', parent: 'R' },
    { key: 'G', title: 'Flux', parent: 'F' },
    { key: 'O', title: 'Other' },
  ]
  test('unticking a parent unticks what sits under it; apply never gets a child without its parent', async () => {
    const { toggleItem, applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const off = toggleItem(tree, 'R')
    expect(off.filter((s) => s.off).map((s) => s.key)).toEqual(['R', 'F', 'G'])
    expect((applyInput({ tickable: true }, off).items as any[]).map((i) => i.key)).toEqual(['O'])
  })
  test('unticking a child takes « avec les sous-éléments » off its parent, ticking it ticks its parents back', async () => {
    const { toggleItem, applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const one = toggleItem(tree, 'G')
    expect(one.find((s) => s.key === 'R')!.with_children).toBeUndefined()
    expect(one.find((s) => s.key === 'F')!.off).toBeUndefined()
    const parentsOff = toggleItem(toggleItem(tree, 'R'), 'G')
    expect(parentsOff.filter((s) => s.off).map((s) => s.key)).toEqual([])
    const sent = (applyInput({ tickable: true, args: { id: 'm.s' }, object: 'm' }, toggleItem(tree, 'F')) as any)
    expect(sent.id).toBe('m.s')
    expect(sent.object).toBe('m')
    expect(sent.items.map((i: any) => i.key)).toEqual(['R', 'O'])
    expect(sent.items[0].with_children).toBeUndefined()
  })
  test('a plain picker hands back its whole selection', async () => {
    const { applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect((applyInput({}, [{ key: 'a', title: 'A', off: true }]).items as any[]).length).toBe(1)
  })
  test('« Décocher les IA » unticks, never removes, and counts the ticked ones', async () => {
    const { untickBadge } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const sel = [{ key: 'a', title: 'A', badges: ['ai'] }, { key: 'b', title: 'B' }]
    const out = untickBadge(sel, 'ai')
    expect(out.map((s) => [s.key, !!s.off])).toEqual([['a', true], ['b', false]])
    const node = { kind: 'picker', search: 'x:s', tickable: true, removeBadge: 'ai', badges: { ai: { label: 'IA' } } }
    const draw = (selected: object[]) => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({}) } as any }, createElement(PickerRenderer, { node: { ...node, selected } as any }))))
    const html = draw([...sel, { key: 'c', title: 'C', badges: ['ai'], off: true }])
    expect(html).toContain('dont 1 IA')
    expect(html).toContain('Décocher les IA')
    expect(html).not.toContain('Retirer les IA')
    expect(draw(out)).not.toContain('dont') // none ticked left
  })
  test('a tickable item keeps its state badge, its why the tooltip', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: {} as any }, createElement(PickerRenderer, { node: { kind: 'picker', search: 'x:s', tickable: true,
        selected: [{ key: 'a', title: 'A', state: 'failed', why: 'denied' }, { key: 'b', title: 'B', why: 'found by name' }] } as any }))))
    expect(html).toContain('en échec')
    expect(html).toContain('found by name')
    expect(html).not.toContain('>denied<') // the state's why is its tooltip, not a line
  })
})

describe('what a tickable tree hands back, whatever moved the ticks', () => {
  const tree = [
    { key: 'R', title: 'ERP Arbor', with_children: true, group: 'A', badges: ['ai'] },
    { key: 'F', title: 'Interfaces facturation', parent: 'R', group: 'B' },
    { key: 'G', title: 'Flux', parent: 'F', group: 'B', badges: ['ai'] },
    { key: 'O', title: 'Other', group: 'A' },
  ]
  const keys = async (sel: any[]) => {
    const { applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    return (applyInput({ tickable: true }, sel).items as any[]).map((i) => i.key + (i.with_children ? '+' : ''))
  }
  test('« Tout décocher » on the parent group hands back no orphan child; « Tout cocher » on the child group ticks its parent back', async () => {
    const { tickGroup } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(await keys(tickGroup(tree, 'A', false))).toEqual([])
    const parentOff = tickGroup(tree, 'A', false)
    // « Tout cocher » on the child group ticks their parent back: never an orphan.
    expect(await keys(tickGroup(parentOff, 'B', true))).toEqual(['R+', 'F', 'G'])
  })
  test('re-ticking a parent alone hands it back without « avec les sous-éléments »', async () => {
    const { toggleItem } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const back = toggleItem(toggleItem(tree, 'R'), 'R')
    expect(back.find((s) => s.key === 'F')!.off).toBe(true)
    expect(await keys(back)).toEqual(['R', 'O'])
  })
  test('« Décocher les IA » over a tree: a badged parent takes its children, a badged child takes with_children off its parents', async () => {
    const { untickBadge } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect(await keys(untickBadge(tree, 'ai'))).toEqual(['O'])
    const childOnly = untickBadge(tree.map((s) => (s.key === 'R' ? { ...s, badges: [] } : s)), 'ai')
    expect(await keys(childOnly)).toEqual(['R', 'F', 'O'])
  })
  test('everything ticked: the tree goes back whole, with_children kept', async () => {
    expect(await keys(tree)).toEqual(['R+', 'F', 'G', 'O'])
  })
  test('a parent absent from the selection is not judged', async () => {
    expect(await keys([{ key: 'F', title: 'F', parent: 'missing' }])).toEqual(['F'])
  })
})

describe('a tickable picker shows ticked exactly what it hands back', () => {
  const sel = [
    { key: 'R', title: 'ERP Arbor', group: 'A', off: true },
    { key: 'F', title: 'Interfaces facturation', parent: 'R', group: 'B' },
    { key: 'O', title: 'Other', group: 'B' },
  ]
  test('the counter, the checkboxes and « Sélection » agree with what apply receives', async () => {
    const { applyInput, pickerGroups } = await import('../src/ui-builders/page/nodes/picker.tsx')
    expect((applyInput({ tickable: true }, sel).items as any[]).map((i) => i.key)).toEqual(['O'])
    expect(pickerGroups(sel).map((g) => [g.group, g.ticked, g.items.length])).toEqual([['A', 0, 1], ['B', 1, 2]])
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: {} as any }, createElement(PickerRenderer, { node: { kind: 'picker', search: 'x:s', tickable: true, selected: sel } as any }))))
    expect(html).toContain('1/2 retenus')
    expect(html).not.toContain('2/2 retenus')
    expect(html).toContain('Sélection (1)')
    const row = (key: string) => { const at = html.indexOf(`data-guide-name="${key}"`); return html.slice(at, html.indexOf('</div></div></div>', at)) }
    expect(row('F')).toContain('data-off="true"') // left out by its unticked parent
    expect(row('F')).not.toMatch(/<input[^>]*checked/)
    expect(row('O')).toMatch(/<input[^>]*checked/)
  })
  test('« Tout cocher » on a group under an unticked parent of another group sends the group whole', async () => {
    const { tickGroup, pickerGroups, applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const all = tickGroup(sel, 'B', true)
    expect((applyInput({ tickable: true }, all).items as any[]).map((i) => i.key)).toEqual(['R', 'F', 'O'])
    expect(pickerGroups(all).map((g) => [g.group, g.ticked, g.items.length])).toEqual([['A', 1, 1], ['B', 2, 2]])
  })
  test('ticking an item its unticked ancestor leaves out ticks the ancestor back', async () => {
    const { toggleItem, applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const back = toggleItem(sel, 'F', true)
    expect((applyInput({ tickable: true }, back).items as any[]).map((i) => i.key)).toEqual(['R', 'F', 'O'])
  })
})

describe('a picker says who applies', () => {
  test('the host person rides with the apply as by; no person, none', async () => {
    const { applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    const sel = [{ key: 'P1', title: 'ERP Arbor' }]
    const by = { id: 'jdoe', kind: 'person' }
    const withPerson = applyInput({ args: { connector: 'confluence' }, object: 'etude' }, sel, by) as any
    expect(withPerson.by).toEqual(by)
    expect(withPerson.items).toEqual([{ key: 'P1', title: 'ERP Arbor' }])
    expect('by' in (applyInput({}, sel) as any)).toBe(false)
    expect('by' in (applyInput({}, sel, { id: '', kind: 'person' }) as any)).toBe(false)
  })
})

// The single-choice tree - one item chosen at a time,
// replacing the one held, never with what sits under it; the tree browsed
// level by level; apply handed the one choice.
describe('the single-choice picker', () => {
  test('picking replaces the one held; an unreachable item is never picked', async () => {
    const { pickInto, applyInput } = await import('../src/ui-builders/page/nodes/picker.tsx')
    let sel = pickInto([], { ...space, with_children: true }, 'single')
    expect(sel).toEqual([space])
    sel = pickInto(sel, page, 'single')
    expect(sel.map((s) => s.key)).toEqual(['P1'])
    expect(pickInto(sel, dead, 'single')).toBe(sel)
    expect(pickInto(sel, child).map((s) => s.key)).toEqual(['P1', 'P2'])
    expect(applyInput({ object: 'pub' }, sel).items).toEqual([{ key: 'P1', title: 'ERP Arbor', parent: 'S' }])
  })

  const render = (host: object, node: object) => renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))

  test('draws « Choisir » and browse, never « avec les sous-éléments »', () => {
    const node = { kind: 'picker', browse: 'x:browse', apply: 'x:apply', layout: 'tree', choice: 'single', selected: [page] }
    const single = render({ callMethod: async () => ({ items: [] }) }, node)
    expect(single).toContain('ERP Arbor')
    expect(single).toContain('Parcourir')
    expect(single).not.toContain('avec les sous-éléments')
    // Several items: an item taken with its children says so.
    const multi = render({ callMethod: async () => ({ items: [] }) }, { ...node, choice: undefined, selected: [{ ...page, has_children: true, with_children: true }] })
    expect(multi).toContain('avec les sous-éléments')
  })

  test('the schema accepts choice single', () => {
    const def = (UIPageNodeJSONSchema as any).definitions.Picker
    expect(def.properties.choice.enum).toEqual(['single', 'multiple'])
  })
})
