// SPDX-License-Identifier: Apache-2.0
// Charts in a slide (modules/slidedeck/): a fenced block `chart <kind>` with
// comma-separated data — a header (the category column, then one name per
// series) and one line per category — drawn in the theme's colours.
//
//   ```chart bar
//   Year, Build, Run
//   2024, 120, 40
//   2025, 90, 60
//   ```
//
// The same rules as the lint's (slidedeck/api/chart.go). The drawn chart
// carries its data (data-chart) so the editable PowerPoint rebuilds it as a
// native, editable chart (deck-pptx.ts).

import { useLayoutEffect, useRef, useState } from 'react'
import {
  Bar, BarChart, CartesianGrid, Cell, Legend, Line, LineChart, Pie, PieChart,
  ResponsiveContainer, Tooltip, XAxis, YAxis,
} from 'recharts'

export const CHART_KINDS = ['bar', 'line', 'pie'] as const
export type ChartKind = (typeof CHART_KINDS)[number]

export interface ChartData {
  kind: ChartKind
  categories: string[]
  series: string[]
  /** Per category, one value per series. */
  values: number[][]
}

/** Reads a chart block; an error says what to fix, as the lint does. */
export function parseChart(kind: string, body: string): ChartData | { error: string } {
  if (!(CHART_KINDS as readonly string[]).includes(kind)) return { error: `chart kind "${kind}": write \`\`\`chart ${CHART_KINDS.join('|')}` }
  const rows = body.split('\n').filter((l) => l.trim() !== '').map((l) => l.split(',').map((c) => c.trim()))
  if (rows.length < 2) return { error: 'a chart needs a header line and at least one data line' }
  const head = rows[0]!
  if (head.length < 2) return { error: 'the header names the category column and at least one series' }
  const series = head.slice(1)
  if (kind === 'pie' && series.length !== 1) return { error: `a pie chart has one series; this one has ${series.length}` }
  const categories: string[] = []
  const values: number[][] = []
  for (const [i, r] of rows.slice(1).entries()) {
    if (r.length !== head.length) return { error: `data line ${i + 1} has ${r.length} values; the header has ${head.length} columns` }
    const vals: number[] = []
    for (const cell of r.slice(1)) {
      const v = Number(cell.replace(/ /g, ''))
      if (cell === '' || Number.isNaN(v)) return { error: `data line ${i + 1}: "${cell}" is not a number (decimals with a point)` }
      vals.push(v)
    }
    categories.push(r[0]!)
    values.push(vals)
  }
  return { kind: kind as ChartKind, categories, series, values }
}

/** Colours added after the theme's, for a chart of many series or slices:
 *  vivid ones that show on a light slide, then deeper and paler ones, so a
 *  light or a dark background keeps eight of them. */
const EXTRA_COLOURS = ['#0ea5e9', '#f59e0b', '#10b981', '#ef4444', '#a855f7', '#0369a1', '#b45309', '#047857', '#be185d', '#7dd3fc', '#fcd34d', '#6ee7b7', '#fca5a5']

/** A CSS colour as RGB (#rgb, #rrggbb, rgb()/rgba()), or undefined. */
function rgbOf(c: string): [number, number, number] | undefined {
  const h = c.trim().toLowerCase()
  let m = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/.exec(h)
  if (m) return [parseInt(m[1]! + m[1]!, 16), parseInt(m[2]! + m[2]!, 16), parseInt(m[3]! + m[3]!, 16)]
  m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})?$/.exec(h)
  if (m) return [parseInt(m[1]!, 16), parseInt(m[2]!, 16), parseInt(m[3]!, 16)]
  m = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)/.exec(h)
  if (m) return [Number(m[1]), Number(m[2]), Number(m[3])]
  return undefined
}

/** WCAG relative luminance. */
function luminance([r, g, b]: [number, number, number]): number {
  const f = (v: number) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}

/** The contrast ratio of two colours (1 to 21). */
export function contrast(a: [number, number, number], b: [number, number, number]): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p)
  return (x! + 0.05) / (y! + 0.05)
}

/** How far a series colour must stand from the background, and from the
 *  colours before it (RGB distance): a slice or a bar that shows, and two
 *  that never look the same. */
export const MIN_CONTRAST = 2
export const MIN_DISTANCE = 60

/** The series colours: the theme's candidates first, then EXTRA_COLOURS,
 *  each kept only when it shows against the background and differs from
 *  those kept (a theme may give two tokens one colour, as a brand theme's accents,
 *  or a token close to the background, as its border on a white slide). */
export function chartPalette(candidates: string[], background: string): string[] {
  const bg = rgbOf(background)
  const kept: string[] = []
  const rgbs: [number, number, number][] = []
  for (const c of [...candidates, ...EXTRA_COLOURS]) {
    const rgb = rgbOf(c)
    if (!rgb) {
      if (!kept.some((k) => k.toLowerCase() === c.toLowerCase())) kept.push(c)
      continue
    }
    if (bg && contrast(rgb, bg) < MIN_CONTRAST) continue
    if (rgbs.some((k) => Math.hypot(k[0] - rgb[0], k[1] - rgb[1], k[2] - rgb[2]) < MIN_DISTANCE)) continue
    kept.push(c)
    rgbs.push(rgb)
  }
  return kept
}

/** The theme's colours for the series, then its text colour and font. */
function themeColours(el: HTMLElement) {
  const css = getComputedStyle(el)
  const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback
  const series = chartPalette(
    [v('--deck-accent', '#4f46e5'), v('--deck-accent-2', '#0ea5e9'), v('--deck-heading', '#111827'), v('--deck-muted', '#6b7280'), v('--deck-border', '#d1d5db')],
    v('--deck-bg', '#ffffff'),
  )
  return {
    series,
    text: v('--deck-fg', '#111827'),
    grid: v('--deck-border', '#e5e7eb'),
    font: v('--deck-font', 'sans-serif'),
  }
}

type Colours = ReturnType<typeof themeColours>

export function ChartView({ kind, body }: { kind: string; body: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [colours, setColours] = useState<Colours | undefined>()
  useLayoutEffect(() => {
    if (host.current) setColours(themeColours(host.current))
  }, [kind, body])
  const chart = parseChart(kind, body)
  if ('error' in chart) {
    return (
      <div className="deck-diagram" data-diagram="chart">
        <div className="deck-diagram-error">Chart not drawn: {chart.error}</div>
      </div>
    )
  }
  const rows = chart.categories.map((c, i) => Object.fromEntries([['category', c], ...chart.series.map((s, j) => [s, chart.values[i]![j]])]))
  const c = colours
  const colour = (i: number) => (c ? c.series[i % c.series.length]! : '#888')
  const axis = { stroke: c?.text, tick: { fill: c?.text, fontSize: 18 } }
  const legend = { wrapperStyle: { fontSize: 18, color: c?.text } }
  // What the editable PowerPoint rebuilds: the data and the colours drawn.
  const spec = JSON.stringify({ ...chart, colours: chart.series.map((_, i) => colour(i)), pieColours: chart.categories.map((_, i) => colour(i)), text: c?.text, font: c?.font })
  return (
    <div ref={host} className="deck-diagram deck-chart" data-diagram="chart" data-chart={spec} style={{ fontFamily: c?.font }}>
      {c && (
        <div className="deck-diagram-svg">
          <ResponsiveContainer width="100%" height="100%">
            {chart.kind === 'pie' ? (
              <PieChart>
                <Pie data={rows} dataKey={chart.series[0]!} nameKey="category" outerRadius="80%" isAnimationActive={false}
                  label={{ fill: c.text, fontSize: 18 }}>
                  {rows.map((_, i) => <Cell key={i} fill={colour(i)} />)}
                </Pie>
                <Legend {...legend} />
                <Tooltip />
              </PieChart>
            ) : chart.kind === 'line' ? (
              <LineChart data={rows} margin={{ top: 16, right: 24, bottom: 8, left: 8 }}>
                <CartesianGrid stroke={c.grid} strokeDasharray="3 3" />
                <XAxis dataKey="category" {...axis} />
                <YAxis {...axis} />
                {chart.series.map((s, i) => <Line key={s} dataKey={s} stroke={colour(i)} strokeWidth={3} dot={{ r: 5 }} isAnimationActive={false} />)}
                {chart.series.length > 1 && <Legend {...legend} />}
                <Tooltip />
              </LineChart>
            ) : (
              <BarChart data={rows} margin={{ top: 16, right: 24, bottom: 8, left: 8 }}>
                <CartesianGrid stroke={c.grid} strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="category" {...axis} />
                <YAxis {...axis} />
                {chart.series.map((s, i) => <Bar key={s} dataKey={s} fill={colour(i)} isAnimationActive={false} />)}
                {chart.series.length > 1 && <Legend {...legend} />}
                <Tooltip />
              </BarChart>
            )}
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
