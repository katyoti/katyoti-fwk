// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'

// The left panel's tree tab is named with the universe settings' own word for
// what it shows, the projects; "objects" means nothing to the person.
const json = (lang: string, ns: string) =>
  JSON.parse(readFileSync(new URL(`../src/locales/${lang}/${ns}.json`, import.meta.url), 'utf8'))

describe('the tree tab', () => {
  for (const lang of ['en', 'fr']) {
    test(`${lang}: reads the settings' word for projects`, () => {
      const settings: string = json(lang, 'universes').universe?.tabs?.projects ?? findProjects(json(lang, 'universes'))
      expect(json(lang, 'shell').project.objects.toLowerCase()).toBe(settings.toLowerCase())
    })
    test(`${lang}: no tour text names an object tree`, () => {
      const text = JSON.stringify(json(lang, 'guide')).toLowerCase()
      expect(text).not.toContain('object tree')
      expect(text).not.toContain('arbre des objets')
    })
  }
})

// The settings' label for their Projects tab, wherever it sits.
function findProjects(o: unknown): string {
  if (o && typeof o === 'object') {
    for (const [k, v] of Object.entries(o as Record<string, unknown>)) {
      if (k === 'projects' && typeof v === 'string') return v
      const found = findProjects(v)
      if (found) return found
    }
  }
  return ''
}
