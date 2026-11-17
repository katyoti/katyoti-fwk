// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { switchOn } from '../src/hooks/useDisplaySwitch'

// An increment's display switch is off unless this start said it is on:
// the former display shows until the delivery is approved.
describe('display switch', () => {
  test('off while unread, unknown or not true', () => {
    expect(switchOn(undefined, 'in045')).toBe(false)
    expect(switchOn({}, 'in045')).toBe(false)
    expect(switchOn({ in045: false }, 'in045')).toBe(false)
    expect(switchOn({ in046: true }, 'in045')).toBe(false)
  })
  test('on when the backend says so', () => {
    expect(switchOn({ in045: true }, 'in045')).toBe(true)
  })
})

// An approved increment's switch is retired: no GUI source asks it - its screens show for everyone.
describe('retired display switches', () => {
  test('no source asks in030, in038, in039, in047, in062, in063 or in064', () => {
    const { readdirSync, readFileSync, statSync } = require('node:fs') as typeof import('node:fs')
    const { join } = require('node:path') as typeof import('node:path')
    const root = new URL('../src', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')
    const files: string[] = []
    const walk = (d: string) => {
      for (const n of readdirSync(d)) {
        const p = join(d, n)
        if (statSync(p).isDirectory()) walk(p)
        else if (/\.tsx?$/.test(n)) files.push(p)
      }
    }
    walk(root)
    expect(files.length).toBeGreaterThan(50)
    for (const f of files) {
      expect(readFileSync(f, 'utf8')).not.toMatch(/useDisplaySwitch\(\s*['"`]in0(30|38|39|47|62|63|64)['"`]\s*\)|KTW_IN0(30|38|39|47|62|63|64)_DISPLAY/)
    }
  })
  // The object-levels switch is retired: its display is the only one.
  test("no source asks IN-036's switch, and objectLevels.ts is gone", () => {
    const { existsSync, readdirSync, readFileSync, statSync } = require('node:fs') as typeof import('node:fs')
    const { join } = require('node:path') as typeof import('node:path')
    const root = new URL('../src', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')
    expect(existsSync(join(root, 'sections', 'workspace', 'objectLevels.ts'))).toBe(false)
    const files: string[] = []
    const walk = (d: string) => {
      for (const n of readdirSync(d)) {
        const p = join(d, n)
        if (statSync(p).isDirectory()) walk(p)
        else if (/\.tsx?$/.test(n)) files.push(p)
      }
    }
    walk(root)
    expect(files.length).toBeGreaterThan(50)
    for (const f of files) {
      expect(readFileSync(f, 'utf8')).not.toMatch(/objectLevelsDisplay|OBJECT_LEVELS_DISPLAY|setObjectLevelsDisplay|devSwitchOn\('in-036'\)/)
    }
  })
})
