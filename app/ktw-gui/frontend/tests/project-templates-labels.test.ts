// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'bun:test'
import { readFileSync } from 'node:fs'

// A universe's starting content is called « Project templates » /
// « Modèles de projet » wherever a person reads it: no « seed »
// left in the Marketplace's and the universes' labels nor in the guide's text
// (the internal word stays in keys, ids and a YAML example), and a seed marked
// as demo data draws a « Démo » badge.
const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const values = (node: unknown): string[] =>
  typeof node === 'string' ? [node] : node && typeof node === 'object' ? Object.values(node).flatMap(values) : []

describe('project templates, never « seed », in what a person reads', () => {
  for (const lang of ['en', 'fr']) {
    for (const ns of ['marketplace', 'universes']) {
      it(`${lang}/${ns}.json has no « seed » in its labels`, () => {
        const seeded = values(JSON.parse(read(`../src/locales/${lang}/${ns}.json`))).filter((v) => /seed/i.test(v))
        expect(seeded).toEqual([])
      })
    }
  }
  it('names the tab and the demo badge', () => {
    expect(JSON.parse(read('../src/locales/en/marketplace.json')).tabs.seeds).toBe('Project templates')
    const fr = JSON.parse(read('../src/locales/fr/marketplace.json'))
    expect(fr.tabs.seeds).toBe('Modèles de projet')
    expect(fr.seeds.demo).toBe('Démo')
  })
  it('the guide speaks of project templates, its YAML example aside', () => {
    for (const ext of ['yaml', 'i18n.yaml']) {
      const text = read(`../../../ktw-hub/embed/guide/concept-universes-solutions-and-sidecar-versions.${ext}`)
      const lines = text.split('\n').filter((l) => /seed/i.test(l) && !/^\s*seed:\s*\{/.test(l))
      expect(lines).toEqual([])
    }
  })
  it('a demo seed draws the « Démo » badge', () => {
    expect(read('../src/sections/marketplace/SeedsPanel.tsx')).toContain(`{sd.demo && <Badge`)
  })
})
