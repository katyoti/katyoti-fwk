// SPDX-License-Identifier: Apache-2.0
// One set of display components: every GUI view draws a figure, a titled
// panel, an item, a state and an empty view through ktw-display-lib's shared
// components (KpiTile, SectionCard, ItemCard, StateBadge, EmptyState), never
// a look of its own.

import { describe, expect, it } from 'bun:test'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { stateTone } from '@ktw/display-lib/ui-core/components/other/StateBadge.tsx'
import { toneBorder } from '@ktw/display-lib/ui-core/components/other/ItemCard.tsx'

const sections = join(import.meta.dir, '../src/sections')
const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => {
  const p = join(dir, f)
  return statSync(p).isDirectory() ? files(p) : /\.(tsx|css)$/.test(f) ? [p] : []
})
const rel = (p: string) => p.slice(sections.length + 1).replaceAll('\\', '/')
const offenders = (re: RegExp, allowed: string[] = []) =>
  files(sections).filter((p) => re.test(readFileSync(p, 'utf8'))).map(rel).filter((p) => !allowed.includes(p))

describe('one set of display components', () => {
  it('no view declares its own tile, titled panel, status badge or empty state', () => {
    expect(offenders(/function (Kpi|KpiCard|KpiTile|SectionCard|StatusBadge|StateBadge|EmptyState)\b/)).toEqual([])
  })
  it('no view draws a tone border by hand: an item takes its tone through ItemCard', () => {
    // The chat message frame is a frame, not an item card.
    expect(offenders(/3px solid var\(--mantine-color-/, ['agent/MessageWrapperBox.tsx'])).toEqual([])
  })
  it('no view styles its own KPI tile', () => {
    expect(offenders(/\.[a-z]+-kpi\b|className="[^"]*-kpi\b/)).toEqual([])
  })
  it('the page DSL draws its list items and lane cards through ItemCard: a planification card is a GUI item card', () => {
    const interactive = readFileSync(join(import.meta.dir, '../../../ktw-display-lib/src/ui-builders/page/nodes/interactive.tsx'), 'utf8')
    expect(interactive).toContain('<ItemCard')
    expect(interactive).not.toMatch(/function accent\b|<(Card|Paper) withBorder[^>]*\n?[^>]*accent\(/)
    // an item's tone is ItemCard's left border; a lane column keeps its coloured top
    expect(interactive.match(/borderLeft: `3px solid var\(--mantine-color-/g) ?? []).toEqual([])
  })
})

describe('the shared pieces', () => {
  it('a state takes its view\'s tone, gray when unknown', () => {
    expect(stateTone('running', { running: 'green' })).toBe('green')
    expect(stateTone('other', { running: 'green' })).toBe('gray')
    expect(stateTone('x')).toBe('gray')
  })
  it('an item\'s tone is a 3px left border, none without a tone', () => {
    expect(toneBorder('red')?.borderLeft).toBe('3px solid var(--mantine-color-red-6)')
    expect(toneBorder(undefined)).toBeUndefined()
    expect(toneBorder('blue', true)?.animation).toContain('ktw-pulse')
  })
})
