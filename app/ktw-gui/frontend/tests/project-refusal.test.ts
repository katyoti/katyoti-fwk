// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { PROJECT_REFUSED, projectRefusal, retryUnlessRefused } from '../src/sections/workspace/projectRefusal'
import { runPart } from '../src/hooks/useHostLoad'

// A universe whose project is refused says why in its
// project area, at once, instead of spinning.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

const reason = "this project is a Dossier d'architecture, which needs the solution archifactory; this universe has none - give it the solution with `katyoti universe set-solution acme archifactory`, or open the project in a universe of archifactory"

describe('a refusal of the project', () => {
  test('is read from the failure, wrapped or not, as its reason', () => {
    expect(projectRefusal(PROJECT_REFUSED + reason)).toBe(reason)
    expect(projectRefusal(`ktw-agents:cwm/ObjectTree: invalid input: ${PROJECT_REFUSED}${reason}`)).toBe(reason)
    expect(projectRefusal(new Error(PROJECT_REFUSED + reason))).toBe(reason)
  })
  test('is not any other failure', () => {
    expect(projectRefusal('readiness deadline elapsed (last err: no responders)')).toBeNull()
    expect(projectRefusal(null)).toBeNull()
    expect(projectRefusal(PROJECT_REFUSED)).toBeNull()
  })
})

describe('the tree load', () => {
  test('asks a starting core again, never a refused project', async () => {
    expect(retryUnlessRefused(new Error('no responders'))).toBe(true)
    expect(retryUnlessRefused(new Error(PROJECT_REFUSED + reason))).toBe(false)

    let tries = 0
    const refused = runPart({ universe: 'u-refusal-test', key: 'refused', retryErrors: retryUnlessRefused,
      load: async () => { tries++; throw new Error(PROJECT_REFUSED + reason) } }, new AbortController().signal, [1, 1, 1])
    await expect(refused).rejects.toThrow(reason)
    expect(tries).toBe(1)

    tries = 0
    const starting = runPart({ universe: 'u-refusal-test', key: 'starting', retryErrors: retryUnlessRefused,
      load: async () => { tries++; if (tries < 3) throw new Error('no responders'); return 'ok' } }, new AbortController().signal, [1, 1, 1])
    expect(await starting).toBe('ok')
    expect(tries).toBe(3)
  })
  test('draws the reason and the way to the universe page', () => {
    const tree = src('sections/workspace/WorkspaceObjectsTree.tsx')
    expect(tree).toContain('retryErrors: retryUnlessRefused')
    expect(tree).toContain('const refused = projectRefusal(error)')
    expect(tree).toContain('data-guide="tree.project-refused-reason"')
    expect(tree).toContain("setMainPaneRoute({ kind: 'edit', name: universe })")
  })
})
