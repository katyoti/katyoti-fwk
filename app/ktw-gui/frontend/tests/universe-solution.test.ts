// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { installedSolutionChoices, offersSolutionChoice } from '../src/pages/universeSolution'

// The universe editor offers a universe with no solution an
// installed one, over the orchestrator's set-universe-solution.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

describe('the generated orchestrator client', () => {
  test('carries setUniverseSolution dispatching set-universe-solution', () => {
    const client = src('ktsp/orchestrator.ts')
    expect(client).toContain('setUniverseSolution: (input: SetUniverseSolutionInput)')
    expect(client).toContain('dispatch("ktw-univers:orchestrator/set-universe-solution", input)')
  })
})

describe('the solution action', () => {
  test('is offered only to a universe with no solution', () => {
    expect(offersSolutionChoice({})).toBe(true)
    expect(offersSolutionChoice({ solution: '' })).toBe(true)
    expect(offersSolutionChoice({ solution: '  ' })).toBe(true)
    expect(offersSolutionChoice({ solution: 'archi-factory@2026.10' })).toBe(false)
    expect(offersSolutionChoice(null)).toBe(false)
  })
  test('suggests the installed solutions the universes show, each once, sorted', () => {
    expect(installedSolutionChoices([
      { solution: 'software-factory@1.2' },
      { solution: 'archi-factory@2026.10' },
      {},
      { solution: 'archi-factory@2026.10' },
      { solution: 'no-version' },
    ])).toEqual(['archi-factory@2026.10', 'software-factory@1.2'])
  })
  test('the pane calls the service and draws the action only when offered', () => {
    const pane = src('pages/UniverseEditPane.tsx')
    expect(pane).toContain('orchestratorClient().setUniverseSolution({ name, solution: want })')
    expect(pane).toContain('offersSolutionChoice(info.summary) ?')
    expect(pane).toContain('data-guide="universe-edit.give-solution"')
  })
})
