// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { essentials, type RecipeEntry } from '../src/guide/recipes'

// A sidecar names some of its guides essential - what a starter
// needs, not a level; at the Essentiel display level the guide lists the
// essentials of the universe's wired sidecars first. Only wired sidecars'
// guides are loaded, so unwiring one takes its essentials away.
const entry = (provider: string, id: string, name: string, essential?: boolean): RecipeEntry =>
  ({ uri: `${provider}:/guide-recipe/${id}`, provider, id, recipe: { name, essential, steps: [{ id: 's', say: 'x' }] } }) as unknown as RecipeEntry

const hub = [entry('ktw-hub', 'welcome', 'Welcome', true), entry('ktw-hub', 'record', 'Record your screen')]
const acme = [entry('ktw-acme', 'new-study', 'Start an as-is study', true), entry('ktw-acme', 'flows', 'Draw the flows')]

describe('the essentials', () => {
  test('come first at Essentiel: each wired sidecar\'s, by name', () => {
    expect(essentials([...hub, ...acme], 'essential').map((e) => e.id)).toEqual(['new-study', 'welcome'])
  })
  test('are not set apart at Standard or Expert', () => {
    expect(essentials([...hub, ...acme], 'standard')).toEqual([])
    expect(essentials([...hub, ...acme], 'expert')).toEqual([])
  })
  test('go with their sidecar when it is unwired', () => {
    expect(essentials(hub, 'essential').map((e) => e.id)).toEqual(['welcome'])
  })
  test('the guide browser shows them first, at the person\'s level', () => {
    const browser = readFileSync(new URL('../src/guide/GuideBrowser.tsx', import.meta.url), 'utf8')
    expect(browser).toContain('const essentialList = essentials(filtered, level)')
    expect(browser.indexOf("t('browser.essentials')")).toBeLessThan(browser.indexOf("t('browser.suggested')"))
  })
})
