// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync, statSync } from 'fs'
import { join } from 'path'
import { noteRevision, revisionOf } from '../src/sections/workspace/objectRevision'

// A save never overwrites a change made elsewhere since the pane loaded: the
// version the GUI read travels with every save (cwm.objectConflict, « reload »).
describe('the version a save sends', () => {
  test('is the one last read or written', () => {
    expect(revisionOf('m.a')).toBe('')
    noteRevision('m.a', 'r1')
    expect(revisionOf('m.a')).toBe('r1')
    noteRevision('m.a', 'r2') // a save wrote r2
    expect(revisionOf('m.a')).toBe('r2')
    noteRevision('m.a', undefined) // a refused save writes nothing
    expect(revisionOf('m.a')).toBe('r2')
  })
})

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n)
    return statSync(p).isDirectory() ? sources(p) : /\.tsx?$/.test(n) ? [p] : []
  })
}

describe('every save in the GUI', () => {
  test('sends the version its pane read', () => {
    const calls: string[] = []
    for (const f of sources(new URL('../src', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))) {
      const src = readFileSync(f, 'utf8')
      for (const m of src.matchAll(/WorkspaceObjectUpdate(?:WithBody)?\(([^\n]*)\)/g)) calls.push(`${f}: ${m[1]}`)
    }
    expect(calls.length).toBeGreaterThan(0)
    for (const c of calls) expect(c).toContain('revisionOf(')
  })
})
