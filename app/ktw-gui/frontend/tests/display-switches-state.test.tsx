// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { switchRows } from '../src/sections/dev/displaySwitchRows'
import { DisplaySwitchesTable } from '../src/sections/dev/DisplaySwitchesPanel'
import fr from '../src/locales/fr/shell.json'

// The debug view's « Interrupteurs d'affichage » shows each
// switch's state whole - the État column fits its longest value, Variable and
// Incrément never wrap, « Ce qu'il change » takes the rest.
const features = [
  { id: 'm.F-063', name: "Planification resilience's screen changes (the increment's Blockers tab, unclassified waits) stay behind a display switch until the person approves the increment's delivery", state: 'done' },
  { id: 'm.F-070', name: "One source selector's source-model screen changes (source cards, the one header, the reader) stay behind its display switch, the former display showing", state: 'done' },
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

describe('display switches state column', () => {
  test('the state is shown whole: its column fits its longest value, the badge never clips', () => {
    // The person saw « o.. » on every row: the long « Ce qu'il change » text
    // squeezed the last column and Mantine's Badge cut its label.
    const rows = switchRows(features, takers, states, { enabled: false, value: '' })
    const html = renderToStaticMarkup(
      <MantineProvider><DisplaySwitchesTable rows={rows} t={t} onOpen={() => {}} /></MantineProvider>,
    )
    const fits = (cell: string) => /white-space:\s*nowrap/.test(cell) && /width:\s*1%/.test(cell)
    const heads = [...html.matchAll(/<th(\s[^>]*)?>/g)].map((m) => m[1] ?? '')
    expect(heads.map(fits)).toEqual([true, true, false, true])
    const row = html.slice(html.indexOf('<tbody')).split('<tr')[1]
    const cells = [...row.matchAll(/<td(\s[^>]*)?>/g)].map((m) => m[1] ?? '')
    expect(cells.map(fits)).toEqual([true, true, false, true])
    // The badge and its label show their overflow: nothing clips « éteint ».
    const at = row.indexOf('data-element="display-switch-state-KTW_IN045_DISPLAY"')
    const badgeTag = row.slice(row.lastIndexOf('<', at), row.indexOf('>', at))
    expect(badgeTag).toMatch(/overflow:\s*visible/)
    expect(row.slice(at)).toMatch(/<span[^>]*style="[^"]*overflow:\s*visible[^"]*"[^>]*>éteint/)
  })

  test("the dev row's list of switches wraps: only the badge keeps nowrap", () => {
    // With many dev switches on, the list sat in the nowrap État cell and grew
    // the column to its whole length, squeezing « Ce qu'il change ».
    const value = 'ops.IN-045, ops.IN-053, ops.IN-1056, ops.IN-080'
    const rows = switchRows(features, takers, states, { enabled: true, value })
    const html = renderToStaticMarkup(
      <MantineProvider><DisplaySwitchesTable rows={rows} t={t} onOpen={() => {}} /></MantineProvider>,
    )
    const at = html.indexOf('data-element="display-switch-dev-listed"')
    expect(at).toBeGreaterThan(-1)
    const tag = html.slice(html.lastIndexOf('<', at), html.indexOf('>', at))
    expect(tag).toMatch(/white-space:\s*normal/)
    expect(html.slice(at)).toContain('ops.in-080')
  })
})
