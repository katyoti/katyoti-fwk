// SPDX-License-Identifier: Apache-2.0
// The tickable picker's proposal region (mock-up v6) - ticks
// at the left, search hits ticked the same way, every chosen item as a chip,
// and a save bar « N choisi(e)s » with « Annuler » / « Enregistrer et lire ».

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { chosenChips, countLine, PickerProposalsRenderer, proposedKeys, saveBarState, tickAllProposals, tickHit, undoTo } from '../src/ui-builders/page/nodes/picker-proposals.tsx'

const proposals = [
  { key: 'A1', title: 'Arbor', why: 'mot-clé « trafic »' },
  { key: 'A2', title: 'Parko', off: true },
]
const proposed = new Set(['A1', 'A2'])
const keys = (s: { key: string; off?: boolean }[]) => s.filter((x) => !x.off).map((x) => x.key)

describe('the picker proposal region', () => {
  test('a hit ticked joins the choice ticked', () => {
    const got = tickHit(proposals, { key: 'B9', title: 'Billy' }, true, proposed)
    expect(keys(got)).toEqual(['A1', 'B9'])
    // a proposal found again is ticked back, never added twice
    const back = tickHit(proposals, { key: 'A2', title: 'Parko' }, true, proposed)
    expect(back.length).toBe(2)
    expect(keys(back)).toEqual(['A1', 'A2'])
  })

  test('unticked, a proposal stays unticked and another item goes', () => {
    const withHit = tickHit(proposals, { key: 'B9', title: 'Billy' }, true, proposed)
    const offProp = tickHit(withHit, { key: 'A1', title: 'Arbor' }, false, proposed)
    expect(offProp.find((s) => s.key === 'A1')?.off).toBe(true)
    const gone = tickHit(withHit, { key: 'B9', title: 'Billy' }, false, proposed)
    expect(gone.some((s) => s.key === 'B9')).toBe(false)
  })

  test('chips list every chosen item, proposals included', () => {
    const withHit = tickHit(proposals, { key: 'B9', title: 'Billy' }, true, proposed)
    expect(chosenChips(withHit).map((s) => s.key)).toEqual(['A1', 'B9'])
  })

  test('the save bar is dirty only while the ticked keys differ from the held ones', () => {
    expect(saveBarState(proposals, proposals)).toEqual({ count: 1, dirty: false })
    const ticked = tickAllProposals(proposals, proposed)
    expect(saveBarState(ticked, proposals)).toEqual({ count: 2, dirty: true })
    // back to the same ticks by another way: not dirty
    expect(saveBarState(tickHit(ticked, proposals[1], false, proposed), proposals).dirty).toBe(false)
  })

  test('« Annuler » restores the held choice', () => {
    const changed = tickHit(proposals, { key: 'B9', title: 'Billy' }, true, proposed)
    const back = undoTo(proposals)
    expect(back).toEqual(proposals)
    expect(saveBarState(back, proposals).dirty).toBe(false)
    expect(saveBarState(changed, proposals).dirty).toBe(true)
  })

  test('an item chosen but not proposed is a chip only, gone when unticked', () => {
    const selected = [...proposals, { key: 'C3', title: 'Chosy', proposed: false }]
    const keysProposed = proposedKeys(selected)
    expect([...keysProposed]).toEqual(['A1', 'A2'])
    expect(chosenChips(selected).map((s) => s.key)).toEqual(['A1', 'C3'])
    const gone = tickHit(selected, selected[2], false, keysProposed)
    expect(gone.some((s) => s.key === 'C3')).toBe(false)
    const node = {
      kind: 'picker', id: 'apps', tickable: true, search: 'x:apps/search', apply: 'x:apps/hold', selected,
      proposals: { title: 'Proposé d’après vos mots-clés' },
    }
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({ ok: true }) } as any },
        createElement(PickerProposalsRenderer, { node: node as any }))))
    expect(html.match(/data-guide="picker.proposal"/g)?.length).toBe(2)
    expect(html).not.toContain('data-guide="picker.proposal" data-guide-name="C3"')
    expect(html).toContain('data-guide="picker.chip" data-guide-name="C3"')
  })

  test('the count is singular up to one', () => {
    expect(countLine(0, 'application choisie', 'applications choisies')).toBe('0 application choisie')
    expect(countLine(1, 'application choisie', 'applications choisies')).toBe('1 application choisie')
    expect(countLine(3, 'application choisie', 'applications choisies')).toBe('3 applications choisies')
  })

  test('is declared on the picker', () => {
    const p = (UIPageNodeJSONSchema as any).definitions.Picker.properties
    expect(p.proposals.required).toEqual(['title'])
    expect(p.savebar.required).toEqual(['one', 'many'])
    expect(p.selected.items.properties.proposed.type).toBe('boolean')
  })

  test('draws the proposals ticked at the left, the chips and the idle save bar', () => {
    const node = {
      kind: 'picker', id: 'apps', tickable: true, search: 'x:apps/search', apply: 'x:apps/hold', selected: proposals,
      proposals: { title: 'Proposé d’après vos mots-clés' },
      savebar: { one: 'application choisie', many: 'applications choisies', idle: 'Enregistré · lu il y a 2 h', empty: 'Rien de coché' },
      labels: { search: 'Chercher une autre application (nom ou code)' },
    }
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({ ok: true }) } as any },
        createElement(PickerProposalsRenderer, { node: node as any }))))
    expect(html.match(/data-guide="picker.proposal"/g)?.length).toBe(2)
    expect(html).toContain('data-guide="picker.chip" data-guide-name="A1"')
    expect(html).not.toContain('data-guide="picker.chip" data-guide-name="A2"')
    expect(html).toContain('1 application choisie')
    expect(html).toContain('Enregistré · lu il y a 2 h')
    expect(html).not.toContain('data-guide="picker.save"')
    expect(html).toContain('Tout cocher')
    expect(html).not.toContain('Sélection')
  })
})
