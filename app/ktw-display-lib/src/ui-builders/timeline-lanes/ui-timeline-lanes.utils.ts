// SPDX-License-Identifier: Apache-2.0
// The lane timeline's pure helpers: dates on the axis, the
// Monday ticks, the axis' default range, the rows that keep colliding cards
// apart, a lane's colour, and the item type a lane's items field holds.
// No React: bun tests them directly.

import type { ArrayType, ObjectType, Schema, SchemaType } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import { color } from '../fields/_planning/palette.ts'
import type { UITimelineLanesDef } from './ui-timeline-lanes.schema.ts'

const DAY = 86_400_000

/** parseDay reads a date as the UTC midnight of its day, in ms: a
 *  `YYYY-MM-DD` as that day, any other text through `Date`; undefined when
 *  it is no date. */
export function parseDay(v: unknown): number | undefined {
  if (v instanceof Date) return Number.isNaN(v.getTime()) ? undefined : Date.UTC(v.getFullYear(), v.getMonth(), v.getDate())
  if (typeof v !== 'string' || v.trim() === '') return undefined
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(v.trim())
  if (m) return Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? undefined : Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())
}

/** todayOf is the day the today line marks: `true` the current date, a date
 *  string that day, anything else none. */
export function todayOf(today: boolean | string | undefined, now: Date = new Date()): number | undefined {
  if (today === true) return parseDay(now)
  if (typeof today === 'string') return parseDay(today)
  return undefined
}

/** dayLabel is a day as the axis writes it: dd/mm. */
export function dayLabel(day: number): string {
  const d = new Date(day)
  return `${String(d.getUTCDate()).padStart(2, '0')}/${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

/** A lane's items, as the def's `items` field holds them. */
export function laneItems(lane: unknown, def: Pick<UITimelineLanesDef, 'items'>): Record<string, unknown>[] {
  const v = lane && typeof lane === 'object' ? (lane as Record<string, unknown>)[def.items] : undefined
  return Array.isArray(v) ? v.filter((x): x is Record<string, unknown> => !!x && typeof x === 'object') : []
}

/** The days the axis keeps before and after today. */
const TODAY_MARGIN = 3 * DAY

/** An axis: its first and last day (ms). */
export interface AxisRange { from: number; to: number }

/** axisRange is the axis' span: `from`/`to` when the def sets them, else the
 *  earliest item date minus 7 days (three days before today, when earlier
 *  still) and the latest plus 7 days (three days after today, when later
 *  still) - the today line never sits on the lane's edge; two weeks around
 *  today with no item date at all. */
export function axisRange(lanes: unknown[], def: Pick<UITimelineLanesDef, 'items' | 'itemDate' | 'from' | 'to'>, today?: number): AxisRange {
  const days: number[] = []
  for (const lane of lanes) {
    for (const it of laneItems(lane, def)) {
      const d = parseDay(it[def.itemDate])
      if (d !== undefined) days.push(d)
    }
  }
  const pivot = today ?? parseDay(new Date())!
  let from = days.length ? Math.min(...days) - 7 * DAY : pivot - 7 * DAY
  let to = days.length ? Math.max(...days) + 7 * DAY : pivot + 7 * DAY
  if (today !== undefined) { from = Math.min(from, today - TODAY_MARGIN); to = Math.max(to, today + TODAY_MARGIN) }
  from = parseDay(def.from) ?? from
  to = parseDay(def.to) ?? to
  if (to <= from) to = from + DAY
  return { from, to }
}

/** datePct is a day's place on the axis, in percent (unclamped). */
export function datePct(day: number, range: AxisRange): number {
  return ((day - range.from) / (range.to - range.from)) * 100
}

/** The share of the track a card's centre keeps to (the mock-up's clamp). */
export const CARD_MIN_PCT = 10
export const CARD_MAX_PCT = 92

/** cardPct is where a card is centred: its date's percent kept within
 *  10%..92% so a card never leaves its track. */
export function cardPct(day: number, range: AxisRange): number {
  return Math.min(CARD_MAX_PCT, Math.max(CARD_MIN_PCT, datePct(day, range)))
}

/** A tick of the axis: its day, its label (dd/mm) and its place. */
export interface Tick { day: number; label: string; pct: number }

/** The most ticks an axis draws: their dd/mm labels stay apart on the
 *  nominal track. */
export const MAX_TICKS = 10

/** mondayTicks is a tick every Monday within the range — every second,
 *  third… Monday when more than `max` would crowd the axis — but on its very
 *  edges (within `edge` percent), where its label would be cut in half. */
export function mondayTicks(range: AxisRange, edge = 2, max = MAX_TICKS): Tick[] {
  const out: Tick[] = []
  const first = new Date(range.from)
  const shift = (8 - first.getUTCDay()) % 7 // days to the next Monday (0 when it is one)
  const weeks = Math.floor((range.to - (range.from + shift * DAY)) / (7 * DAY)) + 1
  const step = Math.max(1, Math.ceil(weeks / Math.max(1, max)))
  for (let d = range.from + shift * DAY; d <= range.to; d += 7 * step * DAY) {
    const pct = datePct(d, range)
    if (pct >= edge && pct <= 100 - edge) out.push({ day: d, label: dayLabel(d), pct })
  }
  return out
}

/** The nominal card and track the stacking reckons with: a card is about
 *  160px wide on a track at least 726px wide (the 980px timeline less its
 *  230px header column and padding) — so two centres closer than ~22% of
 *  the track collide. */
export const NOMINAL_CARD_PX = 160
export const NOMINAL_TRACK_PX = 726
export const MIN_GAP_PCT = (100 * NOMINAL_CARD_PX) / NOMINAL_TRACK_PX

/** stackRows gives each card a row so no two cards of a row are closer than
 *  minGap percent: by place, each takes the first row whose last card it
 *  clears. `blockedTo` (percent) is the first row's start a chip holds (the
 *  backlog): a card reaching into it goes to a later row. The answer is the
 *  row of each card, in the given order. */
export function stackRows(pcts: number[], minGap: number = MIN_GAP_PCT, blockedTo = 0): number[] {
  const order = pcts.map((p, i) => ({ p, i })).sort((a, b) => a.p - b.p || a.i - b.i)
  // The chip as a card already placed in the first row, its right edge at
  // blockedTo: a card clears it once its left edge does.
  const last: number[] = blockedTo > 0 ? [blockedTo - minGap / 2] : []
  const rows = new Array<number>(pcts.length).fill(0)
  for (const { p, i } of order) {
    let r = last.findIndex((q) => p - q >= minGap)
    if (r < 0) { r = last.length; last.push(p) } else last[r] = p
    rows[i] = r
  }
  return rows
}

/** The card geometry of a track: the first row's top, a card's nominal
 *  height, the gap between rows (the mock-up's top 10px, then card + 8px). */
export const CARD_TOP_PX = 10
export const CARD_HEIGHT_PX = 92
export const ROW_GAP_PX = 8

/** rowTop is a row's top in its track. */
export function rowTop(row: number): number {
  return CARD_TOP_PX + row * (CARD_HEIGHT_PX + ROW_GAP_PX)
}

/** trackHeight is the least height a track holding `rows` rows needs. */
export function trackHeight(rows: number): number {
  return rows <= 0 ? 74 : Math.max(74, rowTop(rows - 1) + CARD_HEIGHT_PX + CARD_TOP_PX)
}

/** The lane colours by index (the mock-up's streams, then more hues). */
export const LANE_TOKENS = ['lane-1', 'lane-2', 'lane-3', 'lane-4', 'lane-5', 'lane-6'] as const

/** laneColor is a lane's colour: its colour field (a palette token or a CSS
 *  colour), else the palette's lane colour by its index. */
export function laneColor(lane: unknown, index: number, colorField = 'color'): string {
  const v = lane && typeof lane === 'object' ? (lane as Record<string, unknown>)[colorField] : undefined
  if (typeof v === 'string' && v.trim() !== '') {
    const s = v.trim()
    return /^[a-z][a-z0-9-]*$/.test(s) && !CSS_NAMED.has(s) ? color(s) : s
  }
  return color(LANE_TOKENS[((index % LANE_TOKENS.length) + LANE_TOKENS.length) % LANE_TOKENS.length])
}

// A few CSS colour names a lane may set as itself rather than as a token.
const CSS_NAMED = new Set(['red', 'green', 'blue', 'orange', 'purple', 'teal', 'gray', 'grey', 'black', 'white', 'yellow', 'pink', 'brown', 'transparent'])

/** elementSchema is the schema rooted at the element type of a field of the
 *  root type (an array of a named type, or of an inline object, which takes
 *  a synthetic name) — what an item's ui-view variant is looked up on.
 *  undefined when the field is no array of objects. */
export function elementSchema(schema: Schema, field: string): Schema | undefined {
  let root: SchemaType
  try { root = SchemaQuery.getType(schema) } catch { return undefined }
  const f = (root as ObjectType).fields?.[field]
  if (f === undefined) return undefined
  let t: SchemaType
  if (typeof f === 'string' && schema.types?.[f] === undefined) return undefined
  try { t = SchemaQuery.getType(schema, f) } catch { return undefined }
  if (t.type !== 'array') return undefined
  const items = (t as ArrayType).items
  if (typeof items === 'string') return schema.types?.[items] !== undefined ? SchemaQuery.getSchema(schema, items) : undefined
  if (items && typeof items === 'object' && '$ref' in items) {
    const ref = (items as { $ref: string }).$ref
    return schema.types?.[ref] !== undefined ? SchemaQuery.getSchema(schema, ref) : undefined
  }
  if (items && typeof items === 'object' && (items as SchemaType).type === 'object') {
    const name = `${String(schema.root)}.${field}[]`
    return { name: schema.name, root: name, types: { ...schema.types, [name]: items as SchemaType } }
  }
  return undefined
}

/** backlogLabel is the backlog chip's text: « Backlog <n>: » and its first
 *  three ids, « … » when there are more. */
/** backlogPct is how far the backlog chip reaches into its track, in
 *  percent of the nominal track (its text's width reckoned in the mono 12px
 *  font, its padding and left offset): the first row's start it holds. */
export function backlogPct(label: string): number {
  return (100 * (6 + 18 + label.length * 7.3)) / NOMINAL_TRACK_PX
}

export function backlogLabel(ids: string[], shown = 3): string {
  return `Backlog ${ids.length}: ${ids.slice(0, shown).join(', ')}${ids.length > shown ? '…' : ''}`
}
