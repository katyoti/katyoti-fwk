// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { openableTarget, railUniverses } from '../src/sections/shell/workUniverses'

// The admin universe holds no work: it is not listed with the work universes
// and is never opened as one.
const registry = [
  { name: 'admin', protected: true },
  { name: 'dev' },
  { name: 'acme', protected: false },
]

describe('the universe rail', () => {
  test('lists the work universes, never the admin universe', () => {
    expect(railUniverses(registry).map((u) => u.name)).toEqual(['dev', 'acme'])
  })
  test('the rail draws its icons from that list', () => {
    const src = readFileSync(new URL('../src/sections/shell/IconRibbon.tsx', import.meta.url), 'utf8')
    expect(src).toContain('railUniverses(universes).map(')
    expect(src).not.toMatch(/\{universes\.map\(/)
  })
})

describe('the launch target', () => {
  test('a work universe opens', () => {
    expect(openableTarget(registry, ' dev ')).toBe('dev')
  })
  test('the admin universe lands on the management instead', () => {
    expect(openableTarget(registry, 'admin')).toBeNull()
  })
  test('an unknown or empty target opens nothing', () => {
    expect(openableTarget(registry, 'nope')).toBeNull()
    expect(openableTarget(registry, undefined)).toBeNull()
  })
})
