// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { isLeftOut, viewAllows, type ViewLists } from '../src/sections/shell/universeGui'
import { viewLevel } from '../src/ktsp/universeView'

// The GUI shows what the universe's resolved view allows at the
// person's level. A universe whose tags list items shows exactly those; an
// untagged one (`all`) or one whose view could not be read shows everything.
// The view answers the element ids it allows; the GUI asks
// by element id - no prefix stripped, no other spelling.
const essentiel: ViewLists = {
  all: false,
  allowed: ['rail:chat', 'rail:inbox', 'home:portfolio', 'quick:term', 'quick:task/ktw-methods/new-dossier', 'new:dossier', 'tab:links'],
}
const standard: ViewLists = { ...essentiel, allowed: [...essentiel.allowed, 'rail:marketplace', 'new:decision'] }

type Place = 'rail' | 'home' | 'quick' | 'tab' | 'new'
const offered: Record<Place, string[]> = {
  rail: ['rail:chat', 'rail:inbox', 'rail:marketplace', 'rail:debug', 'rail:camera'],
  home: ['home:actions', 'home:runs', 'home:portfolio', 'home:planning-deck'],
  quick: ['quick:term', 'quick:drawio', 'quick:debug', 'quick:task/ktw-methods/new-dossier', 'quick:task/ktw-methods/other'],
  tab: ['tab:links', 'tab:history'],
  new: ['new:dossier', 'new:decision', 'new:goal'],
}
const shown = (place: Place, view: ViewLists | null | undefined) => offered[place].filter((id) => viewAllows(id, view))

describe('a universe tagged with allow-lists', () => {
  test('shows exactly its items at Essentiel', () => {
    expect(shown('rail', essentiel)).toEqual(['rail:chat', 'rail:inbox'])
    expect(shown('home', essentiel)).toEqual(['home:actions', 'home:portfolio'])
    expect(shown('quick', essentiel)).toEqual(['quick:term', 'quick:task/ktw-methods/new-dossier'])
    expect(shown('tab', essentiel)).toEqual(['tab:links'])
    expect(shown('new', essentiel)).toEqual(['new:dossier'])
  })
  test('shows what the higher level adds', () => {
    expect(shown('rail', standard)).toEqual(['rail:chat', 'rail:inbox', 'rail:marketplace'])
    expect(shown('new', standard)).toEqual(['new:dossier', 'new:decision'])
  })
  test('Home\'s actions always show', () => {
    expect(viewAllows('home:actions', { all: false, allowed: [] })).toBe(true)
  })
  test('an element of a place the view does not list shows', () => {
    expect(viewAllows('badge:aspect', essentiel)).toBe(true)
  })
})

describe('a universe without a tag', () => {
  test('shows everything, and so does one whose view could not be read', () => {
    const all = { ...essentiel, all: true }
    for (const place of Object.keys(offered) as Place[]) {
      expect(shown(place, all)).toEqual(offered[place])
      expect(shown(place, null)).toEqual(offered[place])
      expect(shown(place, undefined)).toEqual(offered[place])
    }
  })
})

describe('the GUI asks the view at the person\'s level', () => {
  test('in the view\'s words', () => {
    expect(viewLevel('essential')).toBe('essentiel')
    expect(viewLevel('standard')).toBe('standard')
    expect(viewLevel('expert')).toBe('expert')
  })
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('each surface asks by its element id', () => {
    expect(src('../src/sections/shell/IconRibbon.tsx')).toContain('universeGui.hidden(id)')
    const home = src('../src/sections/home/HomeTab.tsx')
    expect(home).toContain('.filter((e) => !universeGui.hidden(homeElementId(e.id)))')
    expect(home).toContain('universeGui.hidden(`quick:${a.id}`)')
    expect(home).toContain("universeGui.hidden('quick:start-chat')")
    expect(home).toContain("const defaults = view && !view.all ? view.allowed.filter((id) => id.startsWith('quick:task/')) : undefined")
    expect(src('../src/sections/workspace/ObjectTab.tsx')).toContain('hidden(tabSurface(tb.id))')
    expect(src('../src/sections/workspace/ObjectCreateModal.tsx')).toContain('offeredChoices(choices, presetType, (id) => !hidden(id))')
    expect(src('../src/sections/workspace/createChoices.ts')).toContain('allows(`new:${c.name}`)')
  })
})

describe('the view holds element ids only', () => {
  test('a former spelling is no longer an element', () => {
    const old: ViewLists = { all: false, allowed: ['chat', 'panel:portfolio', 'ktw-methods:tasks/new-dossier', 'links'] }
    for (const id of ['rail:chat', 'home:portfolio', 'quick:task/ktw-methods/new-dossier', 'tab:links']) expect(viewAllows(id, old)).toBe(false)
  })
})

// In a universe with tags the view applies at every level -
// Expert shows what the tags list up to expert (the view resolved at that
// level), nothing else; a universe without tags shows everything at Expert.
describe('a tagged universe at every level', () => {
  const expertView: ViewLists = { all: false, allowed: ['rail:chat', 'rail:marketplace', 'home:actions', 'home:mes-etudes', 'tab:lifecycle', 'new:etude-existant'] }
  test('Expert does not bring back what the tags leave out', () => {
    for (const id of ['home:person-queue', 'home:planning-deck', 'home:portfolio', 'tab:goals', 'new:module', 'quick:term']) {
      expect(isLeftOut(id, 'expert', expertView)).toBe(true)
    }
    for (const id of ['rail:marketplace', 'tab:lifecycle', 'home:mes-etudes']) expect(isLeftOut(id, 'expert', expertView)).toBe(false)
  })
  test('an untagged universe shows everything at Expert', () => {
    for (const id of ['home:person-queue', 'tab:goals', 'new:module']) expect(isLeftOut(id, 'expert', { all: true, allowed: [] })).toBe(false)
  })
  test('the GUI asks it at every level', () => {
    const hook = readFileSync(new URL('../src/hooks/useUniverseGui.ts', import.meta.url), 'utf8')
    expect(hook).toContain('useCallback((id: string) => isLeftOut(id, level, view), [level, view])')
    expect(hook).not.toContain('!expert && !viewAllows')
  })
  test('the object strip follows the view when the level changes, Expert or not', () => {
    // Essentiel to Standard leaves `expert` false: the strip's memo must
    // depend on `hidden` (new with the level's view) or it keeps the old list.
    const pane = readFileSync(new URL('../src/sections/workspace/ObjectTab.tsx', import.meta.url), 'utf8')
    expect(pane).toContain('), [detail, expert, hidden, payload.showMore, payload.activeKind])')
  })
})
