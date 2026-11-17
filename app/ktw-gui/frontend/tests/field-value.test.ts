// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { fieldView } from '../src/sections/workspace/fieldValue'

// A fields tab's read mode draws a structured value as a
// readable list or label/value rows, never as raw JSON.
describe('a stored field value', () => {
  test('a selection reads as a list of records: title, who kept it, when', () => {
    const selection = {
      items: [
        { key: 'p-1', title: 'Architecture', kept_by: { id: 'jdoe', kind: 'person' }, kept_at: '2026-11-13T20:22:17Z' },
        { key: 'p-2', title: 'Runbook', with_children: true },
      ],
    }
    expect(fieldView(selection)).toEqual({
      kind: 'rows',
      rows: [{
        label: 'items',
        value: {
          kind: 'list',
          items: [
            { kind: 'record', title: 'Architecture', facts: ['jdoe', '2026-11-13 20:22'] },
            { kind: 'record', title: 'Runbook', facts: ['with children'] },
          ],
        },
      }],
    })
  })

  test('an object reads as label/value rows, a scalar as its text', () => {
    expect(fieldView({ space: 'SAB', pageCount: 3 })).toEqual({
      kind: 'rows',
      rows: [
        { label: 'space', value: { kind: 'text', text: 'SAB' } },
        { label: 'page count', value: { kind: 'text', text: '3' } },
      ],
    })
    expect(fieldView(['a', 'b'])).toEqual({ kind: 'list', items: [{ kind: 'text', text: 'a' }, { kind: 'text', text: 'b' }] })
  })

  test('no view carries JSON text', () => {
    const text = JSON.stringify(fieldView({ items: [{ key: 'k', title: 'T', kept_by: { id: 'x', kind: 'agent' } }], deep: { a: [{ b: 1 }] } }))
    expect(text).not.toContain('{\\"')
  })
})

describe('the fields tab read mode', () => {
  test('draws every value through fieldView, never JSON.stringify', () => {
    const src = require('fs').readFileSync(require('path').join(__dirname, '../src/sections/workspace/ObjectFieldsForm.tsx'), 'utf8')
    expect(src).toContain('fieldView(v)')
    expect(src).not.toContain('JSON.stringify(v')
  })
})
