// SPDX-License-Identifier: Apache-2.0
// The Contributions panel's model (sys-contributions): the source's answer
// read defensively, the groups a filter keeps, the count; and the view is the
// one registered under the name the panel declares.

import { describe, expect, it } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { asContributions, countOf, filterGroups, filterOverrides, partsOf, type ContributionGroup } from '../src/sections/workspace/contributionsModel'

const groups: ContributionGroup[] = [
  { kind: 'skill', entries: [
    { uri: 'acme:/skill/greet', id: 'greet', name: 'Greet', description: 'Say hello' },
    { uri: 'acme:/skill/review', id: 'review', name: 'Review', description: 'Review a change' },
  ] },
  { kind: 'task', entries: [{ uri: 'acme:/task/ship', id: 'ship', name: 'Ship' }] },
]

describe('contributions panel model', () => {
  it('reads a source that answered nothing as no group', () => {
    expect(asContributions(undefined)).toEqual({ provider: '', folder: '', groups: [], overrides: [] })
    expect(asContributions({ provider: 'acme', groups: 'x' }).groups).toEqual([])
  })

  it('counts every entry', () => {
    expect(countOf(groups)).toBe(3)
    expect(countOf([])).toBe(0)
  })

  it('keeps every group for an empty filter', () => {
    expect(filterGroups(groups, '  ')).toBe(groups)
  })

  it('keeps the matching entries, by id, name or description, case-folded, and drops empty groups', () => {
    const got = filterGroups(groups, 'HELLO')
    expect(got.map((g) => g.kind)).toEqual(['skill'])
    expect(got[0].entries.map((e) => e.id)).toEqual(['greet'])
    expect(filterGroups(groups, 'nothing')).toEqual([])
  })

  it('keeps a whole group whose kind matches', () => {
    expect(filterGroups(groups, 'task')).toEqual([groups[1]])
  })
})

describe('contributions panel registration', () => {
  it('registers the view under the name the panel declares', () => {
    const panel = readFileSync(join(import.meta.dir, '../../../../internal/extensions/projectcontributions/contrib/embed/cwm/object-panels/contributions.yaml'), 'utf8')
    const name = /extension:\s*"?([a-z-]+)"?/.exec(panel)?.[1]
    expect(name).toBe('contributions')
    const registry = readFileSync(join(import.meta.dir, '../src/sections/workspace/RenderDefView.tsx'), 'utf8')
    expect(registry).toMatch(new RegExp(`\\n\\s+'?${name}'?: \\(\\{[^}]*\\}\\) => \\(\\s*<ContributionsView`))
  })
})

describe('contributions panel overrides', () => {
  const overrides = [
    { uri: 'ktw-compose:/skill/greet', layers: ['instructions', 'source'] },
    { uri: 'ktw-agents:/guide-recipe/tour', layers: ['translations'] },
  ]

  it('reads the overrides, none when the source gave none', () => {
    expect(asContributions({ overrides }).overrides).toEqual(overrides)
    expect(asContributions({}).overrides).toEqual([])
  })

  it('filters the overridden entries by URI or layer', () => {
    expect(filterOverrides(overrides, '').length).toBe(2)
    expect(filterOverrides(overrides, 'TRANSLATIONS').map((o) => o.uri)).toEqual(['ktw-agents:/guide-recipe/tour'])
    expect(filterOverrides(overrides, 'compose').map((o) => o.uri)).toEqual(['ktw-compose:/skill/greet'])
  })

  it('reads a URI into its provider, kind and id', () => {
    expect(partsOf('ktw-compose:/skill/greet')).toEqual({ provider: 'ktw-compose', kind: 'skill', id: 'greet' })
    expect(partsOf('ktw-agents:/task/sub/review')).toEqual({ provider: 'ktw-agents', kind: 'task', id: 'sub/review' })
  })
})
