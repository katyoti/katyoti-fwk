// SPDX-License-Identifier: Apache-2.0
// The lane timeline: its pure helpers — a date's place,
// clamped for a card; a tick every Monday; the axis' default range; rows that
// keep colliding cards apart; the item type's view found through a named
// type — the select a card's click runs, and a static render of lanes with
// their headers, cards, backlog chip and today line.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import {
  axisRange, backlogLabel, buildUITimelineLanes, cardPct, datePct, elementSchema, laneColor, mondayTicks,
  backlogPct, parseDay, rowTop, stackRows, todayOf, toUITimelineLanesDef, trackHeight,
} from '../src/ui-builders/timeline-lanes/index.ts'
import { selectAction, tableSelection } from '../src/ui-builders/page/nodes/schema-bound.tsx'
import { routePageAction } from '../src/ui-builders/page/page-context.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { SCHEMA_BOUND_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { color } from '../src/ui-builders/fields/_planning/palette.ts'
import { dataType, deliveryLanes, timelineSchema } from './fixtures/delivery-sample.ts'

wireFieldRenderers()

const D = (s: string) => parseDay(s)!
const range = { from: D('2026-10-14'), to: D('2026-12-06') }

describe('dates on the axis', () => {
  test('a day is its UTC midnight; no date is none', () => {
    expect(parseDay('2026-10-18')).toBe(Date.UTC(2026, 9, 18))
    expect(parseDay('')).toBeUndefined()
    expect(parseDay('soon')).toBeUndefined()
    expect(todayOf('2026-10-18')).toBe(Date.UTC(2026, 9, 18))
    expect(todayOf(true, new Date(2026, 9, 18, 15))).toBe(Date.UTC(2026, 9, 18))
    expect(todayOf(false)).toBeUndefined()
  })

  test('a date is its share of the range; a card keeps within 10%..92%', () => {
    expect(datePct(range.from, range)).toBe(0)
    expect(datePct(range.to, range)).toBe(100)
    expect(datePct(D('2026-10-31'), range)).toBeCloseTo((17 / 53) * 100, 6)
    expect(cardPct(D('2026-10-14'), range)).toBe(10)
    expect(cardPct(D('2026-12-06'), range)).toBe(92)
    expect(cardPct(D('2026-11-09'), range)).toBeCloseTo((26 / 53) * 100, 6)
  })

  test('a tick every Monday, written dd/mm', () => {
    const ticks = mondayTicks(range)
    expect(ticks.map((t) => t.label)).toEqual(['19/10', '26/10', '02/11', '09/11', '16/11', '23/11', '30/11'])
    expect(ticks[0].pct).toBeCloseTo((5 / 53) * 100, 6)
    // A Monday on the range's very edge is not ticked (its label would be cut).
    expect(mondayTicks({ from: D('2026-10-19'), to: D('2026-11-09') }).map((t) => t.label)).toEqual(['26/10', '02/11'])
    expect(mondayTicks({ from: D('2026-10-19'), to: D('2026-11-09') }, 0).map((t) => t.label)).toEqual(['19/10', '26/10', '02/11', '09/11'])
    // Four months: every second Monday, never more than ten labels.
    const long = mondayTicks({ from: D('2026-10-19'), to: D('2027-02-22') }, 0)
    expect(long.length).toBeLessThanOrEqual(10)
    expect(long.slice(0, 3).map((t) => t.label)).toEqual(['19/10', '02/11', '16/11'])
  })

  test('the range defaults to the item dates less and plus 7 days, widened around today', () => {
    const def = { items: 'releases', itemDate: 'date' }
    const r = axisRange(deliveryLanes, def, D('2026-10-18'))
    expect(r).toEqual({ from: D('2026-10-14'), to: D('2026-12-07') })
    // Today before the start widens it to three days before today, never
    // leaving the today line on the lane's edge.
    expect(axisRange(deliveryLanes, def, D('2026-10-01')).from).toBe(D('2026-09-28'))
    expect(axisRange(deliveryLanes, def, D('2027-01-10')).to).toBe(D('2027-01-13'))
    expect(axisRange([], def, D('2026-10-18'))).toEqual({ from: D('2026-10-11'), to: D('2026-10-25') })
    // from / to set win.
    expect(axisRange(deliveryLanes, { ...def, from: '2026-10-01', to: '2026-12-31' })).toEqual({ from: D('2026-10-01'), to: D('2026-12-31') })
  })
})

describe('cards that collide', () => {
  test('stack in rows, each clearing the last of its row', () => {
    expect(stackRows([30, 60, 90], 20)).toEqual([0, 0, 0])
    expect(stackRows([30, 30, 35, 60], 20)).toEqual([0, 1, 2, 0])
    // The backlog chip holds the first row up to 25%: a card reaching into it
    // (left edge 20 - 10 < 25) drops a row; one clear of it (45 - 10) stays.
    expect(stackRows([20, 45], 20, 25)).toEqual([1, 0])
    expect(stackRows([20], 20, 0)).toEqual([0])
    expect(backlogPct('Backlog 1: IN-002')).toBeGreaterThan(15)
    expect(stackRows([60, 30, 31], 20)).toEqual([0, 0, 1])
    expect(stackRows([])).toEqual([])
  })

  test('a second row sits a card and 8px under the first, the track growing', () => {
    expect(rowTop(0)).toBe(10)
    expect(rowTop(1)).toBe(10 + 92 + 8)
    expect(trackHeight(0)).toBe(74)
    expect(trackHeight(2)).toBe(rowTop(1) + 92 + 10)
  })
})

describe('the lane timeline’s definition', () => {
  test('a lane takes its colour field, else the palette by its index', () => {
    expect(laneColor({ color: '#123456' }, 0)).toBe('#123456')
    expect(laneColor({ color: 'plan' }, 0)).toBe(color('plan'))
    expect(laneColor({}, 0)).toBe(color('lane-1'))
    expect(laneColor({}, 3)).toBe(color('lane-4'))
    expect(laneColor({}, 6)).toBe(color('lane-1'))
  })

  test('the item view is the items field’s element type’s, a named type', () => {
    const schema = dataType('stream-lane')
    const item = elementSchema(schema, 'releases')!
    expect(item.root).toBe('ReleaseCard')
    expect(elementSchema(schema, 'name')).toBeUndefined()
    const tl = buildUITimelineLanes({ schema })!
    expect(tl.definition.laneHeader).toBe('lane-header')
    expect(tl.header?.definition.name).toBe('lane-header')
    expect(tl.itemView?.definition.name).toBe('card')
    expect(tl.itemView?.schema.root).toBe('ReleaseCard')
    expect(tl.backlogView).toBeUndefined()
    expect(tl.itemId({ id: 'RL-033' })).toBe('RL-033')
    expect(toUITimelineLanesDef(schema, 'nope')).toBeUndefined()
  })

  test('an inline element type takes a name of its own', () => {
    const schema = { name: 'x', root: 'L', types: { L: { type: 'object', fields: { items: { type: 'array', items: { type: 'object', fields: { id: { type: 'string' } } } } } } } } as never
    const s = elementSchema(schema, 'items')!
    expect(s.root).toBe('L.items[]')
    expect((s.types as any)['L.items[]'].type).toBe('object')
  })

  test('the backlog chip lists three ids and counts them all', () => {
    expect(backlogLabel(['IN-008', 'IN-009', 'IN-011', 'IN-012'])).toBe('Backlog 4: IN-008, IN-009, IN-011…')
    expect(backlogLabel(['IN-001p', 'IN-002p'])).toBe('Backlog 2: IN-001p, IN-002p')
  })

  test('a card’s click runs the page’s select verb with the item’s id', () => {
    const tl = buildUITimelineLanes({ schema: dataType('stream-lane') })!
    const ran: unknown[] = []
    const sel = tableSelection({ definition: tl.definition, rowId: tl.itemId }, (item) => ran.push(item), { release: 'RL-033' })
    const card = deliveryLanes[0].releases[1]
    sel.onRowSelect!(card)
    expect(ran).toEqual([card])
    expect(sel.isSelected!(deliveryLanes[0].releases[0])).toBe(true)
    expect(sel.isSelected!(card)).toBe(false)
    const set: Record<string, unknown> = {}
    routePageAction({ action: selectAction(tl.definition.select!), item: card }, {
      show: () => {}, select: (k, v) => { set[k] = v }, host: () => {}, selection: {},
    })
    expect(set).toEqual({ release: 'RL-034' })
  })

  test('timeline-lanes is a schema-bound kind whose data binds as the lanes', () => {
    expect(SCHEMA_BOUND_KINDS).toContain('timeline-lanes')
    const bound = applyDataBinding({ kind: 'timeline-lanes', dataField: '$.lanes' } as never, { lanes: deliveryLanes }) as { data: unknown }
    expect(bound.data).toEqual(deliveryLanes)
  })
})

describe('the drawing', () => {
  const lanes = [
    { ...deliveryLanes[0], releases: deliveryLanes[0].releases.slice(0, 2) },
    deliveryLanes[1],
  ]
  const draw = (selection: Record<string, unknown>) => renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(UIPageView, {
      node: { kind: 'timeline-lanes', schema: timelineSchema('2026-10-18'), data: lanes } as never,
      data: { selection }, onAction: () => {},
    })))

  test('two lanes, three releases, a backlog, the today line, the selected card outlined', () => {
    const out = draw({ release: 'RL-033' })
    expect(out).toContain('data-ktw-timeline-lanes=""')
    expect(out).toContain('Stream · release mode')
    expect((out.match(/data-part="lane"/g) ?? []).length).toBe(2)
    expect((out.match(/data-part="item"/g) ?? []).length).toBe(3)
    for (const s of ['KaTyoti', 'ArchiFactory', 'SM-001', 'RL-033', 'RL-034', 'RL-014', '1.006', '1.007', '2026.10', 'Cut now']) {
      expect(out).toContain(s)
    }
    expect(out).toContain('Backlog 11: IN-008, IN-009, IN-011…')
    expect(out).toContain('title="IN-008, IN-009, IN-011, IN-012, IN-013, IN-014, IN-015, IN-019, IN-023, IN-027, IN-028"')
    expect(out).toContain('Backlog 8: IN-001, IN-002, IN-003…')
    expect((out.match(/data-part="today"/g) ?? []).length).toBe(2)
    expect(out).toContain('data-tick="19/10"')
    expect(out).toMatch(/aria-pressed="true"[^>]*data-item-id="RL-033"/)
    expect(out).toMatch(/aria-pressed="false"[^>]*data-item-id="RL-034"/)
    expect(out).toContain('0 0 0 3px')
  })

  test('nothing selected outlines no card', () => {
    const out = draw({})
    expect(out).not.toContain('aria-pressed="true"')
  })

  test('an item with no date is still drawn, at today, dashed', () => {
    const undated = [{ ...deliveryLanes[1], releases: deliveryLanes[1].releases.map((r) => ({ ...r, date: undefined })) }]
    const out = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(UIPageView, { node: { kind: 'timeline-lanes', schema: timelineSchema('2026-10-18'), data: undated } as never, data: {} })))
    expect(out).toMatch(/data-item-id="RL-014"[^>]*data-undated="true"/)
    expect(out).toContain('title="no date"')
    expect(out).toContain('dashed')
  })
})
