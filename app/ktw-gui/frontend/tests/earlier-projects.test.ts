// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { importable, importPicked, lastUseDay, offerRows, type EarlierProject, type Importer } from '../src/sections/ready/earlierProjects'
import type { ImportProjectInput } from '../src/ktsp/marketplace'

// « Prêt à démarrer » offers the earlier installations' projects:
// each with its installation, folder and last use; none imported without the
// person's pick.

const project = (name: string, p: Partial<EarlierProject> = {}): EarlierProject => ({
  folder: `/old/${name}`, name, installation: 'KaTyoti 2026.09', universe: 'main', data_version: 1, ...p,
})
function hub(failing = ''): Importer & { calls: ImportProjectInput[] } {
  const calls: ImportProjectInput[] = []
  return {
    calls,
    importProject: async (input) => {
      calls.push(input)
      if (input.folder === failing) throw new Error('migrate the copy: step 3 broke - the copy is removed, the original is untouched')
      return { project: `/me/Katyoti/${input.folder.split('/').pop()}`, from: 1, to: 2 }
    },
  }
}

describe('the earlier projects offer', () => {
  test('nothing picked: nothing imported, no call', async () => {
    const h = hub()
    expect(await importPicked(h, offerRows({ projects: [project('acme'), project('beta')] }), new Set())).toEqual([])
    expect(h.calls).toEqual([])
  })

  test('only the picked ones are imported, each as a copy; a failure says why and the others go on', async () => {
    const h = hub('/old/beta')
    const rows = offerRows({ projects: [project('acme'), project('beta'), project('gamma')] })
    const out = await importPicked(h, rows, new Set(['/old/beta', '/old/gamma']))
    expect(h.calls.map((c) => c.folder).sort()).toEqual(['/old/beta', '/old/gamma'])
    expect(out.find((o) => o.folder === '/old/gamma')?.copy).toBe('/me/Katyoti/gamma')
    expect(out.find((o) => o.folder === '/old/beta')?.error).toContain('the original is untouched')
  })

  test('one imported already is not offered again', async () => {
    const done = project('acme', { imported_to: '/me/Katyoti/acme' })
    expect(importable(done)).toBe(false)
    const h = hub()
    expect(await importPicked(h, [done], new Set([done.folder]))).toEqual([])
    expect(h.calls).toEqual([])
  })

  test('each row shows its installation, folder and last use - the newest used first', () => {
    const rows = offerRows({ projects: [project('old', { last_used: '2026-01-02T10:00:00Z' }), project('new', { last_used: '2026-09-30T10:00:00Z' })] })
    expect(rows.map((r) => r.name)).toEqual(['new', 'old'])
    expect(lastUseDay(rows[0], 'en')).toContain('2026')
    expect(lastUseDay(project('x'), 'en')).toBe('')
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'EarlierProjectsSection.tsx'), 'utf8')
    for (const shown of ['p.installation', 'p.folder', 'lastUseDay(']) expect(screen).toContain(shown)
    expect(screen).toContain('section:ready/earlier-projects')
    expect(screen).toContain('action:ready/import-projects')
    // Nothing is picked for the person.
    expect(screen).toMatch(/useState<Set<string>>\(\(\) => new Set\(\)\)/)
  })
})
