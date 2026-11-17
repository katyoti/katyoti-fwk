// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { DEFAULT_SETTINGS, DISPLAY_LEVELS } from '../src/types/settings'
import { isLeftOut } from '../src/sections/shell/universeGui'
import { useAppStore } from '../src/store'

// The display level is the person's, once for all universes: a
// new person starts at Essentiel; switching it changes what every universe
// shows at once; no per-universe « Mode expert » toggle remains.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

describe('the display level', () => {
  test('a new person starts at Essentiel, of three levels', () => {
    expect(DEFAULT_SETTINGS.general.level).toBe('essential')
    expect(DISPLAY_LEVELS).toEqual(['essential', 'standard', 'expert'])
  })
  test('switching it changes what every universe shows, without a reload', () => {
    useAppStore.getState().updateSettings('general', { level: 'essential' })
    const at = () => useAppStore.getState().appSettings.general.level
    expect(isLeftOut('rail:debug', at(), null)).toBe(true)
    expect(isLeftOut('rail:debug', at(), { all: true, allowed: [] })).toBe(true) // another universe too
    useAppStore.getState().updateSettings('general', { level: 'expert' })
    expect(isLeftOut('rail:debug', at(), null)).toBe(false)
    expect(isLeftOut('rail:debug', at(), { all: true, allowed: [] })).toBe(false)
  })
  test('no per-universe expert toggle remains', () => {
    expect(src('sections/shell/IconRibbon.tsx')).not.toContain('universe-menu.expert')
    expect(src('sections/shell/IconRibbon.tsx')).toContain('profile-menu.level')
    expect(src('types/settings.ts')).not.toMatch(/expert\?:\s*string\[\]/)
  })
})
