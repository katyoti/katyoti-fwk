// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { ADMIN_UNIVERSE } from '../src/ktsp'
import { START_ORDER, passStart, settleStart, startKey, startTurn, useStartScreens, type StartScreen } from '../src/sections/shell/startScreens'

// On a fresh install the screens that open by themselves come
// one at a time, in one order decided by one owner: « Prêt à démarrer » first
// and alone, then the universe's first guide, then « Quoi de neuf ».
const set = (...s: StartScreen[]) => new Set<StartScreen>(s)

describe('the start screens', () => {
  test('come in one order: ready, the first guide, the shown-once guides', () => {
    expect([...START_ORDER]).toEqual(['ready', 'first-guide', 'shown-once'])
  })

  test('nothing opens before the start has settled on its universe', () => {
    for (const s of START_ORDER) expect(startTurn(false, 'u', set(), s)).toBe(false)
    expect(startTurn(false, ADMIN_UNIVERSE, set(), 'shown-once')).toBe(false)
  })

  test('in a universe, the ready screen opens alone and the others wait until it is closed', () => {
    expect(startTurn(true, 'u', set(), 'ready')).toBe(true)
    expect(startTurn(true, 'u', set(), 'first-guide')).toBe(false)
    expect(startTurn(true, 'u', set(), 'shown-once')).toBe(false)
    // Ready closed: the first guide, and only it.
    expect(startTurn(true, 'u', set('ready'), 'first-guide')).toBe(true)
    expect(startTurn(true, 'u', set('ready'), 'shown-once')).toBe(false)
    // The first guide closed: « Quoi de neuf ».
    expect(startTurn(true, 'u', set('ready', 'first-guide'), 'shown-once')).toBe(true)
    // A first guide passed before ready is closed opens nothing later.
    expect(startTurn(true, 'u', set('first-guide'), 'shown-once')).toBe(false)
  })

  test('at the picker only the shown-once guides apply', () => {
    expect(startKey(null)).toBe(ADMIN_UNIVERSE)
    expect(startKey('')).toBe(ADMIN_UNIVERSE)
    expect(startTurn(true, ADMIN_UNIVERSE, set(), 'shown-once')).toBe(true)
  })

  test('the store passes a universe through its screens', () => {
    useStartScreens.setState({ settled: false, passed: {} })
    const turn = (s: StartScreen) => {
      const st = useStartScreens.getState()
      return startTurn(st.settled, 'arch', new Set(st.passed.arch ?? []), s)
    }
    expect(turn('ready')).toBe(false)
    settleStart()
    expect(turn('ready')).toBe(true)
    expect(turn('first-guide')).toBe(false)
    passStart('arch', 'ready')
    passStart('arch', 'ready')
    expect(useStartScreens.getState().passed.arch).toEqual(['ready'])
    expect(turn('first-guide')).toBe(true)
    expect(turn('shown-once')).toBe(false)
    passStart('arch', 'first-guide')
    expect(turn('shown-once')).toBe(true)
    // Another universe starts from the beginning.
    expect(startTurn(true, 'other', new Set(useStartScreens.getState().passed.other ?? []), 'first-guide')).toBe(false)
  })

  test('each opener asks its turn and passes it; the boot settles the start', () => {
    const src = (f: string) => readFileSync(new URL(f, import.meta.url), 'utf8')
    const ready = src('../src/sections/ready/ReadyBadge.tsx')
    expect(ready).toContain("passStart(universe, 'ready')")
    expect(ready).toMatch(/!settled/)
    const first = src('../src/guide/FirstGuide.tsx')
    expect(first).toContain("useStartTurn(displayed, 'first-guide')")
    expect(first).toContain("passStart(r.universe, 'first-guide')")
    expect(first).toContain("passStart(displayed, 'first-guide')")
    const once = src('../src/guide/GuideAtStart.tsx')
    expect(once).toContain("useStartTurn(displayed, 'shown-once')")
    expect(once).toContain('!turn')
    expect(src('../src/hooks/useUniverses.ts')).toContain('.finally(settleStart)')
  })
})
