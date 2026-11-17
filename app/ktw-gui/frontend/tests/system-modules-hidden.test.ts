// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { visibleObjects } from '../src/sections/workspace/systemModules'

// The objects tree, the Home module select and the object pickers
// show system modules (and what lies in them) at the Expert level only. The
// flag is data (CWM's system-module capability), never the sys- prefix: a
// user module named like one stays, a flagged one goes whatever its name.
const rows = [
  { id: 'project', depth: 0 },
  { id: 'billing', depth: 1 },
  { id: 'billing.F-001', depth: 2 },
  { id: 'sys-inbox', depth: 1, system: true },
  { id: 'sys-inbox.IB-001', depth: 2, system: true },
  { id: 'ops-journal', depth: 1, system: true },
  { id: 'ops-journal.J-001', depth: 2 },
  { id: 'sys-notes', depth: 1 },
]

describe('system modules below Expert', () => {
  test('are left out, with what lies in them, at Essentiel and Standard', () => {
    for (const level of ['essential', 'standard'] as const) {
      expect(visibleObjects(rows, level).map((r) => r.id)).toEqual(['project', 'billing', 'billing.F-001', 'sys-notes'])
    }
  })

  test('are all there at Expert', () => {
    expect(visibleObjects(rows, 'expert')).toEqual(rows)
  })

  test('a flat list (search results) drops the flagged objects', () => {
    const found = [{ id: 'sys-inbox' , system: true }, { id: 'billing.F-001' }, { id: 'sys-inbox.IB-002', system: true }]
    expect(visibleObjects(found, 'standard').map((r) => r.id)).toEqual(['billing.F-001'])
  })
})
