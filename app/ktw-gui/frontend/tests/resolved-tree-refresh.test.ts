// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { pageIdentity, shownResolution, type Resolution } from '../src/sections/workspace/resolvePanel'

// « À faire »'s cards are schema-bound, so its page tree is
// resolved by the object pane - and it changes on every universe refresh. The
// page must stay mounted across a refresh (its kind filter, its bulk
// selection, a half-typed answer), never swapped for a loader.
describe('a resolved page tree across a refresh', () => {
  const first = { kind: 'page', cards: ['a'] }
  const firstKey = JSON.stringify(first)
  const refreshedKey = JSON.stringify({ kind: 'page', cards: ['a', 'b'] })
  const resolvedFirst = { kind: 'page', cards: ['a'], resolved: true }
  const resolvedNext = { kind: 'page', cards: ['a', 'b'], resolved: true }

  test('shows nothing (the loader) only before the first tree resolved', () => {
    expect(shownResolution(firstKey, { key: '' })).toEqual({})
  })
  test('shows the tree once it resolved', () => {
    expect(shownResolution(firstKey, { key: firstKey, tree: resolvedFirst }).tree).toBe(resolvedFirst)
  })
  test('keeps the last resolved tree while the refreshed one resolves', () => {
    const got: Resolution = { key: firstKey, tree: resolvedFirst }
    const shown = shownResolution(refreshedKey, got)
    expect(shown.tree).toBe(resolvedFirst)
    expect(shown.error).toBeUndefined()
  })
  test('shows the refreshed tree once it resolved', () => {
    expect(shownResolution(refreshedKey, { key: refreshedKey, tree: resolvedNext }).tree).toBe(resolvedNext)
  })
  test('an error shows for its own tree only', () => {
    expect(shownResolution(firstKey, { key: firstKey, error: 'boom' })).toEqual({ tree: undefined, error: 'boom' })
    expect(shownResolution(refreshedKey, { key: firstKey, error: 'boom' })).toEqual({})
  })
})

// A tree is kept only for the SAME page: a direct mount (ObjectTab,
// PanelDrawer, GlobalPanels) that switches to another object, panel or
// universe must never show the last page's tree - its actions would target
// the new scope.
describe('a kept tree is only for the same page', () => {
  const todo = pageIdentity('u1', 'inbox', undefined, 'todo')
  const resolvedTodo = { kind: 'page', cards: ['a'], resolved: true }
  const got: Resolution = { identity: todo, key: 'k1', tree: resolvedTodo }
  test('the same page with new content keeps its last tree', () => {
    expect(shownResolution('k2', got, pageIdentity('u1', 'inbox', undefined, 'todo')).tree).toBe(resolvedTodo)
  })
  test('another object keeps nothing', () => {
    expect(shownResolution('k2', got, pageIdentity('u1', 'gates', undefined, 'todo'))).toEqual({})
  })
  test('another panel keeps nothing', () => {
    expect(shownResolution('k2', got, pageIdentity('u1', 'inbox', undefined, 'done'))).toEqual({})
  })
  test('another universe keeps nothing', () => {
    expect(shownResolution('k2', got, pageIdentity('u2', 'inbox', undefined, 'todo'))).toEqual({})
  })
  test('another scope (a global panel target) keeps nothing', () => {
    const all = pageIdentity('u1', '', { target: '' }, 'src')
    const one = pageIdentity('u1', '', { target: 'inbox' }, 'src')
    expect(shownResolution('k2', { identity: all, key: 'k1', tree: resolvedTodo }, one)).toEqual({})
  })
  test('even the same tree key resolved for another page shows nothing', () => {
    expect(shownResolution('k1', got, pageIdentity('u1', 'gates', undefined, 'todo'))).toEqual({})
  })
})

describe('the panel draws it', () => {
  test('RenderDefView mounts PageHost whenever a tree is shown - the loader only when none is', () => {
    const src = readFileSync(new URL('../src/sections/workspace/RenderDefView.tsx', import.meta.url), 'utf8')
    // Resolved through the host's one load: the page's identity
    // keeps its last tree while a changed one resolves (useHostLoad).
    expect(src).toContain("universe: universeName, identity: `resolve|${identity}`, key: `resolve|${key}`, enabled: needs,")
    expect(src).toContain('if (!resolved.tree) return <Center p="md"><Loader size="sm" /></Center>')
  })
})

describe('the direct mounts remount on another page', () => {
  const read = (f: string) => readFileSync(new URL(f, import.meta.url), 'utf8')
  test.each([
    ['../src/sections/workspace/ObjectTab.tsx', 'key={pageIdentity(universeName, sendId, undefined, activeTab.id)}'],
    ['../src/sections/workspace/ObjectTab.tsx', 'key={pageIdentity(universeName, sendId, undefined, p.id)}'],
    ['../src/sections/workspace/PanelDrawer.tsx', 'key={pageIdentity(universeName, sendId, undefined, panel)}'],
    ['../src/sections/home/GlobalPanels.tsx', "key={pageIdentity(universeName, '', undefined, panel.id)}"],
  ])('%s keys its RenderDefView by its page', (file, key) => {
    expect(read(file)).toContain(key)
  })
})
