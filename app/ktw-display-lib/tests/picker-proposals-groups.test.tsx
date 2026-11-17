// SPDX-License-Identifier: Apache-2.0
// The picker's proposal region drawn by group (mock-up v6's dtGroup): a
// head per group with its box on, mixed or off, badge, title, code, tag chip
// and « k sur n », the group's empty line when it holds nothing.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { groupedProposals, PickerProposalsRenderer, proposalGroups, tickProposalGroup } from '../src/ui-builders/page/nodes/picker-proposals.tsx'
import type { UIPickerGroup, UIPickerItem, UIPickerNode } from '../src/ui-builders/page/ui-page.interactive.ts'

const declared: UIPickerGroup[] = [
  { key: 'orchard', title: 'Orchard', badge: { label: 'EX' }, code: 'ORC', chip: 'app:orc', badges: ['fresh'] },
  { key: 'meadow', title: 'Meadow', code: 'MDW', empty: 'Nothing carries app:mdw.' },
]
const items: UIPickerItem[] = [
  { key: 'e1', title: 'pear-service', group: 'orchard' },
  { key: 'e2', title: 'plum-service', group: 'orchard', off: true },
  { key: 'e3', title: 'loose-host' },
  { key: 'e4', title: 'brook-db', group: 'river' },
]
const keys = (s: UIPickerItem[]) => s.filter((x) => !x.off).map((x) => x.key)

describe('the grouped proposal region', () => {
  test('declared groups first in their order, empty kept, then undeclared, then ungrouped', () => {
    const got = proposalGroups(items, declared)
    expect(got.map((g) => g.key)).toEqual(['orchard', 'meadow', 'river', ''])
    expect(got.map((g) => g.items.length)).toEqual([2, 0, 1, 1])
    expect(got[0].decl?.title).toBe('Orchard')
    expect(got[2].decl).toBeUndefined()
  })

  test('the box is on, mixed or off by the ticked count; none for an empty group', () => {
    const got = proposalGroups(items, declared)
    expect(got[0]).toMatchObject({ ticked: 1, box: 'mixed' })
    expect(got[1].box).toBeUndefined()
    expect(got[2]).toMatchObject({ ticked: 1, box: 'on' })
    const off = proposalGroups(items.map((s) => ({ ...s, off: true })), declared)
    expect(off[0]).toMatchObject({ ticked: 0, box: 'off' })
    expect(proposalGroups(items, declared, new Set(['e1', 'e2']))[0].box).toBe('on')
  })

  test('grouped only with declared groups or a proposal carrying a group', () => {
    expect(groupedProposals([{ key: 'a', title: 'A' }], undefined)).toBe(false)
    expect(groupedProposals([{ key: 'a', title: 'A' }], declared)).toBe(true)
    expect(groupedProposals([{ key: 'a', title: 'A', group: 'g' }], [])).toBe(true)
  })

  test('a group box ticks and unticks its proposals only', () => {
    const sel: UIPickerItem[] = [...items, { key: 'e9', title: 'old-one', group: 'orchard', off: true, proposed: false }]
    const proposed = new Set(['e1', 'e2', 'e3', 'e4'])
    const on = tickProposalGroup(sel, 'orchard', true, proposed)
    expect(keys(on)).toEqual(['e1', 'e2', 'e3', 'e4'])
    expect(on.find((s) => s.key === 'e9')?.off).toBe(true)
    const kept: UIPickerItem[] = sel.map((s) => (s.key === 'e9' ? { ...s, off: undefined } : s))
    const off = tickProposalGroup(kept, 'orchard', false, proposed)
    expect(keys(off)).toEqual(['e3', 'e4', 'e9'])
  })

  test('the region draws each group head, its box mixed, « 1 sur 2 », and the empty line', () => {
    const node: UIPickerNode = {
      kind: 'picker', tickable: true,
      proposals: { title: 'Entities', groups: declared },
      badges: { fresh: { label: 'just added', color: 'orange' } },
      selected: items.slice(0, 2),
    }
    const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(PickerProposalsRenderer, { node })))
    expect(html).toContain('data-guide="picker.prop-group" data-guide-name="orchard"')
    expect(html).toContain('data-guide="picker.prop-group-box"')
    expect(html).toMatch(/aria-checked="mixed"[^>]*data-guide="picker.prop-group-box"|data-guide="picker.prop-group-box"[^>]*aria-checked="mixed"/)
    expect(html).toContain('1 sur 2')
    expect(html).toContain('>EX<')
    expect(html).toContain('>ORC<')
    expect(html).toContain('app:orc')
    expect(html).toContain('just added')
    expect(html).toContain('data-guide="picker.prop-group-empty"')
    expect(html).toContain('Nothing carries app:mdw.')
    // the empty group has no box
    expect(html.match(/data-guide="picker.prop-group-box"/g)?.length).toBe(1)
  })
})

describe('a badge declared unticked', () => {
  test('is drawn on a proposed row only while the row is unticked; other badges stay', () => {
    const node: UIPickerNode = {
      kind: 'picker', tickable: true,
      proposals: { title: 'Entities' },
      badges: { fresh: { label: 'brand-new', color: 'blue', unticked: true }, kept: { label: 'always-here' } },
      selected: [
        { key: 'r1', title: 'ticked-row', badges: ['fresh', 'kept'] },
        { key: 'r2', title: 'unticked-row', badges: ['fresh', 'kept'], off: true },
      ],
    }
    const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(PickerProposalsRenderer, { node })))
    expect(html.match(/brand-new/g)?.length).toBe(1)
    expect(html.match(/always-here/g)?.length).toBe(2)
    const unticked = html.slice(html.indexOf('unticked-row'))
    expect(unticked).toContain('brand-new')
  })
})
