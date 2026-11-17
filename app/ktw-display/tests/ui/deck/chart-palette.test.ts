// SPDX-License-Identifier: Apache-2.0
// A chart's series colours (ui-builders/deck/chart.tsx chartPalette): each
// slice or series shows against the slide and differs from the others.

import { describe, expect, it } from 'bun:test'
import { chartPalette, contrast, MIN_CONTRAST, MIN_DISTANCE } from '@ktw/display-lib/ui-builders/deck/chart.tsx'

const hex = (c: string): [number, number, number] => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16)) as [number, number, number]
const dist = (a: [number, number, number], b: [number, number, number]) => Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2])

// The default theme's tokens (deck.css): accent, accent-2 (the accent),
// heading, muted, border — the border is near the white slide.
const KATYOTI = ['#4f46e5', '#4f46e5', '#111827', '#6b7280', '#e5e7eb']
// A dark slide's.
const DARK = ['#818cf8', '#818cf8', '#f8fafc', '#94a3b8', '#334155']

describe('chartPalette', () => {
  for (const [name, tokens, bg] of [['a light slide', KATYOTI, '#ffffff'], ['a dark slide', DARK, '#0f172a']] as const) {
    for (let n = 2; n <= 8; n++) {
      it(`gives ${n} slices on ${name} colours that show and differ`, () => {
        const p = chartPalette([...tokens], bg).slice(0, n)
        expect(p).toHaveLength(n)
        p.forEach((c, i) => {
          expect(contrast(hex(c), hex(bg))).toBeGreaterThanOrEqual(MIN_CONTRAST)
          p.slice(0, i).forEach((d) => expect(dist(hex(c), hex(d))).toBeGreaterThanOrEqual(MIN_DISTANCE))
        })
      })
    }
  }
  it("keeps the theme's colours first and drops the one near the background", () => {
    const p = chartPalette(KATYOTI, '#ffffff')
    expect(p.slice(0, 3)).toEqual(['#4f46e5', '#111827', '#6b7280'])
    expect(p).not.toContain('#e5e7eb')
  })
})
