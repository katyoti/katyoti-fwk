// SPDX-License-Identifier: Apache-2.0
// The operator verbs a caller runs on every member answer and exit: ktw-display
// has no configuration, and a verb it would not answer boots the never-exiting
// server — a scenario case's install (`sc config set`) waited on it forever.
import { describe, expect, test } from 'bun:test'
import { join } from 'node:path'

const INDEX = join(import.meta.dir, '..', 'src', 'index.ts')

function run(...args: string[]) {
  const p = Bun.spawnSync(['bun', INDEX, ...args], { timeout: 20_000, env: { ...process.env, KTW_DISPLAY_PORT: '1' } })
  return { code: p.exitCode, out: p.stdout.toString(), err: p.stderr.toString() }
}

describe('ktw-display operator verbs', () => {
  test('sc config set writes nothing and exits', () => {
    const r = run('sc', 'config', 'set', '--out', 'raw')
    expect(r.code).toBe(0)
  })
  test('sidecar config check reports healthy', () => {
    const r = run('sidecar', 'config', 'check', '--out', 'json')
    expect(r.code).toBe(0)
    expect(JSON.parse(r.out).healthy).toBe(true)
  })
  test('config get has nothing to read', () => {
    const r = run('sidecar', 'config', 'get', 'Display.yaml', '--out', 'json')
    expect(r.code).toBe(1)
    expect(r.err).toContain('no config file Display.yaml')
  })
  test('another sc verb is refused, not served', () => {
    expect(run('sc', 'keys', 'list').code).toBe(2)
  })
})
