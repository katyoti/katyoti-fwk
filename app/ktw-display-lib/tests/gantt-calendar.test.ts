// SPDX-License-Identifier: Apache-2.0
// A gantt on a calendar scale: tasks dated from / to are placed in days from
// the first month any of them (or today) touches, a tick per month, a today
// line where today falls; a bar still opens its item.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { ganttCalendar } from '../src/ui-builders/page/ui-page.interactive.ts'
import { GanttRenderer } from '../src/ui-builders/page/nodes/interactive.tsx'

const tasks = [
  { id: 'RL-1', title: '1.006', from: '2026-10-01', to: '2026-10-31' },
  { id: 'IN-1', title: 'Delivery at a glance', from: '2026-10-15', to: '2026-11-10' },
  { id: 'X', title: 'undated' },
]

describe('a calendar gantt', () => {
  test('places dated tasks in days, a tick per month, today on its day', () => {
    const c = ganttCalendar(tasks, '2026-10-15')
    expect(c.tasks.map((t) => [t.id, t.start, t.length])).toEqual([['RL-1', 0, 31], ['IN-1', 14, 27]])
    expect(c.span).toBe(61) // October and November
    expect(c.ticks).toEqual([{ at: 0, label: 'Oct 2026' }, { at: 31, label: 'Nov' }])
    expect(c.today).toBe(14)
  })

  test('crosses a year with the year on January', () => {
    const c = ganttCalendar([{ id: 'a', title: 'a', from: '2026-12-20', to: '2027-01-05' }])
    expect(c.ticks.map((k) => k.label)).toEqual(['Dec 2026', 'Jan 2027'])
    expect(c.today).toBeUndefined()
  })

  test('draws month ticks and the today line', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(GanttRenderer, { node: { kind: 'gantt', scale: 'calendar', today: '2026-10-15', data: tasks } })))
    expect(html).toContain('Oct 2026')
    expect(html).toContain('Nov')
    expect(html).toContain('data-gantt-today')
    expect(html).toContain('Delivery at a glance: 2026-10-15 to 2026-11-10')
    expect(html).not.toContain('undated')
  })

  test('keeps the relative scale by default', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(GanttRenderer, { node: { kind: 'gantt', unit: 'h', data: [{ id: 'a', title: 'A', start: 0, length: 4 }] } })))
    expect(html).toContain('4 h')
    expect(html).not.toContain('data-gantt-today')
  })
})
