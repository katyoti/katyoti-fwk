// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { canCreate, checkName, createNewProject, refusalKey, type ProjectCreator } from '../src/sections/univers/newProject'
import type { CreateProjectInput, CreateProjectOutput } from '../src/ktsp/orchestrator'

// A new project from its name: the folder it will make is shown, a
// name the hub refuses keeps Create off with its reason, and Create calls the
// hub once - nothing is created before.

const folder = '/home/me/Documents/Katyoti'
// A hub with "archi" registered: the rule is the hub's.
function hub(): ProjectCreator & { calls: CreateProjectInput[] } {
  const calls: CreateProjectInput[] = []
  return {
    calls,
    createProject: async (input: CreateProjectInput): Promise<CreateProjectOutput> => {
      calls.push(input)
      const out: CreateProjectOutput = { name: input.name, projectDir: `${folder}/${input.name}` }
      if (input.name === 'archi') {
        out.refused = `a project named "archi" is registered: ${folder}/archi, in universe "main" - choose another name`
        out.reason = 'registered'
      }
      if (!input.check && !out.refused) {
        out.created = true
        out.universe = input.universe
      }
      return out
    },
  }
}

describe('a new project from its name', () => {
  test('the folder it will make is <projects folder>/<name>, asked as a check that creates nothing', async () => {
    const h = hub()
    const c = await checkName(h, '  study-1 ', 'main')
    expect(c).toEqual({ name: 'study-1', folder: `${folder}/study-1`, refused: '', reason: '' })
    expect(h.calls).toEqual([{ name: 'study-1', universe: 'main', check: true }])
    expect(canCreate('study-1', c, false)).toBe(true)
  })

  test("an existing name keeps Create off and says why, in the form's language", async () => {
    const c = await checkName(hub(), 'archi', 'main')
    expect(c.refused).toContain('registered')
    expect(refusalKey(c)).toBe('projects.refused.registered')
    expect(canCreate('archi', c, false)).toBe(false)
  })

  test('a universe attaching a project takes a new one beside it: no refusal, Create on', async () => {
    const c = await checkName(hub(), 'study', 'busy')
    expect(c.refused).toBe('')
    expect(canCreate('study', c, false)).toBe(true)
  })

  test('needs-project-object is no refusal reason any more', async () => {
    const { REFUSAL_REASONS } = await import('../src/sections/univers/newProject')
    expect(REFUSAL_REASONS as readonly string[]).not.toContain('needs-project-object')
  })

  test('inside-project is no refusal reason any more: placeProject cannot answer it', async () => {
    const { REFUSAL_REASONS } = await import('../src/sections/univers/newProject')
    expect(REFUSAL_REASONS as readonly string[]).not.toContain('inside-project')
  })

  test('every refusal reason has its message in English and French', async () => {
    const { REFUSAL_REASONS } = await import('../src/sections/univers/newProject')
    for (const lang of ['en', 'fr']) {
      const t = (await import(`../src/locales/${lang}/universes.json`)).default as { projects: { refused: Record<string, string> } }
      for (const r of REFUSAL_REASONS) expect(t.projects.refused[r] ?? '').not.toBe('')
    }
  })

  test('no Create before the check of the name typed: empty, unchecked, stale or busy', async () => {
    const c = await checkName(hub(), 'study', 'main')
    expect(canCreate('', undefined, false)).toBe(false)
    expect(canCreate('study', undefined, false)).toBe(false)
    expect(canCreate('study-2', c, false)).toBe(false)
    expect(canCreate('study', c, true)).toBe(false)
  })

  test('Create calls the hub once, attached to the universe', async () => {
    const h = hub()
    const out = await createNewProject(h, 'main', ' study ')
    expect(h.calls).toEqual([{ name: 'study', universe: 'main' }])
    expect(out.created).toBe(true)
    expect(out.projectDir).toBe(`${folder}/study`)
  })
})
