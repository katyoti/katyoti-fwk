// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { addressOf, homeStripId, parseAddress, stepAddress } from '../src/address'
import fixture from './fixtures/addresses.json'

// The GUI starts on an element by its address
// `<universe>/<place>/<path>` - from `--open`, a katyoti:// link, or the
// development page's `#/open/<address>`.
describe('an address', () => {
  test('names an object\'s tab, its children of a type, or a section of its Information', () => {
    expect(parseAddress('dev/object/m.F-23/links')).toEqual({ kind: 'object', universe: 'dev', id: 'm.F-23', tab: 'tab:links' })
    expect(parseAddress('dev/object/m.F-23/children/record')).toEqual({ kind: 'object', universe: 'dev', id: 'm.F-23', tab: 'tab:children/record' })
    expect(parseAddress('dev/object/m.F-23/information/fields')).toEqual({ kind: 'object', universe: 'dev', id: 'm.F-23', tab: 'tab:information', section: 'section:fields' })
    expect(parseAddress('dev/object/m.F-23/information/cap/plan')).toEqual({ kind: 'object', universe: 'dev', id: 'm.F-23', tab: 'tab:information', section: 'section:cap/plan' })
  })
  test("without a tab leaves it unset: the pane opens on its type's first tab, not Information", () => {
    const t = parseAddress('dev/object/m.F-23')
    expect(t).toEqual({ kind: 'object', universe: 'dev', id: 'm.F-23' })
    expect('tab' in t).toBe(false)
    // openAddress hands no panel to openObjectTab, which then sets no activeKind.
    const open = readFileSync(new URL('../src/sections/shell/openAddress.ts', import.meta.url), 'utf8')
    expect(open).toContain('openObjectTab(t.universe, { id: t.id, panel: t.tab })')
    const tab = readFileSync(new URL('../src/sections/workspace/openObjectTab.tsx', import.meta.url), 'utf8')
    expect(tab).toContain('...(target.panel ? { activeKind: objectTabId(target.panel) } : {})')
  })
  test('names Home\'s entry, a screen and its section, or what a rail entry opens', () => {
    expect(parseAddress('archi/home/mes-etudes')).toEqual({ kind: 'home', universe: 'archi', entry: 'mes-etudes' })
    expect(homeStripId('mes-etudes')).toBe('home:mes-etudes')
    expect(homeStripId('runs')).toBe('runs')
    expect(parseAddress('archi/screen/marketplace/sidecars')).toEqual({ kind: 'screen', universe: 'archi', screen: 'marketplace', section: 'sidecars' })
    expect(parseAddress('archi/screen/debug/skills')).toEqual({ kind: 'screen', universe: 'archi', screen: 'debug', section: 'skills' })
    expect(parseAddress('archi/rail/inbox')).toEqual({ kind: 'screen', universe: 'archi', screen: 'inbox' })
  })
  test('comes as a link or from the page, and an unknown place or screen is refused with why', () => {
    expect(addressOf('katyoti://open/dev/home/runs')).toBe('dev/home/runs')
    expect(addressOf('#/open/dev%2Fhome%2Fruns')).toBe('dev/home/runs')
    for (const bad of ['dev', 'dev/nowhere/x', 'dev/screen/garden', 'dev/rail/camera', 'dev/object']) {
      expect('error' in parseAddress(bad)).toBe(true)
    }
  })
  test('the GUI opens it at start and when a link reaches the running window', () => {
    const boot = readFileSync(new URL('../src/hooks/useUniverses.ts', import.meta.url), 'utf8')
    expect(boot).toContain('if (open) await openAddress(open)')
    expect(boot).toContain("EventsOn('open.address'")
  })
})

// A recipe step may name an element to open first - its
// address in the person's universe; the player opens it, then spotlights.
describe('a guide step that opens an element', () => {
  test('names it in the universe shown', () => {
    expect(stepAddress('archi', 'home/mes-etudes')).toBe('archi/home/mes-etudes')
    expect(stepAddress('archi', '/screen/marketplace/sidecars')).toBe('archi/screen/marketplace/sidecars')
    expect(parseAddress(stepAddress('archi', 'object/acme.F-25/cadrage'))).toEqual({ kind: 'object', universe: 'archi', id: 'acme.F-25', tab: 'tab:cadrage' })
  })
  test('the player opens it before it looks for the step control', () => {
    const player = readFileSync(new URL('../src/guide/GuidePlayer.tsx', import.meta.url), 'utf8')
    const open = player.indexOf('void openAddress(stepAddress(universe, step.open))')
    expect(open).toBeGreaterThan(0)
    expect(open).toBeLessThan(player.indexOf('const el = resolveTarget(step.target'))
  })
})

// The examples the scenario's Go ParseAddress reads too
// (internal/extensions/scenario/api/address_test.go): one grammar, two
// languages, the same answers.
describe('the shared address examples', () => {
  for (const c of fixture.opens) {
    test(`opens ${c.raw}`, () => {
      expect(parseAddress(c.raw)).toEqual(c.want)
    })
  }
  for (const c of fixture.refused) {
    test(`refuses ${c.raw}`, () => {
      const got = parseAddress(c.raw) as { error?: string }
      expect(got.error ?? '').toContain(c.error)
    })
  }
})
