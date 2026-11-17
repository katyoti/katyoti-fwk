// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { DEFAULT_SETTINGS, mergeLoadedSettings } from '../src/types/settings'

// What the person chose is still there after a restart: the settings read at
// start keep every section — the starred tasks and the sort orders of the
// universes section included — over the defaults.
describe('the settings read at start', () => {
  const loaded = {
    appearance: { themeMode: 'light' },
    universes: { preferredDefault: 'dev', starred: { dev: ['p:tasks/b'] }, sort: { tasks: 'category', skills: 'source' } },
  } as never

  test('keep the starred tasks and the sort orders', () => {
    const s = mergeLoadedSettings(loaded)
    expect(s.universes.starred).toEqual({ dev: ['p:tasks/b'] })
    expect(s.universes.sort).toEqual({ tasks: 'category', skills: 'source' })
    expect(s.universes.preferredDefault).toBe('dev')
  })
  test('fill what the file does not say with the defaults', () => {
    const s = mergeLoadedSettings(loaded)
    expect(s.appearance.themeMode).toBe('light')
    expect(s.appearance.fontSize).toBe(DEFAULT_SETTINGS.appearance.fontSize)
    expect(mergeLoadedSettings(undefined)).toEqual(DEFAULT_SETTINGS)
  })
  test('are what the GUI hydrates its store with', () => {
    const src = readFileSync(new URL('../src/hooks/useSettingsPersistence.ts', import.meta.url), 'utf8')
    expect(src).toContain('mergeLoadedSettings(loaded)')
  })
})
