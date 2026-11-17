// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { offersRestart, saysHubKeepsRunning } from '../src/sections/shell/universeRuntime'
import { panelMenuItems } from '../src/sections/shell/panelMenu'
import { ELEMENTS, RESTART_UNIVERSE_ELEMENT } from '../src/elements'

// « Redémarrer l'univers » on a running universe, and the notice
// that its hub keeps running after the GUI closes - shown for everyone since
// the person approved its increment's delivery (its switch retired).
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

describe('« Redémarrer l\'univers »', () => {
  test('is offered on a running universe', () => {
    expect(offersRestart({ busRunning: true })).toBe(true)
  })
  test('is not offered on a universe that does not run', () => {
    expect(offersRestart({ busRunning: false })).toBe(false)
    expect(offersRestart(undefined)).toBe(false)
  })
  test('the left panel\'s menu holds it only when offered, after « Vérifier l\'installation »', () => {
    expect(panelMenuItems([], true).map((i) => i.key)).toEqual(['checkInstall', 'restart', 'openFolder', 'remove'])
    expect(panelMenuItems([]).map((i) => i.key)).not.toContain('restart')
  })
  test('is one element, drawn by both universe menus and calling the restart binding', () => {
    expect(ELEMENTS.map((e) => e.id)).toContain(RESTART_UNIVERSE_ELEMENT)
    expect(src('sections/shell/IconRibbon.tsx')).toContain('data-element={RESTART_UNIVERSE_ELEMENT}')
    expect(src('sections/shell/IconRibbon.tsx')).toContain('offersRestart(universe)')
    expect(src('sections/shell/ProjectPanel.tsx')).toContain('offersRestart(displayedUniverse)')
    // The store calls the Go binding that ends the hub, never only a deactivate/activate.
    const store = src('store.ts')
    expect(store).toContain('UniverseRestart')
  })
})

describe('the hub-keeps-running notice', () => {
  test('shows when the GUI found the hub running', () => {
    expect(saysHubKeepsRunning({ busRunning: true, hubKeepsRunning: true })).toBe(true)
  })
  test('is silent for a hub the GUI started, or on a stopped universe', () => {
    expect(saysHubKeepsRunning({ busRunning: true, hubKeepsRunning: false })).toBe(false)
    expect(saysHubKeepsRunning({ busRunning: true })).toBe(false)
    expect(saysHubKeepsRunning({ busRunning: false, hubKeepsRunning: true })).toBe(false)
  })
  test('the rail says it, in both languages', () => {
    expect(src('sections/shell/IconRibbon.tsx')).toContain('saysHubKeepsRunning(universe)')
    for (const lang of ['fr', 'en']) {
      const shell = JSON.parse(src(`locales/${lang}/shell.json`))
      expect(shell.universe.hubKeepsRunning).toBeTruthy()
      expect(shell.universe.restart).toBeTruthy()
      expect(shell.project.restart).toBeTruthy()
    }
  })
})

// The person approved their increments' deliveries: no screen reads their
// switches any more.
describe('the approved switches are retired', () => {
  test('no screen reads in063 or in064', () => {
    for (const rel of ['sections/shell/IconRibbon.tsx', 'sections/shell/ProjectPanel.tsx', 'sections/marketplace/InfraPanel.tsx', 'sections/marketplace/SidecarsPanel.tsx']) {
      expect(src(rel)).not.toContain("useDisplaySwitch('in063')")
      expect(src(rel)).not.toContain("useDisplaySwitch('in064')")
    }
  })
})
