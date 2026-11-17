// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { ELEMENTS, ESSENTIAL_HIDDEN, PLACES, homeElementId, isElementId, objectTabId, placeOf, taskQuickId, taskURIOf } from '../src/elements'
import { NEVER_HIDDEN, isLeftOut, tabSurface } from '../src/sections/shell/universeGui'

// Every element the GUI draws has one id
// `<place>:<name>`, from one registry the GUI reads to draw, hide and guide.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')
const ids = ELEMENTS.map((e) => e.id)

describe('the registry', () => {
  test('every id is <place>:<name>, kebab, of a known place, once', () => {
    for (const id of ids) {
      expect(isElementId(id)).toBe(true)
      expect(PLACES).toContain(placeOf(id) as never)
    }
    expect(new Set(ids).size).toBe(ids.length)
    expect(isElementId('debug-and-tests')).toBe(false)
    expect(isElementId('quick:Debug')).toBe(false)
    expect(isElementId('panel:links')).toBe(false)
  })
  test('a project type is an element by project:<type>', () => {
    expect(isElementId('project:dossier-architecture')).toBe(true)
    expect(placeOf('project:software-project')).toBe('project')
  })
  test('a contributed element is built from its declaration', () => {
    expect(taskQuickId('ktw-acme:tasks/new-study')).toBe('quick:task/ktw-acme/new-study')
    expect(isElementId(taskQuickId('ktw-acme:tasks/new-study'))).toBe(true)
    expect(homeElementId('panel:mes-dossiers')).toBe('home:mes-dossiers')
    expect(homeElementId('runs')).toBe('home:runs')
  })
  test('a task\'s quick action names its task', () => {
    expect(taskURIOf('quick:task/ktw-acme/new-study')).toBe('ktw-acme:tasks/new-study')
    expect(taskURIOf('quick:debug')).toBe('')
  })
})

describe('every element the GUI draws is in it, and carries its id', () => {
  test('each rail control', () => {
    const ribbon = src('sections/shell/IconRibbon.tsx')
    const drawn = [...ribbon.matchAll(/data-element="([^"]+)"/g)].map((m) => m[1])
    expect(drawn.sort()).toEqual(ids.filter((id) => placeOf(id) === 'rail').sort())
  })
  test('each built-in quick action and Home strip entry', () => {
    const home = src('sections/home/HomeTab.tsx')
    const builtIns = [...home.matchAll(/^\s+id: '([a-z-]+)',$/gm)].map((m) => m[1])
    expect(builtIns.length).toBeGreaterThan(0)
    for (const id of builtIns) expect(ids).toContain(`quick:${id}`) // a new built-in needs an element
    expect(home).toContain("universeGui.hidden('quick:debug')")
    expect(home).toContain("id:          'quick:debug'")
    expect(home).toContain("universeGui.hidden('quick:start-chat')")
    expect(home).toContain("id:          'quick:start-chat'")
    expect(home).not.toContain('debug-and-tests')
    for (const entry of ['actions', 'runs', 'errors', 'chat']) expect(ids).toContain(homeElementId(entry))
    expect(src('sections/workspace/SearchableCardGrid.tsx')).toContain('data-element={isElementId(q.id) ? q.id : undefined}')
  })
  test('each badge and technical tab, checked where it is drawn', () => {
    const objectTab = src('sections/workspace/ObjectTab.tsx')
    expect(objectTab).toContain("hidden('badge:aspect')")
    expect(objectTab).toContain("hidden('badge:capability')")
    expect(objectTab).toContain('hidden(tabSurface(tb.id))')
    expect(src('sections/workspace/WorkspaceObjectsTree.tsx')).toContain("hidden('badge:tree')")
    for (const panel of ['links', 'agent-context', 'actions', 'lifecycle']) expect(ids).toContain(tabSurface(`panel:${panel}`))
  })
})

describe('leaving an element out', () => {
  test('a universe without tags shows every element at Standard and Expert', () => {
    for (const id of ids) {
      expect(isLeftOut(id, 'standard', null)).toBe(false)
      expect(isLeftOut(id, 'expert', { all: true, allowed: [] })).toBe(false)
    }
  })
  test('the elements never hidden show at every level, whatever the view', () => {
    for (const id of NEVER_HIDDEN) {
      for (const level of ['essential', 'standard', 'expert'] as const) {
        expect(isLeftOut(id, level, null)).toBe(false)
        expect(isLeftOut(id, level, { all: false, allowed: [] })).toBe(false)
      }
    }
  })
  test('Essentiel hides exactly the elements marked so', () => {
    for (const id of ids) expect(isLeftOut(id, 'essential', null)).toBe(ESSENTIAL_HIDDEN.has(id))
    expect([...ESSENTIAL_HIDDEN].sort()).toEqual([
      'badge:aspect', 'badge:capability', 'home:errors', 'home:runs', 'quick:debug',
      'rail:debug', 'rail:dev-console', 'rail:marketplace',
      'tab:agent-context', 'tab:lifecycle', 'tab:links',
    ])
  })
})

// An object's tabs are `tab:<stem>`, its children
// `tab:children/<type>`, its sections `section:fields` and
// `section:cap/<capability>`; every one can be opened by navigation.
describe('every tab of an object is reachable', () => {
  test('a reference names a stem, a full tab or section id, or a former panel id', () => {
    expect(objectTabId('links')).toBe('tab:links')
    expect(objectTabId('panel:links')).toBe('tab:links')
    expect(objectTabId('tab:children/record')).toBe('tab:children/record')
    expect(objectTabId('section:fields')).toBe('section:fields')
    expect(objectTabId('section:cap/sizing')).toBe('section:cap/sizing')
    for (const ref of ['links', 'tab:children/record', 'section:cap/sizing']) expect(isElementId(objectTabId(ref))).toBe(true)
  })
  test('the GUI opens and reads tabs by that id', () => {
    const open = src('sections/workspace/openObjectTab.tsx')
    expect(open).toContain('activeKind: objectTabId(target.panel)')
    expect(open).toContain('panelQuery: { panel: objectTabId(target.panel), query: target.query }')
    expect(open).not.toContain("'panel:' + target.panel")
    expect(src('sections/workspace/ObjectTab.tsx')).toContain('payload.activeKind && objectTabId(payload.activeKind)')
    expect(src('sections/workspace/ObjectCorePanels.tsx')).toContain('t.id === objectTabId(decl.panel!)')
  })
  test('a global panel is a Home element by its stem', () => {
    expect(homeElementId('home:objects')).toBe('home:objects')
    expect(homeElementId('panel:objects')).toBe('home:objects')
  })
})

// The controls the
// usage guide points at carry their id and anchor; a control that does
// nothing yet is not shown.
describe('the controls of the approved proposal', () => {
  const ribbon = src('sections/shell/IconRibbon.tsx')
  test('the avatar, the profile entries, the camera and « + » carry their id and anchor', () => {
    for (const id of ['rail:profile', 'rail:add-universe']) expect(ids).toContain(id)
    expect(ribbon).toContain('data-element="rail:profile"')
    expect(ribbon).toContain('data-guide="profile.open"')
    expect(ribbon).toContain("{!hidden('rail:add-universe') && <AddUniverseMenu")
    for (const a of ['ribbon.camera', 'profile-menu.profile', 'profile-menu.appearance', 'profile-menu.feedback']) expect(ribbon).toContain(`data-guide="${a}"`)
  })
  test('the rail search, « Se déconnecter » and the chat microphone are not shown', () => {
    expect(ids).not.toContain('rail:search')
    expect(ribbon).not.toContain('rail:search')
    expect(ribbon).not.toContain("t('profile.signOut')")
    expect(src('sections/agent/ChatInput.tsx')).not.toContain('IconMicrophone')
  })
})
