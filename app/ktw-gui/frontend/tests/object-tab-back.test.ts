// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { MAX_BACK, backTarget, type BackablePayload, type StripState } from '../src/sections/workspace/objectTabHistory'
import { openObjectTab, closeObjectTab } from '../src/sections/workspace/openObjectTab'
import { useTabsStore } from '../src/store/tabsSlice'
import { forgetPanels, recall, remember } from '../src/sections/workspace/panelMemory'

// Opening a request or a decision from a panel (a module's Planification
// panel in the Object tab, a Global panel, the Inbox) must never lose that
// panel: closing the request brings the person back to it as it was.

type Tab = { id: string; label: string; payload?: unknown }
const U = 'u'

// The real wiring: openObjectTab and closeObjectTab over the tabs store. A
// strip is seeded into the store, the move made, the strip read back.
function seed(strip: StripState): void {
  const tabs = strip.openTabs.map((t) => ({ kind: t.id === 'home' ? 'home' : t.id === 'inbox' ? 'inbox' : 'object', closeable: t.id !== 'home', openedAt: 0, lastActivityAt: 0, ...t }))
  useTabsStore.setState({ byUniverse: new Map([[U, { openTabs: tabs as never, activeTabId: strip.activeTabId }]]) })
}
function read(): StripState {
  return useTabsStore.getState().getForUniverse(U)
}
function open(strip: StripState, next: BackablePayload): StripState {
  seed(strip)
  openObjectTab(U, { id: next.objectId, ...(next.objectTitle ? { title: next.objectTitle } : {}) })
  return read()
}
function close(strip: StripState): StripState {
  seed(strip)
  closeObjectTab(U)
  return read()
}

const shown = (s: StripState) => s.openTabs.find((t) => t.id === 'object')?.payload as BackablePayload | undefined
const home: Tab = { id: 'home', label: 'Global' }
const inbox: Tab = { id: 'inbox', label: 'Inbox' }

describe('closing an object opened from a panel returns to that panel', () => {
  test("from a module's Planification panel: the module, on that panel, with its filter", () => {
    const module: BackablePayload = { objectId: 'gui', objectTitle: 'GUI', activeKind: 'panel:planification', panelQuery: { panel: 'panel:planification', query: { state: 'open' } } }
    let s: StripState = { openTabs: [home, { id: 'object', label: 'Object: GUI', payload: module }], activeTabId: 'object' }
    s = open(s, { objectId: 'm.RQ-12' })
    expect(shown(s)?.objectId).toBe('m.RQ-12')
    expect(backTarget(s)?.label).toBe('GUI')
    s = close(s)
    expect(s.activeTabId).toBe('object')
    expect(shown(s)).toEqual({ ...module, back: [] })
    // Nothing left to go back to: the next close closes the tab.
    s = close(s)
    expect(s.openTabs.some((t) => t.id === 'object')).toBe(false)
  })

  test('from a Global panel, the Object tab not open: it closes onto Global', () => {
    let s: StripState = { openTabs: [home, inbox], activeTabId: 'home' }
    s = open(s, { objectId: 'notes.RQ-108' })
    expect(backTarget(s)?.from).toBe('home')
    s = close(s)
    expect(s.activeTabId).toBe('home')
    expect(s.openTabs.some((t) => t.id === 'object')).toBe(false)
  })

  test('from the Inbox, the Object tab showing another object: back on the Inbox, that object kept', () => {
    const before: BackablePayload = { objectId: 'shop.api', objectTitle: 'Shop API', activeKind: 'panel:files' }
    let s: StripState = { openTabs: [home, { id: 'object', label: 'Object: Shop API', payload: before }, inbox], activeTabId: 'inbox' }
    s = open(s, { objectId: 'm.RQ-12' })
    s = close(s)
    expect(s.activeTabId).toBe('inbox')
    expect(shown(s)).toEqual({ ...before, back: [] })
    expect(s.openTabs.find((t) => t.id === 'object')?.label).toBe('Object: Shop API')
  })

  test('two objects deep, two closes walk back the same way', () => {
    let s: StripState = { openTabs: [home, { id: 'object', label: 'Object: GUI', payload: { objectId: 'gui', activeKind: 'panel:planification' } }], activeTabId: 'object' }
    s = open(s, { objectId: 'm.RQ-12' })
    s = open(s, { objectId: 'm.F-4' })
    s = close(s)
    expect(shown(s)?.objectId).toBe('m.RQ-12')
    s = close(s)
    expect(shown(s)?.objectId).toBe('gui')
    expect(shown(s)?.activeKind).toBe('panel:planification')
  })

  test('the tab it came from is gone: the Object tab shows the step back itself', () => {
    let s: StripState = { openTabs: [home, { id: 'object', label: 'Object: A', payload: { objectId: 'a' } }, inbox], activeTabId: 'inbox' }
    s = open(s, { objectId: 'b' })
    s = { ...s, openTabs: s.openTabs.filter((t) => t.id !== 'inbox') }
    s = close(s)
    expect(s.activeTabId).toBe('object')
    expect(shown(s)?.objectId).toBe('a')
  })

  test('re-opening the object shown, from the tab itself, records no step and keeps its panel', () => {
    let s: StripState = { openTabs: [home, { id: 'object', label: 'Object: GUI', payload: { objectId: 'gui', activeKind: 'panel:planification' } }], activeTabId: 'object' }
    s = open(s, { objectId: 'gui' })
    expect(shown(s)?.back ?? []).toEqual([])
    expect(shown(s)?.activeKind).toBe('panel:planification')
  })

  test('the history is bounded', () => {
    let s: StripState = { openTabs: [home], activeTabId: 'home' }
    for (let i = 0; i < MAX_BACK + 5; i++) s = open(s, { objectId: 'o' + i })
    expect(shown(s)?.back?.length).toBe(MAX_BACK)
  })
})

describe('a panel outside the Object tab is found as it was left', () => {
  beforeEach(() => forgetPanels())
  test('what a panel remembers comes back; another panel has its own', () => {
    remember('global|u|planification', 'target', 'gui')
    remember('inbox|u', 'expanded', 'item-3')
    expect(recall('global|u|planification', 'target', '__all__')).toBe('gui')
    expect(recall('inbox|u', 'expanded', null)).toBe('item-3')
    expect(recall('global|v|planification', 'target', '__all__')).toBe('__all__')
  })
})
