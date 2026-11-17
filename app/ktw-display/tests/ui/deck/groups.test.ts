// SPDX-License-Identifier: Apache-2.0
// Slide groups (ktw-display-lib deck/groups.ts, th22): an include slide shows
// the group's slides in its place, renumbered, marked as the group's.

import { describe, expect, it } from 'bun:test'
import { parseDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/parse.ts'
import { expandGroups, includesOf, inlineGroups } from '../../../../ktw-display-lib/src/ui-builders/deck/groups.ts'

const deck = parseDeck('---\ntitle: T\n---\n\n# Cover\n\n---\n\n<!-- slide: include=team -->\n\n---\n\n## Close\n')
const team = '## Our team\n- Ann\n\n--\n\n## Ann\n- architect\n\n---\n\n## How we work\n- weekly\n'

describe('expandGroups', () => {
  it('names the groups a deck includes', () => {
    expect(includesOf(deck)).toEqual(['team'])
  })
  it('puts the group in the include slide\'s place, renumbered and marked', () => {
    const d = expandGroups(deck, { team })
    expect(d.slides.map((s) => [s.h, s.v, s.group ?? ''])).toEqual([
      [0, 0, ''], [1, 0, 'team'], [1, 1, 'team'], [2, 0, 'team'], [3, 0, ''],
    ])
    expect(d.slides[4]!.slots[0]!.markdown).toBe('## Close')
  })
  it('shows a group it does not have as not found, in its place', () => {
    const d = expandGroups(deck, {})
    expect(d.slides.length).toBe(3)
    expect(d.slides[1]!.group).toBe('team')
    expect(d.slides[1]!.slots[0]!.markdown).toContain('Not found')
  })
  it('does not follow a group\'s own includes, and leaves a deck without includes as is', () => {
    const d = expandGroups(deck, { team: '## A\n\n---\n\n<!-- slide: include=team -->\n' })
    expect(d.slides.map((s) => s.group ?? '')).toEqual(['', 'team', ''])
    const plain = parseDeck('---\ntitle: T\n---\n\n# A\n')
    expect(expandGroups(plain, { team })).toBe(plain)
  })
})

describe('inlineGroups', () => {
  it('writes the group in place of its include, and parses to the expanded deck', () => {
    const md = '---\ntitle: T\n---\n\n# Cover\n\n---\n\n<!-- slide: include=team -->\n\n---\n\n## Close\n'
    const out = inlineGroups(md, { team })
    expect(out).not.toContain('include=team')
    const flat = parseDeck(out).slides.map((s) => [s.h, s.v])
    const expanded = expandGroups(parseDeck(md), { team }).slides.map((s) => [s.h, s.v])
    expect(flat).toEqual(expanded)
    expect(inlineGroups(md, {})).toBe(md)
  })
})

import { slideIndexAt } from '../../../../ktw-display-lib/src/ui-builders/deck/settings.ts'

describe('slideIndexAt with groups', () => {
  it('maps a shown slide back to the deck\'s own, and a group\'s slide to none', () => {
    const md = '---\ntitle: T\n---\n\n# Cover\n\n---\n\n<!-- slide: include=team -->\n\n---\n\n## Close\n'
    // Shown: 0 Cover, 1/1.1/2 the group, 3 Close.
    expect(slideIndexAt(md, 0, 0, { team })).toBe(0)
    expect(slideIndexAt(md, 1, 1, { team })).toBe(-1)
    expect(slideIndexAt(md, 3, 0, { team })).toBe(2)
    expect(slideIndexAt(md, 2, 0)).toBe(2)
  })
})
