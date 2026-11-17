// SPDX-License-Identifier: Apache-2.0
// The page DSL's `progress` and `log` nodes (decision F6 of
// the display-panels study): a labelled bar, and the lines a piece of work
// writes as it goes. Both are pushed - a panel's ops patch them by their node
// id (a merge sets the bar, an append adds lines) -, never polled. The pure
// part; the renderers are nodes/progress.tsx and nodes/log.tsx.

import type { UIPageNodeBase } from './ui-page.types.ts'

/** A labelled bar: `current` of `total` - « 2 / 5 pages » under it, the
 *  unit naming what is counted; no total, a bar that only says it runs. */
export interface UIProgressNode extends UIPageNodeBase {
  kind: 'progress'
  label?: string
  current?: number
  total?: number
  /** What is counted (« pages », « programmes »), after « 2 / 5 ». */
  unit?: string
  /** running (the default, animated), done, failed. */
  state?: 'running' | 'done' | 'failed'
}

/** One line of a log: its text, or the text with its level and when. */
export type UILogLine = string | { text: string; level?: 'info' | 'warn' | 'error'; at?: string }

/** The lines a piece of work writes as it goes (`data`, appended by the
 *  panel's ops): the last `max` shown (default 200), the view following
 *  the newest, a line repeated in a row shown once with its count. */
export interface UILogNode extends UIPageNodeBase {
  kind: 'log'
  title?: string
  data?: UILogLine[]
  max?: number
  /** Shown while no line is written. */
  empty?: string
}

/** The share of the bar in percent: 0 with no total, 100 once done. */
export function progressPercent(n: Pick<UIProgressNode, 'current' | 'total' | 'state'>): number {
  if (n.state === 'done') return 100
  const total = n.total ?? 0
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round(((n.current ?? 0) / total) * 100)))
}

/** The count under the bar: « 2 / 5 pages », empty with no total. */
export function progressCount(n: Pick<UIProgressNode, 'current' | 'total' | 'unit'>): string {
  const total = n.total ?? 0
  if (total <= 0) return ''
  return `${n.current ?? 0} / ${total}${n.unit ? ` ${n.unit}` : ''}`
}

/** A log line drawn: its text, its level, when, and how many times in a
 *  row it was written. */
export interface LogRow { text: string; level: 'info' | 'warn' | 'error'; at?: string; count: number }

/** logRows coalesces the lines: a line written again right after itself is
 *  one row with its count; only the last `max` rows are kept, `hidden`
 *  saying how many earlier ones are left out. Pure. */
export function logRows(lines: readonly unknown[] | undefined, max = 200): { rows: LogRow[]; hidden: number } {
  const rows: LogRow[] = []
  for (const raw of lines ?? []) {
    const line = typeof raw === 'string' ? { text: raw } : raw && typeof raw === 'object' ? raw as { text?: unknown; level?: unknown; at?: unknown } : { text: String(raw ?? '') }
    const text = typeof line.text === 'string' ? line.text : String(line.text ?? '')
    const level = line.level === 'warn' || line.level === 'error' ? line.level : 'info'
    const at = typeof line.at === 'string' ? line.at : undefined
    const last = rows[rows.length - 1]
    if (last && last.text === text && last.level === level) { last.count++; if (at) last.at = at; continue }
    rows.push({ text, level, at, count: 1 })
  }
  const keep = Math.max(1, max)
  const hidden = Math.max(0, rows.length - keep)
  return { rows: hidden > 0 ? rows.slice(hidden) : rows, hidden }
}
