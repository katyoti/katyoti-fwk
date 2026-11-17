// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { firstGuideDue, firstGuideKey } from '../src/guide/recipes'

// The first guide a universe names plays at its first open, once
// per universe, and ends on the guide tab - the essentials at Essentiel.
const archi = { name: 'archi', firstGuide: 'ktw-acme:/guide-recipe/first-study' }

describe('a universe\'s first guide', () => {
  test('is due at the universe\'s first open', () => {
    expect(firstGuideDue(archi, new Set())).toBe('ktw-acme:/guide-recipe/first-study')
  })
  test('is seen once per universe', () => {
    const seen = new Set([firstGuideKey(archi.firstGuide, 'archi')])
    expect(firstGuideDue(archi, seen)).toBe('')
    expect(firstGuideDue({ ...archi, name: 'other' }, seen)).toBe('ktw-acme:/guide-recipe/first-study')
  })
  test('nothing is due for a universe that names none', () => {
    expect(firstGuideDue({ name: 'dev' }, new Set())).toBe('')
    expect(firstGuideDue(undefined, new Set())).toBe('')
  })
  test('the GUI plays it once nothing else plays, then opens the guide tab and records it', () => {
    const src = readFileSync(new URL('../src/guide/FirstGuide.tsx', import.meta.url), 'utf8')
    expect(src).toContain('if (!loaded || off !== false || busy || running.current')
    expect(src).toContain("complete(firstGuideKey(r.uri, r.universe), 'seen')")
    expect(src).toContain('launchGuide()')
    expect(readFileSync(new URL('../src/layout/BaseLayout.tsx', import.meta.url), 'utf8')).toContain('<FirstGuide />')
  })
})
