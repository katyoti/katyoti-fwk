// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { CHANGE_LINE_MAX, changeLine, incrementOf, switchRows } from '../src/sections/dev/displaySwitchRows'
import { DisplaySwitchesTable } from '../src/sections/dev/DisplaySwitchesPanel'
import fr from '../src/locales/fr/shell.json'

// The debug view's « Interrupteurs d'affichage »: one table, one
// row per switch feature tagged display-switch - Variable | Incrément |
// Ce qu'il change | État - then the dev front-end's list.
const features = [
  { id: 'm.F-063', name: "Planification resilience's screen changes (the increment's Blockers tab, unclassified waits) stay behind a display switch until the person approves the increment's delivery", state: 'done' },
  { id: 'm.F-070', name: "One source selector's source-model screen changes (source cards, the one header, the reader) stay behind its display switch, the former display showing", state: 'done' },
  { id: 'm.F-099', name: 'A switch no increment takes stays behind nothing', state: 'done' },
]
const takers = { 'm.F-063': 'ops.IN-1056', 'm.F-070': 'ops.IN-045' }
const states = [
  { increment: 'ops.IN-1056', variable: 'KTW_IN1056_DISPLAY', on: true },
  { increment: 'ops.IN-045', variable: 'KTW_IN045_DISPLAY', on: false },
]

const t = (key: string) => {
  const v = key.split('.').reduce<unknown>((o, k) => (o as Record<string, unknown>)?.[k], fr)
  return typeof v === 'string' ? v : key
}

describe('display switches table', () => {
  test('one line of what a switch changes, cut before « stay behind »', () => {
    expect(changeLine(features[0].name)).toBe("Planification resilience's screen changes (the increment's Blockers tab, unclassified waits)")
    expect(changeLine('Ask-once screen changes - the one list - stay behind an interaction switch')).toBe('Ask-once screen changes - the one list')
    expect(changeLine('No cut here')).toBe('No cut here')
    // A name with no « stays behind » is cut at
    // its first colon or semicolon outside parentheses, then capped.
    expect(changeLine("The study's pane holds the whole as-is study, its tabs on the left: a header with its perimeter; the panels")).toBe("The study's pane holds the whole as-is study, its tabs on the left")
    expect(changeLine('Sources (the reader: one pane) show once; the rest stays')).toBe('Sources (the reader: one pane) show once')
    const long = changeLine('word '.repeat(60))
    expect(long.length).toBeLessThanOrEqual(CHANGE_LINE_MAX)
    expect(long.endsWith('word…')).toBe(true)
  })

  test('the increment is the one taking the feature, never a release', () => {
    expect(incrementOf(['ops.RL-034', 'ops.IN-1056'])).toBe('ops.IN-1056')
    expect(incrementOf(['ops.RL-034'])).toBe('')
  })

  test('rows by variable, the unknown increment last, the dev list at the end', () => {
    const rows = switchRows(features, takers, states, { enabled: true, value: 'in-036, IN-037' })
    expect(rows.map((r) => r.variable)).toEqual(['KTW_IN045_DISPLAY', 'KTW_IN1056_DISPLAY', '', 'VITE_KTW_DISPLAY_SWITCHES'])
    expect(rows.map((r) => r.on)).toEqual([false, true, false, true])
    expect(rows[3].listed).toEqual(['in-036', 'in-037'])
    // Outside a dev build the dev list is off whatever it says.
    expect(switchRows(features, takers, states, { enabled: false, value: 'in-036' }).at(-1)?.on).toBe(false)
  })

  test('a switch not built yet reads « pas encore construit », never on nor « éteint »', () => {
    // A switch feature tagged before it is built: no code reads its variable
    // yet, so even set it turns nothing on (a review's finding).
    const unbuilt = [{ id: 'm.F-9001', name: "An increment's screen changes stay behind a display switch", state: 'idea' }]
    const rows = switchRows(unbuilt, { 'm.F-9001': 'ops.IN-9001' },
      [{ increment: 'ops.IN-9001', variable: 'KTW_IN9001_DISPLAY', on: true }], { enabled: false, value: '' })
    expect(rows[0]).toMatchObject({ variable: 'KTW_IN9001_DISPLAY', built: false, on: false })
    const html = renderToStaticMarkup(
      <MantineProvider><DisplaySwitchesTable rows={rows} t={t} onOpen={() => {}} /></MantineProvider>,
    )
    const row = html.slice(html.indexOf('<tbody')).split('<tr')[1]
    expect(row).toMatch(/display-switch-state-KTW_IN9001_DISPLAY"[^>]*>(<[^>]*>)*pas encore construit/)
    expect(row).not.toMatch(/éteint|allumé/)
  })

  test('the table: four columns in order, the increment a link, the state as started', () => {
    const rows = switchRows(features, takers, states, { enabled: false, value: '' })
    const html = renderToStaticMarkup(
      <MantineProvider><DisplaySwitchesTable rows={rows} t={t} onOpen={() => {}} /></MantineProvider>,
    )
    const heads = [...html.matchAll(/<th[^>]*>([^<]*)<\/th>/g)].map((m) => m[1])
    expect(heads).toEqual(['Variable', 'Incrément', 'Ce qu&#x27;il change', 'État'])
    const body = html.slice(html.indexOf('<tbody'))
    const rowsHtml = body.split('<tr').slice(1)
    expect(rowsHtml.length).toBe(4)
    // KTW_IN045_DISPLAY | IN-045 (a link) | its line | éteint
    expect(rowsHtml[0]).toContain('KTW_IN045_DISPLAY')
    expect(rowsHtml[0]).toContain('data-element="display-switch-open-ops.IN-045"')
    expect(rowsHtml[0]).toContain('>IN-045<')
    expect(rowsHtml[0]).toContain('One source selector')
    // The cell's title keeps the feature's full name.
    expect(rowsHtml[0]).toContain('title="One source selector&#x27;s source-model screen changes (source cards, the one header, the reader) stay behind its display switch, the former display showing"')
    expect(rowsHtml[0]).toMatch(/display-switch-state-KTW_IN045_DISPLAY"[^>]*>(<[^>]*>)*éteint/)
    expect(rowsHtml[1]).toMatch(/display-switch-state-KTW_IN1056_DISPLAY"[^>]*>(<[^>]*>)*allumé/)
    expect(rowsHtml[2]).toContain('aucun incrément')
    expect(rowsHtml[3]).toContain('VITE_KTW_DISPLAY_SWITCHES')
    expect(rowsHtml[3]).toContain('(GUI de dev)')
    expect(rowsHtml[3]).toMatch(/éteint/)
    // No toggling: nothing in the table is a switch or a checkbox.
    expect(html).not.toMatch(/type="checkbox"|role="switch"/)
  })
})
