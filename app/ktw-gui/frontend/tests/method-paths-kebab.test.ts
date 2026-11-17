// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

// Every method the GUI calls is named by its URI, every segment kebab (the
// URI rule: `ktw-agents:cwm/object-list`, never `ktw-agents:cwm/ObjectList`):
// the name the method is declared, and its timeout read, under.

const SRC = join(import.meta.dir, '..', 'src')

function sources(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...sources(p))
    else if (/\.(ts|tsx)$/.test(name)) out.push(p)
  }
  return out
}

// A quoted method URI (`provider:service/method`, nested paths included)
// with an upper-case letter in its service or method path.
const PASCAL_METHOD = /['"`](ktw-[a-z0-9-]+:[a-z][A-Za-z0-9-]*(?:\/[A-Za-z0-9-]+)+)['"`]/g

function pascalMethodPaths(text: string): string[] {
  const found: string[] = []
  for (const m of text.matchAll(PASCAL_METHOD)) {
    if (/[A-Z]/.test(m[1])) found.push(m[1])
  }
  return found
}

describe('the method paths the GUI calls', () => {
  test('the guard sees a PascalCase method path', () => {
    expect(pascalMethodPaths(`call('ktw-agents:cwm/ObjectList', {})`)).toEqual(['ktw-agents:cwm/ObjectList'])
    expect(pascalMethodPaths(`call("ktw-agents:object-pane/Detail")`)).toEqual(['ktw-agents:object-pane/Detail'])
    expect(pascalMethodPaths(`call('ktw-agents:cwm/object-list', {})`)).toEqual([])
    expect(pascalMethodPaths(`'ktw-agents:/panel-type/progress'`)).toEqual([])
  })

  test('are kebab in every source file', () => {
    const offenders: string[] = []
    for (const file of sources(SRC)) {
      for (const path of pascalMethodPaths(readFileSync(file, 'utf8'))) {
        offenders.push(`${relative(SRC, file)}: ${path}`)
      }
    }
    expect(offenders).toEqual([])
  })
})
