// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { concepts, CONCEPTS_TOPIC, type RecipeEntry } from '../src/guide/recipes'

// A sidecar marks some of its guides as concepts - the documents
// explaining KaTyoti; the guide lists them first under « Concepts KaTyoti »,
// and the help menu opens the guide on them in one click.
const entry = (provider: string, id: string, name: string, concept?: boolean): RecipeEntry =>
  ({ uri: `${provider}:/guide-recipe/${id}`, provider, id, recipe: { name, concept, body: '# x' } }) as unknown as RecipeEntry

const all = [
  entry('ktw-hub', 'universes', 'Universes, solutions and sidecar versions', true),
  entry('ktw-hub', 'what-s-new', "What's new"),
  entry('ktw-hub', 'projects', 'Projects, repositories and universes', true),
]

describe('the concept documents', () => {
  test('are the guides marked concept, by name', () => {
    expect(concepts(all).map((e) => e.id)).toEqual(['projects', 'universes'])
  })
  test('none when no sidecar marks one', () => {
    expect(concepts([all[1]])).toEqual([])
  })
  const browser = readFileSync(new URL('../src/guide/GuideBrowser.tsx', import.meta.url), 'utf8')
  test('come first in the guide, before the essentials, and not again among the readings', () => {
    expect(browser).toContain('const conceptList = concepts(filtered)')
    expect(browser.indexOf("t('browser.concepts')")).toBeLessThan(browser.indexOf("t('browser.essentials')"))
    expect(browser).toContain('!e.recipe.concept && (nature(e.recipe)')
  })
  test('the concepts topic keeps them only', () => {
    expect(browser).toContain('if (topics.includes(CONCEPTS_TOPIC)) return !!e.recipe.concept')
  })
  test('the help menu opens the guide on them in one click', () => {
    const ribbon = readFileSync(new URL('../src/sections/shell/IconRibbon.tsx', import.meta.url), 'utf8')
    expect(ribbon).toContain('onClick={() => launchGuide([CONCEPTS_TOPIC])} data-guide="profile.concepts"')
    expect(CONCEPTS_TOPIC).toBe('concepts')
  })
  test('the section and the entry are named in English and French', () => {
    for (const [lang, name] of [['en', 'KaTyoti concepts'], ['fr', 'Concepts KaTyoti']]) {
      const guide = JSON.parse(readFileSync(new URL(`../src/locales/${lang}/guide.json`, import.meta.url), 'utf8'))
      const shell = JSON.parse(readFileSync(new URL(`../src/locales/${lang}/shell.json`, import.meta.url), 'utf8'))
      expect(guide.browser.concepts).toBe(name)
      expect(shell.profile.concepts).toBe(name)
    }
  })
})
