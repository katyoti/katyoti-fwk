// SPDX-License-Identifier: Apache-2.0
// Compact rows and the planning field renderers: the palette
// is one, the marks (status, priority, stage, object-id, progress-segments,
// gate-dots, stepper) read it; a `slots` view places fields by path; a
// compact table draws dense rows that select, each with its menu. The
// planification delivery data types are rendered from their committed DSL.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import {
  color, gateColor, gateTitle, gateTone, PALETTE, priorityChip, segments, segmentsTitle,
  stageChip, statusChip, statusTone, stepperStates, toneColors,
} from '../src/ui-builders/fields/_planning/palette.ts'
import { buildUIViews } from '../src/ui-builders/view/index.ts'
import { buildUITables, compactGrid, compactRoles, rowClickKind } from '../src/ui-builders/table/index.ts'
import { metaLine, resolveSlotPath, slotEntries, slotFields, slotTone } from '../src/ui-builders/view/layouts/slots/slots.ts'
import { extraEmpty } from '../src/ui-builders/table/ui-table.compact.tsx'
import { SchemaQuery } from '../src/schema/index.ts'
import { selectAction, tableSelection } from '../src/ui-builders/page/nodes/schema-bound.tsx'
import { routePageAction } from '../src/ui-builders/page/page-context.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import {
  dataType, incrementRows, releaseCard, releaseRows, streamLane,
} from './fixtures/delivery-sample.ts'
import { alertChip, gateLabel, gateText } from '../src/ui-builders/fields/_planning/palette.ts'
import { splitMenu } from '../src/ui-builders/page/nodes/choice.tsx'

wireFieldRenderers()

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const page = (node: unknown, data?: unknown, onAction?: (e: unknown) => void) =>
  html(createElement(UIPageView, { node: node as never, data, onAction: onAction as never }))

describe('the palette', () => {
  test('is the mock-up’s hues, light and dark, overridable per token', () => {
    expect(PALETTE.done).toEqual(['#2f7d4a', '#6cc58b'])
    expect(PALETTE.run).toEqual(['#c45f00', '#f0a050'])
    expect(PALETTE.block).toEqual(['#b3261e', '#ef7d74'])
    expect(PALETTE.plan).toEqual(['#7b3fb8', '#b58ae8'])
    expect(color('wait')).toBe('var(--ktw-wait, light-dark(#9a7400, #e2c14a))')
  })

  test('maps every status word to its tone', () => {
    const tone = (w: string) => statusTone(w).tone
    for (const w of ['done', 'shipped']) expect(tone(w)).toBe('done')
    for (const w of ['running', 'delivering']) expect(tone(w)).toBe('run')
    for (const w of ['waiting', 'on-hold', 'idle']) expect(tone(w)).toBe('wait')
    for (const w of ['todo', 'defined', 'ready', 'split', 'whatever']) expect(tone(w)).toBe('pend')
    expect(tone('blocked')).toBe('block')
    expect(tone('planned')).toBe('plan')
    expect(statusTone('dropped')).toEqual({ tone: 'pend', struck: true })
  })

  test('is the same colour everywhere a tone is drawn', () => {
    expect(statusChip('done').fg).toBe(toneColors('done').fg)
    expect(gateColor('ok')).toBe(toneColors('done').fg)
    expect(gateColor('ko')).toBe(toneColors('block').fg)
    expect(priorityChip('P0').fg).toBe(toneColors('block').fg)
    expect(priorityChip('P1').fg).toBe(toneColors('run').fg)
    expect(priorityChip('P3').fg).toBe(color('muted'))
    expect(stageChip('candidate')).toMatchObject({ fg: color('run'), bg: color('run-bg') })
    expect(stageChip('released')).toMatchObject({ fg: color('done'), bg: color('done-bg') })
    expect(stageChip('frozen').fg).toBe(color('accent'))
    expect(stageChip('dropped').struck).toBe(true)
  })
})

describe('the marks', () => {
  test('progress segments: [done, running, waiting, todo, blocked], empty ones left out', () => {
    expect(segments([3, 1, 0, 0, 0])).toEqual([
      { tone: 'done', name: 'done', count: 3, pct: 75 },
      { tone: 'run', name: 'running', count: 1, pct: 25 },
    ])
    expect(segments([0, 0, 0, 0, 0])).toEqual([])
    expect(segments('x')).toEqual([])
    expect(segmentsTitle([3, 1, 1, 0, 2])).toBe('3 done · 1 running · 1 waiting · 0 to do · 2 blocked')
  })

  test('gate dots: passed green, failed red, absent grey; « label: why » on hover', () => {
    expect(gateTone({ name: 'e2e', passed: true })).toBe('ok')
    expect(gateTone({ name: 'e2e', passed: false })).toBe('ko')
    expect(gateTone({ name: 'change-log', why: 'needs a candidate' })).toBe('unknown')
    expect(gateTone({ name: 'e2e', passed: null })).toBe('unknown')
    expect(gateTitle({ name: 'change-log', why: 'needs a candidate' })).toBe('Change log: needs a candidate')
    expect(gateTitle({ name: 'e2e', passed: true })).toBe('End to end')
  })

  test('stepper: done | current | todo, running is current, a dropped step one grey pill', () => {
    expect(stepperStates([{ name: 'planned', state: 'done' }, { name: 'frozen', state: 'current' }, { name: 'released', state: 'todo' }]).map((s) => s.state))
      .toEqual(['done', 'current', 'todo'])
    expect(stepperStates([{ name: 'implement', state: 'running' }])[0].state).toBe('current')
    expect(stepperStates([{ name: 'dropped', state: 'current' }])).toEqual([{ name: 'dropped', state: 'off' }])
  })
})

describe('a slots view', () => {
  test('names its fields per slot; a dotted path reaches into a nested object', () => {
    const rows = dataType('release-rows')
    const header = (rows.types.ReleaseRows as any).extensions.find((e: any) => e.name === 'header')
    expect(slotFields(header.slots)).toEqual({
      eyebrow: ['eyebrow'], title: ['title'], titleId: [], badges: [], meta: ['due'], footer: ['stepper', 'gates'],
    })
    expect(resolveSlotPath(rows, 'release.gates')).toEqual({ parentType: 'ReleaseCard', name: 'gates', prefix: ['release'] })
    expect(resolveSlotPath(rows, 'gates')).toEqual({ parentType: 'ReleaseRows', name: 'gates', prefix: [] })
    expect(resolveSlotPath(rows, 'due')).toEqual({ parentType: 'ReleaseRows', name: 'due', prefix: [] })
    expect(metaLine(['31/10 · 5/8 done', '', undefined, 'rc1'])).toBe('31/10 · 5/8 done · rc1')
  })

  test('draws a release card: mono id, bold label with its stage chip, a dimmed meta line, gate dots', () => {
    const view = buildUIViews({ schema: dataType('release-card') }).card
    const out = html(createElement(() => view.renderer({ item: releaseCard }) as ReactElement))
    expect(out).toContain('data-ktw-view="slots"')
    expect(out).toContain('RL-033')
    expect(out).toContain('>1.006<')
    expect(out).toMatch(/data-ktw-mark="stage"[^>]*>candidate</)
    expect(out).toContain('31/10 · 5/8 done')
    // The planned count in the planned tone (violet), bare dots (no labels).
    expect(out).toMatch(/data-tone="plan"[^>]*>\+6 planned</)
    expect(out).toContain('rc1 · build 1.006-rc1')
    expect(out).not.toContain('data-labels=""')
    expect((out.match(/data-gate="ok"/g) ?? []).length).toBe(1)
    expect((out.match(/data-gate="ko"/g) ?? []).length).toBe(2)
    expect((out.match(/data-gate="unknown"/g) ?? []).length).toBe(2)
    expect(out).toContain('title="Change log: needs a candidate"')
  })

  test('draws a lane header with its mode select and its Cut now button, and the release header’s nested gates', () => {
    const lane = page({ kind: 'view', schema: dataType('stream-lane'), variant: 'lane-header', data: streamLane }, {}, () => {})
    expect(lane).toContain('KaTyoti')
    expect(lane).toContain('SM-001')
    expect(lane).toContain('semver · periodic · 14d')
    expect(lane).toContain('next: cut 2026-10-31')
    expect(lane).toContain('aria-label="Release mode"')
    expect(lane).toContain('value="periodic"')
    expect(lane).toContain('Cut now')

    const header = page({ kind: 'view', schema: dataType('release-rows'), variant: 'header', data: releaseRows }, {}, () => {})
    expect(header).toContain('KaTyoti 1.006 — A software factory driven from the PC')
    expect(header).toContain('due 2026-10-31 · 13 days left')
    expect(header).toMatch(/data-step="current"[^>]*>candidate</)
    expect((header.match(/data-gate=/g) ?? []).length).toBe(5)
    expect(header).toContain('Reschedule…')
    // The stage's one primary action, its label the row's.
    expect(header).toMatch(/data-primary="true"[^>]*>[\s\S]*?Sign rc1 off/)
    // At the header's top right; Unfreeze and Drop under its ⋯.
    expect(header).toContain('data-at="top"')
    expect(header).toContain('data-guide="page.menu"')
  })
})

describe('a compact table', () => {
  const schema = dataType('increment-row')
  const table = buildUITables({ schema }).default

  test('keeps its extension: density, no header, select, menu, widths', () => {
    expect(table.definition.density).toBe('compact')
    expect(table.definition.header).toBe(false)
    expect(table.definition.select).toBe('increment')
    expect(table.definition.menu?.map((m) => m.id)).toEqual(['change-release', 'take-out', 'hold', 'resume', 'priority'])
    expect(table.columns.map((c) => c.id)).toEqual(['priority', 'title', 'progress', 'waits'])
    expect(table.columns.map((c) => c.content)).toEqual(['priority', 'text', 'progress-segments', 'text'])
  })

  test('aligns its columns: widths fixed, the first free one the title, the rest secondary', () => {
    expect(compactRoles(table.columns)).toEqual(['fixed', 'title', 'fixed', 'secondary'])
    expect(compactGrid(table.columns, true)).toBe('52px minmax(min(16em,100%),2.4fr) 160px minmax(0,1.2fr) auto')
  })

  test('a row click selects it — the page’s select verb with the row’s id — before it opens', () => {
    expect(rowClickKind({ select: () => {}, open: () => {} })).toBe('select')
    expect(rowClickKind({ open: () => {} })).toBe('open')
    expect(rowClickKind({})).toBeUndefined()
    const ran: unknown[] = []
    const sel = tableSelection(table, (item) => ran.push(item), { increment: 'IN-036' })
    sel.onRowSelect!(incrementRows[0])
    expect(ran).toEqual([incrementRows[0]])
    expect(sel.isSelected!(incrementRows[1])).toBe(true)
    expect(sel.isSelected!(incrementRows[0])).toBe(false)
    // The verb the page runs sets the selection's key to the row's id.
    const set: Record<string, unknown> = {}
    routePageAction({ action: selectAction('increment'), item: incrementRows[0] }, {
      show: () => {}, select: (k, v) => { set[k] = v }, host: () => {}, selection: {},
    })
    expect(set).toEqual({ increment: 'IN-022' })
    expect(tableSelection({ ...table, definition: { type: 'ui-table' } }, undefined, {})).toEqual({})
  })

  test('draws dense rows — no header — the selected one marked, each with its menu', () => {
    const out = page({ kind: 'table', schema, data: incrementRows }, { selection: { increment: 'IN-036' } }, () => {})
    expect(out).toContain('data-ktw-table="compact"')
    expect(out).not.toContain('<thead')
    expect((out.match(/data-ktw-row=""/g) ?? []).length).toBe(4)
    expect((out.match(/data-row-selected="true"/g) ?? []).length).toBe(1)
    expect(out).toMatch(/data-ktw-mark="priority"[^>]*>P0</)
    expect(out).toMatch(/data-ktw-mark="status"[^>]*>on-hold</)
    expect(out).toContain('data-ktw-mark="progress-segments"')
    expect(out).toContain('title="3 done · 1 running · 1 waiting · 0 to do · 0 blocked"')
    expect(out).toContain('The direction follows each stream in benefits')
    expect((out.match(/data-guide="page.menu"/g) ?? []).length).toBe(4)
  })
})

describe('the mock-up’s rows and header', () => {
  test('an alert is a red filled chip; an empty one draws nothing', () => {
    expect(alertChip()).toEqual({ fg: color('surface'), border: color('block'), bg: color('block') })
  })

  test('gates read « Label (reason) », the reason only while not passed', () => {
    expect(gateLabel('increments')).toBe('Increments done')
    expect(gateLabel('alpha-sign-off')).toBe('Sign-off')
    expect(gateLabel('new-gate')).toBe('New gate')
    expect(gateText({ name: 'increments', passed: false, why: '3 open' })).toBe('Increments done (3 open)')
    expect(gateText({ name: 'e2e', why: 'needs a candidate' })).toBe('End to end (needs a candidate)')
    expect(gateText({ name: 'change-log', passed: true, why: 'stale' })).toBe('Change log')
  })

  test('a row menu’s button entries leave the ⋯', () => {
    const { buttons, rest } = splitMenu([{ id: 'a', label: 'A', button: true }, { id: 'b', label: 'B' }])
    expect(buttons.map((e) => e.id)).toEqual(['a'])
    expect(rest.map((e) => e.id)).toEqual(['b'])
    expect(splitMenu([{ id: 'a', label: 'A', button: true }]).rest).toEqual([])
  })

  test('a column draws its appended and prepended fields in its cell; Change release is a button', () => {
    const schema = dataType('increment-row')
    const table = buildUITables({ schema }).default
    expect(table.columns.find((c) => c.id === 'title')!.after!.map((e) => [e.id, e.content]))
      .toEqual([['id', 'object-id'], ['held_text', 'status'], ['planned_chip', 'status']])
    expect(table.columns.find((c) => c.id === 'waits')!.before!.map((e) => [e.id, e.content])).toEqual([['needs_you_text', 'alert']])
    const out = page({ kind: 'table', schema, data: incrementRows }, {}, () => {})
    // The title then its id chip, in one cell.
    expect(out).toMatch(/The direction follows each stream in benefits<\/span><span[^>]*><span[^>]*data-ktw-mark="object-id"[^>]*>IN-022</)
    // The held and planned chips only where they are set.
    expect((out.match(/data-ktw-mark="status"[^>]*>on-hold</g) ?? []).length).toBe(1)
    expect((out.match(/data-ktw-mark="status"[^>]*>planned</g) ?? []).length).toBe(1)
    expect((out.match(/data-ktw-mark="alert"[^>]*>needs you</g) ?? []).length).toBe(1)
    // An empty prepended or appended field draws nothing - no dash anywhere.
    expect(out).not.toContain('—')
    expect((out.match(/Change release ▾/g) ?? []).length).toBe(4)
    expect((out.match(/data-guide="page.menu"/g) ?? []).length).toBe(4)
    // A normal table draws them too, through its column renderer - and
    // leaves the empty ones out.
    const normal = buildUITables({ schema: { ...schema, types: { ...schema.types } } }).default
    const title = normal.columns.find((c) => c.id === 'title')!
    const cell = html(title.renderCell(incrementRows[0] as never, 0) as ReactElement)
    expect(cell).toContain('IN-022')
    expect(cell).not.toContain('data-ktw-mark="status"')
    const plain = html(title.renderCell(incrementRows[3] as never, 0) as ReactElement)
    expect(plain).not.toContain('—')
  })

  test('an empty prepended or appended field is one with nothing to show', () => {
    for (const v of [undefined, null, '', []]) expect(extraEmpty({ x: v }, 'x')).toBe(true)
    expect(extraEmpty({}, 'x')).toBe(true)
    expect(extraEmpty(undefined, 'x')).toBe(true)
    for (const v of ['planned', 0, false, [1]]) expect(extraEmpty({ x: v }, 'x')).toBe(false)
  })

  test('the release header draws its labelled gates from the data type’s long form alone', () => {
    const out = page({ kind: 'view', schema: dataType('release-rows'), variant: 'header', data: releaseRows }, {}, () => {})
    expect(out).toContain('data-labels=""')
    expect(out).toContain('Increments done (3 open)')
    expect(out).toContain('End to end (needs a candidate)')
    expect(out).toContain('Dependencies')
  })
})

describe('a content reference with inline params', () => {
  test('parseContentRef reads flags, numbers and text', () => {
    expect(SchemaQuery.parseContentRef('gate-dots')).toEqual({ name: 'gate-dots', params: {} })
    expect(SchemaQuery.parseContentRef('gate-dots(labels)')).toEqual({ name: 'gate-dots', params: { labels: true } })
    expect(SchemaQuery.parseContentRef('rating(max:10, half)')).toEqual({ name: 'rating', params: { max: 10, half: true } })
    expect(SchemaQuery.parseContentRef('x(a:b)')).toEqual({ name: 'x', params: { a: 'b' } })
  })

  test('a long-form `content: gate-dots(labels)` is the gate-dots content with labels', () => {
    const env = SchemaQuery.toContentEnvelope({ type: 'array', content: 'gate-dots(labels)' } as never, 'text')
    expect(env).toEqual({ type: 'gate-dots', labels: true } as never)
    expect(SchemaQuery.toContentEnvelope({ type: 'array', content: 'gate-dots' } as never, 'text')).toEqual({ type: 'gate-dots' } as never)
  })
})

describe('a slots entry with a tone', () => {
  test('reads `{field, tone}` beside plain paths; a status word names its tone', () => {
    expect(slotEntries({ meta: ['summary', { field: 'planned_text', tone: 'planned' }, { field: 'x', tone: 'block' }, { tone: 'plan' } as never] }).meta)
      .toEqual([{ path: 'summary' }, { path: 'planned_text', tone: 'plan' }, { path: 'x', tone: 'block' }])
    expect(slotFields({ badges: [{ field: 'stage', tone: 'run' }], footer: ['gates'] })).toMatchObject({ badges: ['stage'], footer: ['gates'] })
    expect(slotTone('planned')).toBe('plan')
    expect(slotTone('plan')).toBe('plan')
    expect(slotTone('accent')).toBe('accent')
    expect(slotTone(undefined)).toBeUndefined()
  })

  test('draws a toned meta entry in its tone’s colour, a plain one in the dimmed line', () => {
    const view = buildUIViews({ schema: dataType('release-card') }).card
    const out = html(createElement(() => view.renderer({ item: releaseCard }) as ReactElement))
    expect(out).toContain(`data-tone="plan" style="color:${color('plan')}">+6 planned<`)
    expect(out).not.toMatch(/data-tone="[^"]*"[^>]*>31\/10/)
    // Unset, it draws nothing.
    const bare = html(createElement(() => view.renderer({ item: { ...releaseCard, planned_text: '' } }) as ReactElement))
    expect(bare).not.toContain('data-tone=')
  })
})
