// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync } from 'fs'
import { join } from 'path'
import { offeredIn, type RecipeEntry } from '../src/guide/recipes'

// The Guides tab of a tagged universe offers that universe's own
// guides - those carrying one of its priority tags - and KaTyoti's usage ones
// (katyoti-usage), nothing else - KaTyoti's concept documents among the usage
// ones. Read from the shell's real recipes; a solution's sidecar guides and
// its tag are written here (a neutral « acme-studio » solution and a « crm »
// one).

const APP = join(import.meta.dir, '..', '..', '..')

function recipes(provider: string): RecipeEntry[] {
  const dir = join(APP, provider, 'embed', 'guide')
  return readdirSync(dir)
    .filter((f) => f.endsWith('.yaml') && !f.endsWith('.i18n.yaml'))
    .map((f) => {
      const id = f.replace(/\.yaml$/, '')
      const recipe = Bun.YAML.parse(readFileSync(join(dir, f), 'utf8')) as RecipeEntry['recipe']
      return { uri: `${provider}:/guide-recipe/${id}`, provider, id, universe: 'u', recipe } as RecipeEntry
    })
}

function sidecar(provider: string, guides: Record<string, string[]>): RecipeEntry[] {
  return Object.entries(guides).map(([id, hashtags]) =>
    ({ uri: `${provider}:/guide-recipe/${id}`, provider, id, universe: 'u', recipe: { name: id, hashtags, body: `# ${id}` } }) as unknown as RecipeEntry)
}

const all = [
  ...recipes('ktw-hub'),
  ...sidecar('ktw-acme', {
    'collect-study-sources': ['acme-studio', 'study', 'sources'],
    'create-a-study': ['acme-studio', 'study', 'create'],
    'frame-a-study': ['acme-studio', 'study', 'scope'],
  }),
  ...sidecar('ktw-crm', { 'crm-showcase': ['crm', 'showcase'], 'crm-import': ['crm'] }),
]
const acmeStudio = { first: ['acme-studio'] }
const ids = (es: RecipeEntry[]) => es.map((e) => e.uri).sort()

describe('the Guides tab follows the universe tag', () => {
  test('a solution offers its own guides, the usage ones and the concept documents, no administration guide', () => {
    const offered = ids(offeredIn(all, { all: false, first: acmeStudio.first }))
    expect(offered).toEqual([
      'ktw-acme:/guide-recipe/collect-study-sources',
      'ktw-acme:/guide-recipe/create-a-study',
      'ktw-acme:/guide-recipe/frame-a-study',
      'ktw-hub:/guide-recipe/answer-the-inbox',
      'ktw-hub:/guide-recipe/concept-project-and-module-types',
      'ktw-hub:/guide-recipe/concept-projects-repositories-and-universes',
      'ktw-hub:/guide-recipe/concept-universes-solutions-and-sidecar-versions',
      'ktw-hub:/guide-recipe/record-your-screen',
      'ktw-hub:/guide-recipe/use-katyoti',
      'ktw-hub:/guide-recipe/whats-new',
    ])
  })

  test('a universe without tags, or whose view is not read yet, offers every guide', () => {
    expect(offeredIn(all, { all: true, first: [] })).toHaveLength(all.length)
    expect(offeredIn(all, null)).toHaveLength(all.length)
    expect(offeredIn(all, undefined)).toHaveLength(all.length)
  })

  test("another tagged universe gets its own guides and KaTyoti's usage ones, never another solution's", () => {
    const offered = ids(offeredIn(all, { all: false, first: ['crm'] }))
    expect(offered.some((u) => u.startsWith('ktw-acme:'))).toBe(false)
    expect(offered).toContain('ktw-crm:/guide-recipe/crm-showcase')
    expect(offered).toContain('ktw-hub:/guide-recipe/use-katyoti')
  })

  // Every universe built on KaTyoti - a solution's among them -
  // offers KaTyoti's concept documents, with no work in the solution.
  const concepts = all.filter((e) => e.provider === 'ktw-hub' && e.recipe.concept).map((e) => e.uri).sort()
  test("every universe offers KaTyoti's concept documents, whatever its tags", () => {
    expect(concepts).toEqual([
      'ktw-hub:/guide-recipe/concept-project-and-module-types',
      'ktw-hub:/guide-recipe/concept-projects-repositories-and-universes',
      'ktw-hub:/guide-recipe/concept-universes-solutions-and-sidecar-versions',
    ])
    for (const view of [{ all: false, first: acmeStudio.first }, { all: false, first: ['crm'] }, { all: false, first: ['some-solution'] }, { all: true }, null]) {
      const offered = ids(offeredIn(all, view))
      for (const c of concepts) expect(offered).toContain(c)
    }
  })
  test("even one the shell ships without the usage tag; a sidecar's own concept follows its tags", () => {
    const hub = { uri: 'ktw-hub:/guide-recipe/c', provider: 'ktw-hub', id: 'c', recipe: { name: 'C', concept: true, body: '# c' } } as unknown as RecipeEntry
    const side = { uri: 'ktw-crm:/guide-recipe/c', provider: 'ktw-crm', id: 'c', recipe: { name: 'C', concept: true, body: '# c', hashtags: ['crm'] } } as unknown as RecipeEntry
    expect(ids(offeredIn([hub, side], { all: false, first: ['acme-studio'] }))).toEqual(['ktw-hub:/guide-recipe/c'])
  })
})
