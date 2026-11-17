// SPDX-License-Identifier: Apache-2.0
// The planning palette — the Workflows view's status colours, defined once.
//
// Six tones (done, run, wait, pend, block, plan), each a foreground and a
// background, in light and dark; plus the few neutrals the planning marks
// draw with (accent, soft, muted, line). Every value is a CSS expression
// `var(--ktw-<token>, light-dark(<light>, <dark>))`: it follows Mantine's
// colour scheme (Mantine sets `color-scheme` from data-mantine-color-scheme,
// which `light-dark()` reads) and a host may override any token with its own
// custom property. The content renderers status, priority, stage,
// progress-segments, gate-dots, stepper and alert all read their colours here.
//
// Pure functions only (no React): bun tests them directly. The words they
// return are English unless given a `t` — the planning namespace's
// translator (./i18n/<lang>.json, read through useTexts('planning')).

/** PlanningT translates a key of the `planning` namespace; `defaultValue`
 *  (the English word) answers when the key is missing. UITexts.t fits. */
export type PlanningT = (key: string, opts: { defaultValue: string; [k: string]: unknown }) => string

/** A palette tone: done (green), run (orange), wait (yellow), pend (grey),
 *  block (red), plan (violet). */
export type Tone = 'done' | 'run' | 'wait' | 'pend' | 'block' | 'plan'

export const TONES: readonly Tone[] = ['done', 'run', 'wait', 'pend', 'block', 'plan']

/** [light, dark] per token — the delivery mock-up's hues. */
export const PALETTE: Record<string, readonly [string, string]> = {
  done: ['#2f7d4a', '#6cc58b'], 'done-bg': ['#e3f2e8', '#16301f'],
  run: ['#c45f00', '#f0a050'], 'run-bg': ['#fff0de', '#3a2611'],
  wait: ['#9a7400', '#e2c14a'], 'wait-bg': ['#fff8d6', '#2f2810'],
  pend: ['#a3aeb9', '#5d6b7a'], 'pend-bg': ['#f6f8fa', '#1c252f'],
  block: ['#b3261e', '#ef7d74'], 'block-bg': ['#fbe6e4', '#3a1714'],
  plan: ['#7b3fb8', '#b58ae8'], 'plan-bg': ['#f1e8fb', '#2a1d3a'],
  accent: ['#1d5fd1', '#6c9cf0'], soft: ['#eef3fb', '#1d2836'],
  muted: ['#5d6b7a', '#93a1b0'], line: ['#d5dce3', '#2c3845'],
  surface: ['#ffffff', '#18202a'],
  // The lane timeline's lane colours by index (the mock-up's streams first).
  'lane-1': ['#1d5fd1', '#6c9cf0'], 'lane-2': ['#0d7a6a', '#4fc7b3'],
  'lane-3': ['#7a5a00', '#e2c14a'], 'lane-4': ['#b4540f', '#f0a050'],
  'lane-5': ['#7b3fb8', '#b58ae8'], 'lane-6': ['#a0306e', '#e889b8'],
}

/** color is a token's CSS value: its custom property, else its light/dark pair. */
export function color(token: string): string {
  const pair = PALETTE[token]
  if (!pair) return `var(--ktw-${token})`
  return `var(--ktw-${token}, light-dark(${pair[0]}, ${pair[1]}))`
}

/** toneColors are a tone's foreground and background. */
export function toneColors(t: Tone): { fg: string; bg: string } {
  return { fg: color(t), bg: color(`${t}-bg`) }
}

/** A status word's look: its tone, and whether it is struck (dropped). */
export interface StatusLook { tone: Tone; struck?: boolean }

const STATUS_TONE: Record<string, Tone> = {
  done: 'done', shipped: 'done', released: 'done', passed: 'done',
  running: 'run', delivering: 'run', current: 'run', review: 'run',
  waiting: 'wait', 'on-hold': 'wait', idle: 'wait', held: 'wait',
  todo: 'pend', defined: 'pend', ready: 'pend', pending: 'pend', split: 'pend',
  blocked: 'block', failed: 'block',
  planned: 'plan', 'on-you': 'plan',
  dropped: 'pend',
}

/** statusTone maps a standing word (defined, running, delivering, on-hold,
 *  done, shipped, dropped), a plan state (idle, ready, blocked, split, done,
 *  dropped) or waiting / todo / planned to its look; an unknown word is grey. */
export function statusTone(word: unknown): StatusLook {
  const w = String(word ?? '').trim().toLowerCase()
  const tone = STATUS_TONE[w] ?? 'pend'
  return w === 'dropped' ? { tone, struck: true } : { tone }
}

/** A chip's colours: text, border, background (transparent when none). */
export interface ChipLook { fg: string; border: string; bg: string; struck?: boolean; dashed?: boolean }

/** statusChip is a status word's chip: its tone's colour on its tone's
 *  background; grey reads muted on the grey background. */
export function statusChip(word: unknown): ChipLook {
  const { tone, struck } = statusTone(word)
  const { fg, bg } = toneColors(tone)
  if (tone === 'pend') return { fg: color('muted'), border: color('pend'), bg, struck }
  return { fg, border: fg, bg, struck }
}

/** priorityChip: P0 red, P1 orange, P2 and P3 muted (no background). */
export function priorityChip(p: unknown): ChipLook {
  const v = String(p ?? '').trim().toUpperCase()
  if (v === 'P0') return { fg: color('block'), border: color('block'), bg: 'transparent' }
  if (v === 'P1') return { fg: color('run'), border: color('run'), bg: 'transparent' }
  return { fg: color('muted'), border: color('line'), bg: 'transparent' }
}

/** stageChip: planned muted, frozen accent, candidate orange on its
 *  background, canary orange dashed, released green on its background,
 *  dropped muted and struck. */
export function stageChip(stage: unknown): ChipLook {
  const v = String(stage ?? '').trim().toLowerCase()
  switch (v) {
    case 'frozen': return { fg: color('accent'), border: color('accent'), bg: 'transparent' }
    case 'candidate': return { fg: color('run'), border: color('run'), bg: color('run-bg') }
    case 'canary': return { fg: color('run'), border: color('run'), bg: color('run-bg'), dashed: true }
    case 'released': return { fg: color('done'), border: color('done'), bg: color('done-bg') }
    case 'dropped': return { fg: color('muted'), border: color('line'), bg: 'transparent', struck: true }
    default: return { fg: color('muted'), border: color('line'), bg: 'transparent' }
  }
}

/** The order of a progress bar's numbers and their tones. */
export const SEGMENT_TONES: readonly Tone[] = ['done', 'run', 'wait', 'pend', 'block']
export const SEGMENT_NAMES = ['done', 'running', 'waiting', 'to do', 'blocked'] as const
/** The segments' keys in the planning namespace (`segment.<key>`). */
export const SEGMENT_KEYS = ['done', 'running', 'waiting', 'todo', 'blocked'] as const

/** segmentNames are the segments' names, in t's language (English without t). */
export function segmentNames(t?: PlanningT): string[] {
  return SEGMENT_NAMES.map((n, i) => (t ? t(`segment.${SEGMENT_KEYS[i]}`, { defaultValue: n }) : n))
}

/** A segment of a progress bar: its tone, its count, its share in percent. */
export interface Segment { tone: Tone; name: string; count: number; pct: number }

/** segments reads [done, running, waiting, todo, blocked] as the bar's
 *  non-empty segments, each with its share of the total; anything not a
 *  non-negative number counts 0. */
export function segments(values: unknown, t?: PlanningT): Segment[] {
  const nums = Array.isArray(values) ? values : []
  const counts = SEGMENT_TONES.map((_, i) => {
    const n = Number(nums[i])
    return Number.isFinite(n) && n > 0 ? n : 0
  })
  const total = counts.reduce((a, b) => a + b, 0)
  if (total === 0) return []
  const names = segmentNames(t)
  return counts.flatMap((count, i) => (count > 0
    ? [{ tone: SEGMENT_TONES[i], name: names[i], count, pct: (100 * count) / total }]
    : []))
}

/** segmentsTitle is the bar's tooltip: every count by name. */
export function segmentsTitle(values: unknown, t?: PlanningT): string {
  const nums = Array.isArray(values) ? values : []
  return segmentNames(t).map((n, i) => `${Number(nums[i]) > 0 ? Number(nums[i]) : 0} ${n}`).join(' · ')
}

/** A gate dot: ok (passed true, green), ko (passed false, red), unknown
 *  (passed absent or null, grey). */
export type GateState = 'ok' | 'ko' | 'unknown'

export function gateTone(g: unknown): GateState {
  const passed = g && typeof g === 'object' ? (g as { passed?: unknown }).passed : undefined
  if (passed === true) return 'ok'
  if (passed === false) return 'ko'
  return 'unknown'
}

export function gateColor(s: GateState): string {
  return s === 'ok' ? color('done') : s === 'ko' ? color('block') : color('pend')
}

/** gateTitle is a gate's tooltip: « label: why », the label alone without
 *  why — the label in t's language, as gateLabel reads it. */
export function gateTitle(g: unknown, t?: PlanningT): string {
  if (!g || typeof g !== 'object') return ''
  const { name, why } = g as { name?: unknown; why?: unknown }
  const n = name === undefined || name === null ? '' : gateLabel(name, t)
  const w = why === undefined || why === null ? '' : String(why)
  return w ? (n ? `${n}: ${w}` : w) : n
}

/** A stepper pill's state: done (green), current (accent, filled), todo
 *  (grey), off (a single grey pill — a dropped release's one step). */
export type StepState = 'done' | 'current' | 'todo' | 'off'

/** stepperStates reads [{name, state}] as pills: state done | current | todo,
 *  or done | running | todo (running is current); a step named `dropped`
 *  is drawn off (grey) whatever its state; an unknown state is todo. */
export function stepperStates(steps: unknown): { name: string; state: StepState }[] {
  if (!Array.isArray(steps)) return []
  return steps.flatMap((s) => {
    if (typeof s === 'string') return [{ name: s, state: 'todo' as StepState }]
    if (!s || typeof s !== 'object') return []
    const r = s as { name?: unknown; state?: unknown }
    const name = r.name === undefined || r.name === null ? '' : String(r.name)
    const st = String(r.state ?? '').toLowerCase()
    let state: StepState = st === 'done' ? 'done' : st === 'current' || st === 'running' ? 'current' : 'todo'
    if (name.toLowerCase() === 'dropped') state = 'off'
    return [{ name, state }]
  })
}

/** alertChip is the mock-up's `.needs`: the surface colour on the block red,
 *  filled — something that waits on the person. */
export function alertChip(): ChipLook {
  return { fg: color('surface'), border: color('block'), bg: color('block') }
}

/** How a gate reads beside its dot, by its name; another name reads as
 *  itself, its first letter capitalised and its dashes spaces. */
export const GATE_LABELS: Record<string, string> = {
  increments: 'Increments done',
  dependencies: 'Dependencies',
  'no-open-p0-fix': 'No open P0 fix',
  e2e: 'End to end',
  'alpha-sign-off': 'Sign-off',
  'change-log': 'Change log',
}

export function gateLabel(name: unknown, t?: PlanningT): string {
  const n = name === undefined || name === null ? '' : String(name)
  if (GATE_LABELS[n]) return t ? t(`gate.${n}`, { defaultValue: GATE_LABELS[n] }) : GATE_LABELS[n]
  const s = n.replace(/[-_]+/g, ' ').trim()
  return s ? s[0].toUpperCase() + s.slice(1) : ''
}

/** gateText is a gate as a line beside its dot: its label, and its reason in
 *  brackets while it does not pass (« Increments done (3 open) »). */
export function gateText(g: unknown, t?: PlanningT): string {
  if (!g || typeof g !== 'object') return ''
  const { name, why } = g as { name?: unknown; why?: unknown }
  const label = gateLabel(name, t)
  const w = why === undefined || why === null ? '' : String(why).trim()
  return gateTone(g) !== 'ok' && w ? `${label} (${w})` : label
}

/** keepCapital gives the result the original word's capital: a word that
 *  starts with an uppercase letter (« Running ») reads capitalised in every
 *  language (« Running », « En cours »). */
function keepCapital(original: string, result: string): string {
  const o = original.trim()
  if (!o || o[0] === o[0].toLowerCase() || !result) return result
  return result[0].toUpperCase() + result.slice(1)
}

/** wordLabel is a status or stage word as it reads, in t's language: the
 *  word itself without t or without a translation (`status.<w>`, `stage.<w>`);
 *  a capitalised word keeps its capital. */
export function wordLabel(kind: 'status' | 'stage', word: unknown, t?: PlanningT): string {
  const w = String(word ?? '')
  if (!t) return w
  const key = w.trim().toLowerCase()
  return key ? keepCapital(w, t(`${kind}.${key}`, { defaultValue: w })) : w
}

/** The release stages a stepper names (`stage.<name>`); any other step name
 *  is a request step (`step.<name>`). */
const STAGE_NAMES = new Set(['planned', 'frozen', 'candidate', 'canary', 'released', 'dropped'])

/** stepLabel is a stepper pill's word in t's language: a release stage reads
 *  `stage.<name>`, a request step `step.<name>`; the name itself without t or
 *  without a translation; a capitalised name keeps its capital. */
export function stepLabel(name: unknown, t?: PlanningT): string {
  const w = String(name ?? '')
  if (!t) return w
  const key = w.trim().toLowerCase()
  if (!key) return w
  return keepCapital(w, t(`${STAGE_NAMES.has(key) ? 'stage' : 'step'}.${key}`, { defaultValue: w }))
}
