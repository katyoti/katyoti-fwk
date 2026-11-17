// SPDX-License-Identifier: Apache-2.0
// A button inside a clickable cell runs only its own action -
// a « waiting on » chip in an increment's Open requests row opens the request
// it names, never the row's own request after it. ButtonRenderer presses
// through pressOnly, which stops the click where the button is.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { pressOnly } from '../src/ui-builders/page/ui-page.interactive.ts'

// click dispatches a click from the innermost handler out, as the DOM
// bubbles it, until one stops it.
function click(path: Array<(e: { stopPropagation(): void }) => void>): void {
  let stopped = false
  const e = { stopPropagation: () => { stopped = true } }
  for (const h of path) {
    h(e)
    if (stopped) return
  }
}

describe('a button inside a clickable cell', () => {
  test('fires only its own action', () => {
    const ran: string[] = []
    const cell = () => { ran.push('the row: its own request') }
    const chip = (e: { stopPropagation(): void }) => pressOnly(e, () => { ran.push('the chip: the request it waits on') })
    click([chip, cell])
    expect(ran).toEqual(['the chip: the request it waits on'])
  })

  test('the cell alone still runs its action', () => {
    const ran: string[] = []
    click([() => { ran.push('the row') }])
    expect(ran).toEqual(['the row'])
  })

  test('ButtonRenderer presses through pressOnly, the file chooser too', () => {
    const src = readFileSync(join(import.meta.dir, '../src/ui-builders/page/nodes/interactive.tsx'), 'utf8')
    const at = src.indexOf('export function ButtonRenderer(')
    expect(at).toBeGreaterThan(-1)
    const body = src.slice(at, src.indexOf('\n}\n', at))
    const presses = body.match(/onClick=\{[^\n]*/g) ?? []
    expect(presses.length).toBe(1)
    expect(presses[0]).toMatch(/onClick=\{\(e\) => pressOnly\(e, \(\) => file \? input\.current\?\.click\(\) : run\?\.\(node\.action, item\)\)\}/)
  })
})
