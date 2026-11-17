// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readyLogLine, type Program, type ReadyInputs, type SidecarCheck } from '../src/sections/ready/readiness'
import type { KeySetsOutput, PlanPrereqsOutput } from '../src/ktsp/marketplace'

// The line the GUI's log gets at the end of each check of a universe: the
// clean-machine proof reads the last one and fails on
// verified=no, what the screen shows as « Non vérifié ».

const sidecar = (name: string, healthy: boolean): SidecarCheck => ({
  name,
  universeHome: '/u/main',
  report: {
    healthy, summary: healthy ? 'ok' : '1 problem',
    items: healthy ? [{ category: 'config', name: 'a.yaml', status: 'ok' }] : [{ category: 'env', name: 'A_KEY', status: 'missing', hint: 'set A_KEY' }],
    deps: [], env_keys: [], config_files: [],
  },
})
const program = (name: string, p: Partial<Program>): Program => ({ name, required: true, neededBy: ['s1'], selected: false, state: 'ok', ...p })
const plan = (...programs: Program[]): PlanPrereqsOutput => ({ platform: 'windows', programs })
const keys = (): KeySetsOutput => ({ passwordNeeded: false, remaining: [], universes: ['/u/main'] })
const inputs = (p: Partial<ReadyInputs>): ReadyInputs => ({ loading: false, errors: {}, ...p })

describe('the ready log line', () => {
  test('the 2026-10-14 13:55 run: two calls timed out - unknown, not verified, both errors said', () => {
    const line = readyLogLine('archifactory', inputs({
      keys: keys(),
      errors: { doctor: 'context deadline exceeded\n(timeout 10s)', plan: 'context deadline exceeded' },
    }))
    expect(line).toContain('universe=archifactory state=unknown verified=no')
    expect(line).toContain('errors=doctor: context deadline exceeded (timeout 10s); plan: context deadline exceeded')
    expect(line.includes('\n')).toBe(false)
  })

  test('every part read and green: ready, verified', () => {
    const line = readyLogLine('archifactory', inputs({ doctor: [sidecar('s1', true), sidecar('s2', true)], plan: plan(program('bun', {})), keys: keys() }))
    expect(line).toBe('universe=archifactory state=ready verified=yes sidecars=2/2 programs=0 keys=0')
  })

  test('every part read, something to do: not ready but verified', () => {
    const line = readyLogLine('archifactory', inputs({ doctor: [sidecar('s1', false)], plan: plan(), keys: keys() }))
    expect(line).toContain('state=not-ready verified=yes sidecars=0/1')
  })

  test('something wrong and a part unread: not verified', () => {
    const line = readyLogLine('archifactory', inputs({ doctor: [sidecar('s1', false)], keys: keys(), errors: { plan: 'hub down' } }))
    expect(line).toContain('state=not-ready verified=no')
  })

  test('a check that never finished is not verified', () => {
    expect(readyLogLine('u', inputs({ loading: true }))).toContain('verified=no')
    expect(readyLogLine('u', inputs({}))).toContain('state=checking verified=no')
  })
})

test('each check of a universe ends by logging its line to the GUI log', () => {
  const store = require('node:fs').readFileSync(new URL('../src/sections/ready/readyStore.ts', import.meta.url), 'utf8') as string
  const end = store.indexOf('put({ loading: false, errors })')
  expect(end).toBeGreaterThan(0)
  expect(store.indexOf("logLine('ready', readyLogLine(universe,", end)).toBeGreaterThan(end)
  expect(store).toContain('LogFromFrontend')
})
