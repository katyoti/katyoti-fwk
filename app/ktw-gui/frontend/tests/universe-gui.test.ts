// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { firstByHashtags, isLeftOut, viewAllows } from '../src/sections/shell/universeGui'
import { objectActionId, objectSectionId } from '../src/elements'

// A universe's Univers.yaml `gui` section (carried by the hub's universe
// summary): Home lists the tasks with a `first` hashtag before the others. Its
// `hide` is retired: what a universe shows is its
// view at the person's level and the level's own Essentiel rule.
const gui = { first: ['archifactory'] }
const ids = (xs: { id: string }[]) => xs.map((x) => x.id)

describe('what a universe shows', () => {
  const untagged = { all: true, allowed: [] }
  test('an untagged universe: only the Essentiel rule leaves out', () => {
    expect(isLeftOut('rail:debug', 'essential', untagged)).toBe(true)
    expect(isLeftOut('rail:debug', 'standard', untagged)).toBe(false)
    expect(isLeftOut('home:planning-deck', 'standard', untagged)).toBe(false)
  })
  test('a tagged universe shows what its view allows', () => {
    const view = { all: false, allowed: ['home:errors', 'home:objects'] }
    expect(isLeftOut('home:errors', 'expert', view)).toBe(false)
    expect(isLeftOut('home:errors', 'standard', view)).toBe(false)
    expect(isLeftOut('home:portfolio', 'expert', view)).toBe(true)
  })
  // The tags say at which level each element shows - an element
  // the view allows at Essentiel shows there, whatever the Essentiel rule.
  test('a tagged view decides the places it lists at Essentiel too', () => {
    const view = { all: false, allowed: ['tab:actions', 'tab:information'] }
    expect(isLeftOut('tab:actions', 'essential', view)).toBe(false)
    expect(isLeftOut('tab:lifecycle', 'essential', view)).toBe(true)
    expect(isLeftOut('badge:aspect', 'essential', view)).toBe(true) // not a view place: the Essentiel rule
    expect(isLeftOut('tab:lifecycle', 'essential', untagged)).toBe(true) // without tags: the Essentiel rule
  })
})

describe('the tasks with a first hashtag', () => {
  const tasks = [
    { id: 'a', hashtags: ['dev'] },
    { id: 'b', hashtags: ['archifactory', 'dossier'] },
    { id: 'c' },
    { id: 'd', hashtags: ['archifactory'] },
  ]
  test('come before the others, each part in its order', () => {
    expect(ids(firstByHashtags(tasks, gui.first))).toEqual(['b', 'd', 'a', 'c'])
  })
  test('without a first hashtag the order is kept', () => {
    expect(ids(firstByHashtags(tasks, undefined))).toEqual(['a', 'b', 'c', 'd'])
  })
})

describe('the GUI reads it', () => {
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('Home filters its strip and orders its tasks', () => {
    const home = src('../src/sections/home/HomeTab.tsx')
    expect(home).toContain('allEntries.filter((e) => !universeGui.hidden(homeElementId(e.id)))')
    expect(home).toContain('firstTags={universeGui.first}')
    expect(src('../src/sections/workspace/SearchableCardGrid.tsx')).toContain('firstByHashtags(sortItems(narrowed.items, sort), firstTags)')
  })
  test('the rail filters each item it can hide; the profile menu sets the display level', () => {
    const ribbon = src('../src/sections/shell/IconRibbon.tsx')
    for (const id of ['chat', 'inbox', 'marketplace', 'debug', 'dev-console', 'camera', 'record']) {
      expect(ribbon).toContain(`{!hidden('rail:${id}') && (`)
    }
    expect(ribbon).toContain("t(`profile.level.${l}`)")
  })
})

// An object's sections and actions follow the universe's view as
// its tabs do.
describe('sections and actions follow the view', () => {
  const view = { all: false, allowed: ['tab:information', 'section:fields', 'action:ktw-agents/objects/rename'] }
  test('only those the view lists, at every level', () => {
    expect(viewAllows('tab:lifecycle', view)).toBe(false)
    expect(viewAllows('section:fields', view)).toBe(true)
    expect(viewAllows('section:planning', view)).toBe(false)
    expect(viewAllows(objectActionId('ktw-agents:objects/rename'), view)).toBe(true)
    expect(viewAllows(objectActionId('ktw-agents:plans/finish'), view)).toBe(false)
    expect(isLeftOut('action:ktw-agents/plans/finish', 'expert', view)).toBe(true)
  })
  test('a universe without tags shows them all', () => {
    expect(viewAllows('section:planning', { all: true, allowed: [] })).toBe(true)
  })
  test('their element ids', () => {
    expect(objectSectionId('section:fields')).toBe('section:fields')
    expect(objectSectionId('panel:planning')).toBe('section:planning')
    expect(objectSectionId('tab:planning')).toBe('section:planning')
    expect(objectActionId('ktw-agents:plans/finish')).toBe('action:ktw-agents/plans/finish')
  })
})
