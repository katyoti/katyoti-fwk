// SPDX-License-Identifier: Apache-2.0
import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { PANEL_MEMORY_KEY, forgetPanels, recall, reloadPanels, remember } from '../src/sections/workspace/panelMemory'

// A Global panel's chosen query (its target, its page's filter: « À faire »
// on one action) survives a reload of the GUI: the panel memory is mirrored
// to localStorage, and storage that is absent, corrupt or refusing writes
// never breaks the panel — the memory then lives in memory only.

class FakeStorage {
  items = new Map<string, string>()
  refuse = false
  getItem(k: string): string | null { return this.items.get(k) ?? null }
  setItem(k: string, v: string): void { if (this.refuse) throw new Error('QuotaExceededError'); this.items.set(k, v) }
  removeItem(k: string): void { this.items.delete(k) }
}

const g = globalThis as { localStorage?: unknown }
const before = g.localStorage
let store: FakeStorage
const memo = 'global|u|panel:person-queue'

beforeEach(() => {
  store = new FakeStorage()
  g.localStorage = store
  forgetPanels()
})
afterAll(() => { g.localStorage = before; forgetPanels() })

describe('the panel memory across a reload', () => {
  test('a remembered query is read back after a reload', () => {
    remember(memo, 'target', 'gui')
    remember(memo, 'queries', { gui: { action: 'decide' } })
    expect(store.getItem(PANEL_MEMORY_KEY)).toContain('decide')
    reloadPanels()
    expect(recall(memo, 'target', '')).toBe('gui')
    expect(recall<Record<string, Record<string, unknown>>>(memo, 'queries', {})).toEqual({ gui: { action: 'decide' } })
  })

  test('« Tout » chosen again is what the reload finds', () => {
    remember(memo, 'queries', { __all__: { action: 'test' } })
    remember(memo, 'queries', { __all__: { action: '' } })
    reloadPanels()
    expect(recall(memo, 'queries', {})).toEqual({ __all__: { action: '' } })
  })

  test('corrupt storage reads as nothing remembered, without throwing', () => {
    for (const junk of ['{not json', '[1,2]', '"text"', 'null', '{"global|u|p": 3}']) {
      store.items.set(PANEL_MEMORY_KEY, junk)
      expect(() => reloadPanels()).not.toThrow()
      expect(recall('global|u|p', 'target', 'none')).toBe('none')
    }
  })

  test('absent storage: kept in memory, nothing thrown', () => {
    delete g.localStorage
    expect(() => remember(memo, 'target', 'gui')).not.toThrow()
    expect(recall(memo, 'target', '')).toBe('gui')
    expect(() => reloadPanels()).not.toThrow()
    expect(recall(memo, 'target', 'gone')).toBe('gone')
  })

  test('storage refusing writes: kept in memory, nothing thrown', () => {
    store.refuse = true
    expect(() => remember(memo, 'target', 'gui')).not.toThrow()
    expect(recall(memo, 'target', '')).toBe('gui')
  })

  test('storage whose access throws: kept in memory, nothing thrown', () => {
    Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('SecurityError') } })
    try {
      expect(() => remember(memo, 'target', 'gui')).not.toThrow()
      expect(recall(memo, 'target', '')).toBe('gui')
      expect(() => reloadPanels()).not.toThrow()
    } finally {
      Object.defineProperty(globalThis, 'localStorage', { configurable: true, writable: true, value: store })
    }
  })
})
