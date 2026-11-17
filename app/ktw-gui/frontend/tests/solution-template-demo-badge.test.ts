// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'bun:test'
import { readFileSync } from 'node:fs'
import { isDemoSeed } from '../src/sections/marketplace/demoSeeds'

// A solution's « Modèle de projet » line carries the « Démo » badge when its
// template is demo data, from the same flag the Project
// templates tab reads (list-seeds' demo).
const seeds = [
  { name: 'acme-demo', marketplace: 'acme', demo: true },
  { name: 'archifactory', marketplace: 'acme' },
  { name: 'shared', marketplace: 'other', demo: true },
  { name: 'shared', marketplace: 'acme', demo: false },
]

describe('the demo badge on a solution template line', () => {
  it('is present for a demo seed', () => {
    expect(isDemoSeed(seeds, 'acme-demo', 'acme')).toBe(true)
  })
  it('is absent for a seed not marked demo, or not listed', () => {
    expect(isDemoSeed(seeds, 'archifactory', 'acme')).toBe(false)
    expect(isDemoSeed(seeds, 'unknown', 'acme')).toBe(false)
    expect(isDemoSeed([], 'acme-demo', 'acme')).toBe(false)
  })
  it('reads the seed of the solution own marketplace first', () => {
    expect(isDemoSeed(seeds, 'shared', 'acme')).toBe(false)
    expect(isDemoSeed(seeds, 'shared', 'other')).toBe(true)
    expect(isDemoSeed(seeds, 'acme-demo', 'elsewhere')).toBe(true)
  })
  it('SolutionsPanel draws the badge beside the template line from that flag', () => {
    const src = readFileSync(new URL('../src/sections/marketplace/SolutionsPanel.tsx', import.meta.url), 'utf8')
    expect(src).toContain('isDemoSeed(seeds, latest.seed.name, it.marketplace)')
    expect(src).toContain(`{r.seedDemo && <Badge`)
    expect(src).toContain(`data-guide="solution.seed-demo"`)
  })
})
