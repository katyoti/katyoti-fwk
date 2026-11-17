// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { welcomeOff } from '../src/guide/welcome'

// The welcome screens stay closed when the start says so or the
// universe on screen turned them off; both openers - the shown-once guides
// (« Quoi de neuf », a Welcome) and a universe's first guide - wait for that
// answer and open nothing while it says off.
describe('the welcome screens', () => {
  test('stay closed by the start option or the universe setting, open otherwise', () => {
    expect(welcomeOff(false)).toBe(false)
    expect(welcomeOff(false, { noWelcome: false })).toBe(false)
    expect(welcomeOff(true)).toBe(true)
    expect(welcomeOff(false, { noWelcome: true })).toBe(true)
  })

  test('both openers are gated on it', () => {
    for (const f of ['../src/guide/GuideAtStart.tsx', '../src/guide/FirstGuide.tsx']) {
      const src = readFileSync(new URL(f, import.meta.url), 'utf8')
      expect(src).toContain('useWelcomeOff(displayed)')
      expect(src).toMatch(/off !== false/)
    }
  })
})
