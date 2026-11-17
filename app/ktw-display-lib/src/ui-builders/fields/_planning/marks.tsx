// SPDX-License-Identifier: Apache-2.0
// The planning marks — chips, the segmented progress bar, gate dots and the
// stepper — drawn in the planning palette (./palette.ts). Shared by the
// content renderers status, object-id, priority, stage, progress-segments,
// gate-dots and stepper; each mark is a plain inline element so it sits in a
// compact row, a card's line or a view alike. Their words read in the GUI's
// language through the library's `planning` namespace (./i18n), English when
// no translation answers; the data-* attributes keep the raw words.

import type { CSSProperties, ReactNode } from 'react'

import { refTarget, useHostServices } from '../../../ui-core/host/index.tsx'
import { useTexts } from '../../../ui-core/i18n/index.ts'
import {
  alertChip, color, gateColor, gateText, gateTitle, gateTone, priorityChip, segments, segmentsTitle,
  stageChip, statusChip, stepLabel, stepperStates, toneColors, wordLabel, type ChipLook, type PlanningT, type StepState,
} from './palette.ts'

export const MONO = 'var(--mantine-font-family-monospace)'

/** usePlanningT is the `planning` namespace's translator in the current
 *  language; a missing key (or no i18n set up) answers its English default. */
export function usePlanningT(): PlanningT {
  const texts = useTexts('planning')
  return (key, opts) => texts.t(key, opts)
}

const EMPTY = <span style={{ color: color('muted') }}>—</span>

function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
}

/** chipStyle is the mock-up's `.chip`: mono 11px, a pill, a 1px border. */
export function chipStyle(look: ChipLook): CSSProperties {
  return {
    display: 'inline-flex', alignItems: 'center', gap: 4,
    fontFamily: MONO, fontSize: 11, lineHeight: 1.5, padding: '1px 8px', borderRadius: 999,
    border: `1px ${look.dashed ? 'dashed' : 'solid'} ${look.border}`,
    color: look.fg, background: look.bg, whiteSpace: 'nowrap',
    textDecoration: look.struck ? 'line-through' : undefined,
    opacity: look.struck ? 0.75 : undefined,
  }
}

export function Chip({ look, children, title, testId, kind }: {
  look: ChipLook; children: ReactNode; title?: string; testId?: string; kind?: string
}) {
  return <span style={chipStyle(look)} title={title} data-testid={testId} data-ktw-mark={kind}>{children}</span>
}

export function StatusChip({ value, testId }: { value: unknown; testId?: string }) {
  const t = usePlanningT()
  if (isEmpty(value)) return EMPTY
  return <Chip look={statusChip(value)} testId={testId} kind="status">{wordLabel('status', value, t)}</Chip>
}

export function PriorityChip({ value, testId }: { value: unknown; testId?: string }) {
  if (isEmpty(value)) return EMPTY
  return <Chip look={priorityChip(value)} testId={testId} kind="priority">{String(value).toUpperCase()}</Chip>
}

export function StageChip({ value, testId }: { value: unknown; testId?: string }) {
  const t = usePlanningT()
  if (isEmpty(value)) return EMPTY
  return <Chip look={stageChip(value)} testId={testId} kind="stage">{wordLabel('stage', value, t)}</Chip>
}

/** IdChip is an object's id as a mono chip; a click opens the object through
 *  the host (and goes no further: a row it sits in is not selected). With no
 *  host the chip is plain. */
export function IdChip({ value, testId }: { value: unknown; testId?: string }) {
  const host = useHostServices()
  const t = usePlanningT()
  const target = refTarget(value)
  if (!target) return EMPTY
  const text = target.panel ? `${target.object} › ${target.panel}` : target.object
  const look: ChipLook = { fg: color('muted'), border: color('line'), bg: 'transparent' }
  if (!host.navigate) return <Chip look={look} testId={testId} kind="object-id">{text}</Chip>
  return (
    <button type="button" data-testid={testId} data-ktw-mark="object-id" title={t('open', { id: text, defaultValue: `Open ${text}` })}
      onClick={(e) => { e.stopPropagation(); host.navigate!(target) }}
      style={{ font: 'inherit', ...chipStyle(look), cursor: 'pointer' }}>
      {text}
    </button>
  )
}

/** IdText is an object's id as small mono dimmed text (an eyebrow, an id
 *  beside a title); a click opens the object through the host. */
export function IdText({ value, testId }: { value: unknown; testId?: string }) {
  const host = useHostServices()
  const t = usePlanningT()
  const target = refTarget(value)
  if (!target) return null
  const text = target.panel ? `${target.object} › ${target.panel}` : target.object
  const style: CSSProperties = { fontFamily: MONO, fontSize: 11, fontWeight: 400, color: color('muted') }
  if (!host.navigate) return <span style={style} data-testid={testId} data-ktw-mark="object-id">{text}</span>
  return (
    <span role="button" tabIndex={0} title={t('open', { id: text, defaultValue: `Open ${text}` })} data-testid={testId} data-ktw-mark="object-id"
      style={{ ...style, cursor: 'pointer' }}
      onClick={(e) => { e.stopPropagation(); host.navigate!(target) }}
      onKeyDown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); host.navigate!(target) } }}>
      {text}
    </span>
  )
}

/** SegmentBar is the mock-up's `.bar`: 9px, rounded, one segment per
 *  non-empty count (done, running, waiting, to do, blocked); its tooltip
 *  lists every count. */
export function SegmentBar({ value, testId }: { value: unknown; testId?: string }) {
  const t = usePlanningT()
  const segs = segments(value, t)
  return (
    <div title={segmentsTitle(value, t)} data-testid={testId} data-ktw-mark="progress-segments"
      style={{
        display: 'flex', height: 9, minWidth: 60, borderRadius: 999, overflow: 'hidden',
        background: color('pend-bg'), border: `1px solid ${color('line')}`,
      }}>
      {segs.map((s) => (
        <i key={s.tone} data-segment={s.tone} style={{ display: 'block', height: '100%', width: `${s.pct}%`, background: toneColors(s.tone).fg }} />
      ))}
    </div>
  )
}

/** GateDots is a 9px dot per gate: passed green, failed red, unknown grey;
 *  each dot's tooltip « name: why ». With `labels`, each gate reads « ● Label
 *  (reason) » inline. */
export function GateDots({ value, testId, labels }: { value: unknown; testId?: string; labels?: boolean }) {
  const t = usePlanningT()
  const gates = Array.isArray(value) ? value : []
  if (gates.length === 0) return EMPTY
  const dot = (g: unknown, i: number) => {
    const s = gateTone(g)
    return (
      <i key={i} title={gateTitle(g, t)} data-gate={s}
        style={{ display: 'inline-block', flex: '0 0 auto', width: 9, height: 9, borderRadius: '50%', background: gateColor(s) }} />
    )
  }
  if (labels) {
    // Each gate as « ● Label (reason) », the reasons of those not passed.
    return (
      <span data-testid={testId} data-ktw-mark="gate-dots" data-labels=""
        style={{ display: 'inline-flex', flexWrap: 'wrap', gap: '6px 14px', fontSize: 13, alignItems: 'center' }}>
        {gates.map((g, i) => (
          <span key={i} style={{ display: 'inline-flex', gap: 5, alignItems: 'center' }}>{dot(g, i)}{gateText(g, t)}</span>
        ))}
      </span>
    )
  }
  return (
    <span data-testid={testId} data-ktw-mark="gate-dots" style={{ display: 'inline-flex', gap: 3, alignItems: 'center' }}>
      {gates.map(dot)}
    </span>
  )
}

/** AlertChip is a value that waits on the person (« needs you »): a small
 *  red filled chip; an empty value draws nothing. */
export function AlertChip({ value, testId }: { value: unknown; testId?: string }) {
  if (value === undefined || value === null || String(value).trim() === '') return null
  return <Chip look={alertChip()} testId={testId} kind="alert">{String(value)}</Chip>
}

function stepStyle(state: StepState, first: boolean, last: boolean): CSSProperties {
  const base: CSSProperties = {
    fontSize: 12.5, lineHeight: 1.4, padding: '4px 10px', whiteSpace: 'nowrap',
    border: `1px solid ${color('line')}`, background: color('pend-bg'), color: color('muted'),
    borderRadius: first && last ? 999 : first ? '999px 0 0 999px' : last ? '0 999px 999px 0' : 0,
    marginLeft: first ? 0 : -1,
  }
  if (state === 'done') return { ...base, background: color('done-bg'), color: color('done'), borderColor: color('done') }
  if (state === 'current') return { ...base, background: color('accent'), color: color('surface'), borderColor: color('accent'), fontWeight: 600 }
  return base
}

/** Stepper is the mock-up's `.stepper`: joined pills, done green, the current
 *  one accent-filled, the rest grey; a single `dropped` step one grey pill. */
export function Stepper({ value, testId }: { value: unknown; testId?: string }) {
  const t = usePlanningT()
  const steps = stepperStates(value)
  if (steps.length === 0) return EMPTY
  return (
    <span data-testid={testId} data-ktw-mark="stepper" style={{ display: 'inline-flex', flexWrap: 'wrap', alignItems: 'center' }}>
      {steps.map((s, i) => (
        <span key={i} data-step={s.state} data-name={s.name} style={stepStyle(s.state, i === 0, i === steps.length - 1)}>{stepLabel(s.name, t)}</span>
      ))}
    </span>
  )
}
