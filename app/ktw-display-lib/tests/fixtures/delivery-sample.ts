// SPDX-License-Identifier: Apache-2.0
// The planification delivery data types (stream-lane, release-card,
// release-rows, increment-row) read from their committed DSL, and sample
// values shaped like them — the fixture of the planning marks, the slots
// view and the compact table.

import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { parse } from 'yaml'

import type { Schema } from '../../src/schema/index.ts'

const DIR = join(import.meta.dir, '..', '..', '..', '..', 'internal', 'extensions', 'planification', 'contrib', 'embed', 'cwm', 'data-types')

/** dataType reads a committed planification data type as a schema AST. */
export function dataType(name: string): Schema {
  const doc = parse(readFileSync(join(DIR, `${name}.dsl.yaml`), 'utf8')) as Record<string, unknown>
  delete doc.$schema
  return doc as unknown as Schema
}

export const gates = [
  { name: 'increments', passed: false, why: '3 open' },
  { name: 'dependencies', passed: true },
  { name: 'no-open-p0-fix', passed: false, why: 'IN-031 holds an open P0 fix' },
  { name: 'e2e', why: 'needs a candidate' },
  { name: 'change-log', why: 'needs a candidate' },
]

export const releaseCard = {
  id: 'RL-033', label: '1.006', version: '1.006', stage: 'candidate', stream: 'SM-001',
  summary: '31/10 · 5/8 done', planned_text: '+6 planned', candidate_text: 'rc1 · build 1.006-rc1',
  done: 5, total: 8, planned: 6, gates,
}

export const streamLane = {
  id: 'SM-001', name: 'KaTyoti', mode: 'periodic', scheme: 'semver',
  scheme_mode: 'semver · periodic · 14d', mode_text: 'periodic · next cut 2026-10-31',
  next_text: 'next: cut 2026-10-31', releases: [releaseCard], backlog: [],
}

export const incrementRows = [
  { id: 'IN-022', priority: 'P0', title: 'The direction follows each stream in benefits', standing: 'running', progress: [3, 1, 1, 0, 0], waits: 'awaits your review', needs_you: 1, needs_you_text: 'needs you',
    options: [{ value: 'RL-033', label: '1.006 — ships in' }, { value: 'RL-034', label: '1.007 — planned for' }] },
  { id: 'IN-036', priority: 'P1', title: 'Every project and module shows what it is', standing: 'on-hold', held: 'waits for ArchiFactory', held_text: 'on-hold', progress: [0, 0, 2, 3, 1], waits: 'waits for ArchiFactory 2026.10' },
  { id: 'IN-040', priority: 'P2', title: 'A dossier d’architecture reads its sources', standing: 'defined', progress: [0, 0, 0, 4, 0], planned_for: 'RL-034', planned_chip: 'planned' },
  { id: 'IN-041', priority: 'P3', title: 'A plain row with nothing appended', standing: 'defined', progress: [0, 0, 0, 1, 0], waits: '' },
]

export const releaseRows = {
  eyebrow: 'KaTyoti · RL-033 · the stream’s next release',
  title: 'KaTyoti 1.006 — A software factory driven from the PC',
  due: 'due 2026-10-31 · 13 days left',
  gates,
  next_action: 'Sign off the alpha',
  next_step: 'signoff',
  next_label: 'Sign rc1 off',
  can_reschedule: true,
  can_unfreeze: true,
  can_drop: true,
  release: releaseCard,
  stepper: [{ name: 'planned', state: 'done' }, { name: 'frozen', state: 'done' }, { name: 'candidate', state: 'current' }, { name: 'released', state: 'todo' }],
  ships: incrementRows,
  planned: [],
}

// ── The lane timeline: the mock-up's four streams ──────────

const gatesOf = (inc: boolean | null, dep: boolean | null, p0: boolean | null, e2e: boolean | null, sign: boolean | null) =>
  ([['increments', inc], ['dependencies', dep], ['no-open-p0-fix', p0], ['e2e', e2e], ['alpha-sign-off', sign]] as const)
    .map(([name, passed]) => (passed === null ? { name, why: 'needs a candidate' } : { name, passed }))

const rel = (id: string, label: string, date: string, stage: string, summary: string, extra: Record<string, unknown> = {}) => ({
  id, label, version: label, date, stage, summary, done: 0, total: 0, ...extra,
})

const backlogOf = (ids: string[]) => ids.map((id, i) => ({ id, priority: i < 2 ? 'P1' : 'P2', title: `Increment ${id}` }))

/** The mock-up's timeline: four streams, their releases by date, their backlogs. */
export const deliveryLanes = [
  { id: 'SM-001', name: 'KaTyoti', mode: 'periodic', scheme: 'semver', scheme_mode: 'semver · periodic · 14 d',
    mode_text: 'periodic · next cut 2026-10-31', next_text: 'next: cut 31/10',
    releases: [
      rel('RL-033', '1.006', '2026-10-31', 'planned', '31/10 · 5/8 done', { gates: gatesOf(false, true, true, null, null), next: true }),
      rel('RL-034', '1.007', '2026-11-15', 'planned', '15/11 · 0/0 done', { planned_text: '+6 planned' }),
      rel('RL-035', '1.008', '2026-11-30', 'planned', '30/11 · 0/0 done', { planned_text: '+1 planned' }),
    ],
    backlog: backlogOf(['IN-008', 'IN-009', 'IN-011', 'IN-012', 'IN-013', 'IN-014', 'IN-015', 'IN-019', 'IN-023', 'IN-027', 'IN-028']) },
  { id: 'SM-002', name: 'ArchiFactory', mode: 'manual', scheme: 'calendar', scheme_mode: 'calendar · manual',
    mode_text: 'manual', next_text: 'next: by hand',
    releases: [rel('RL-014', '2026.10', '2026-10-31', 'planned', '31/10 · 5/7 done', { gates: gatesOf(false, true, true, null, null), next: true })],
    backlog: backlogOf(['IN-001', 'IN-002', 'IN-003', 'IN-004', 'IN-005', 'IN-006', 'IN-007', 'IN-010']) },
  { id: 'SM-003', name: 'Docs site', mode: 'on-change', scheme: 'date', scheme_mode: 'date · on-change · at most daily',
    mode_text: 'on-change', next_text: 'next: when an increment is done',
    releases: [rel('RL-D1', '2026-10-21', '2026-10-21', 'candidate', '21/10 · 0/0 done',
      { candidate: 'rc1', candidate_text: 'rc1', gates: gatesOf(true, true, true, true, null), next: true })],
    backlog: [] },
  { id: 'SM-006', name: 'Promotion', mode: 'on-change', scheme: 'date', scheme_mode: 'date · on-change · at most daily',
    mode_text: 'on-change', next_text: 'next: when an increment is done',
    releases: [],
    backlog: backlogOf(['IN-001p', 'IN-002p']) },
]

/** The stream-lane data type with its timeline's today fixed and its axis
 *  labelled as the mock-up's (« Stream · release mode »). */
export function timelineSchema(today = '2026-10-18'): Schema {
  const s = dataType('stream-lane') as any
  const ext = s.types.StreamLane.extensions.find((e: any) => e.type === 'ui-timeline-lanes')
  ext.today = today
  ext.axisLabel = 'Stream · release mode'
  return s as Schema
}
