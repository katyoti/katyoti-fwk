// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { badgeOf, badgeRightEdge, badgeWidth, RAIL_BADGE, RAIL_SCREEN, RAIL_WIDTH, railScreenOf, shownOnStrip } from '../src/sections/shell/RailScreen'
import { ELEMENTS } from '../src/elements'

// The rail's Inbox opens what the backend names: a page, its
// badge's count and the Global strip's panels it stands in for - « À faire ».
// The GUI names no pack's screen, reads no switch and has no
// built-in Inbox.
describe("the rail's Inbox, named by the backend", () => {
  test('it reads what the backend names, and nothing from anything else', () => {
    expect(RAIL_SCREEN).toBe('ktw-agents:waiting/inbox')
    expect(railScreenOf('{"page":"ktw-agents:views/person-queue-page","count":"ktw-agents:views/person-queue-count","replaces":["person-queue"]}'))
      .toEqual({ page: 'ktw-agents:views/person-queue-page', count: 'ktw-agents:views/person-queue-count', replaces: ['person-queue'] })
    expect(railScreenOf('{}')).toEqual({})
    expect(railScreenOf('')).toEqual({})
    expect(railScreenOf('not json')).toEqual({})
    expect(railScreenOf('{"page":3,"replaces":"x"}')).toEqual({})
  })
  test('the badge is the count, none at 0, 99+ past 99', () => {
    expect(badgeOf(0)).toBe('')
    expect(badgeOf(-1)).toBe('')
    expect(badgeOf(Number.NaN)).toBe('')
    expect(badgeOf(7)).toBe('7')
    expect(badgeOf(120)).toBe('99+')
  })
  test('the count bubble sits whole inside the rail at 1, 2 and 3 digits', () => {
    for (const n of [1, 12, 99, 123]) {
      const label = badgeOf(n)
      expect(badgeRightEdge(label)).toBeLessThanOrEqual(RAIL_WIDTH - 1)
      expect(badgeWidth(label)).toBeGreaterThanOrEqual(RAIL_BADGE.size)
    }
    // The bubble wraps the button: inside it, the button's overflow cuts it.
    const rail = readFileSync(join(import.meta.dir, '../src/sections/shell/IconRibbon.tsx'), 'utf8')
    const bubble = rail.indexOf('data-element="rail:todo-count"')
    const button = rail.indexOf('data-element="rail:inbox"')
    expect(bubble).toBeGreaterThan(-1)
    expect(bubble).toBeLessThan(button)
    const end = rail.indexOf('</ActionIcon>', button)
    expect(rail.slice(button, end)).not.toContain('<Indicator')
    expect(rail).toContain('offset={RAIL_BADGE.offset}')
  })
  test('a panel the screen stands in for leaves the Global strip; nothing named, all stay', () => {
    const named = { page: 'p', replaces: ['person-queue'] }
    expect(shownOnStrip('home:person-queue', named)).toBe(false)
    expect(shownOnStrip('panel:person-queue', named)).toBe(false)
    expect(shownOnStrip('home:planning-deck', named)).toBe(true)
    expect(shownOnStrip('home:person-queue', {})).toBe(true)
  })
  test('its screen and its badge carry element ids; the GUI reads no switch for it', () => {
    const ids = ELEMENTS.map((e) => e.id)
    expect(ids).toContain('screen:todo')
    expect(ids).toContain('rail:todo-count')
    const src = readFileSync(join(import.meta.dir, '../src/sections/shell/RailScreen.tsx'), 'utf8')
    expect(src).toContain('data-element="screen:todo"')
    const rail = readFileSync(join(import.meta.dir, '../src/sections/shell/IconRibbon.tsx'), 'utf8')
    expect(rail).toContain('data-element="rail:todo-count"')
    expect(rail).toContain('data-element="rail:inbox"')
    for (const f of ['../src/sections/shell/RailScreen.tsx', '../src/sections/shell/IconRibbon.tsx', '../src/sections/home/HomeTab.tsx']) {
      expect(readFileSync(join(import.meta.dir, f), 'utf8')).not.toContain("useDisplaySwitch('in039')")
    }
    // No built-in Inbox: « À faire » is the one screen.
    expect(src).not.toContain('InboxPanel')
    expect(existsSync(join(import.meta.dir, '../src/sections/inbox/InboxPanel.tsx'))).toBe(false)
  })
})
