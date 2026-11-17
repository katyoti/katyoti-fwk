// SPDX-License-Identifier: Apache-2.0
// Tests for the minimal JSONPath resolver — display-package-restructure
// Phase 3.1.

import { describe, expect, it } from 'bun:test'
import { resolveJSONPath, setJSONPath } from '../../../ktw-display-lib/src/utils/json-path.ts'

describe('resolveJSONPath — basic paths', () => {
  it('returns the data when path is $ or empty', () => {
    expect(resolveJSONPath({ a: 1 }, '$')).toEqual({ a: 1 })
    expect(resolveJSONPath({ a: 1 }, '')).toEqual({ a: 1 })
  })

  it('resolves a top-level property', () => {
    expect(resolveJSONPath({ a: 42 }, '$.a')).toBe(42)
  })

  it('resolves a nested property', () => {
    expect(resolveJSONPath({ a: { b: 'x' } }, '$.a.b')).toBe('x')
  })

  it('returns undefined for missing properties', () => {
    expect(resolveJSONPath({ a: 1 }, '$.b')).toBeUndefined()
    expect(resolveJSONPath({ a: 1 }, '$.a.b')).toBeUndefined()
  })

  it('returns undefined when descending into null/undefined', () => {
    expect(resolveJSONPath({ a: null }, '$.a.b')).toBeUndefined()
    expect(resolveJSONPath(null, '$.a')).toBeUndefined()
  })
})

describe('resolveJSONPath — array indices', () => {
  it('resolves an array index', () => {
    expect(resolveJSONPath({ a: [10, 20, 30] }, '$.a[0]')).toBe(10)
    expect(resolveJSONPath({ a: [10, 20, 30] }, '$.a[2]')).toBe(30)
  })

  it('resolves index then property', () => {
    expect(resolveJSONPath({ items: [{ x: 1 }, { x: 2 }] }, '$.items[1].x')).toBe(2)
  })

  it('returns undefined for out-of-bounds indices', () => {
    expect(resolveJSONPath({ a: [1, 2] }, '$.a[5]')).toBeUndefined()
  })

  it('returns undefined when indexing a non-array', () => {
    expect(resolveJSONPath({ a: { 0: 'oops' } }, '$.a[0]')).toBeUndefined()
  })
})

describe('resolveJSONPath — wildcards', () => {
  it('returns array of all elements with $.field[*]', () => {
    expect(resolveJSONPath({ a: [1, 2, 3] }, '$.a[*]')).toEqual([1, 2, 3])
  })

  it('maps property access through wildcard with $.items[*].x', () => {
    expect(
      resolveJSONPath({ items: [{ x: 1 }, { x: 2 }, { x: 3 }] }, '$.items[*].x'),
    ).toEqual([1, 2, 3])
  })

  it('handles missing properties under wildcard', () => {
    expect(
      resolveJSONPath({ items: [{ x: 1 }, { y: 2 }, { x: 3 }] }, '$.items[*].x'),
    ).toEqual([1, undefined, 3])
  })

  it('handles wildcard on non-array', () => {
    expect(resolveJSONPath({ a: { not: 'array' } }, '$.a[*]')).toBeUndefined()
  })
})

describe('resolveJSONPath — leading $ and `.` flexibility', () => {
  it('accepts `.field` as `$.field`', () => {
    expect(resolveJSONPath({ a: 1 }, '.a')).toBe(1)
  })

  it('accepts `field` as `$.field`', () => {
    expect(resolveJSONPath({ a: 1 }, 'a')).toBe(1)
  })

  it('accepts `$field` (no dot) as `$.field`', () => {
    expect(resolveJSONPath({ a: 1 }, '$a')).toBe(1)
  })
})

describe('resolveJSONPath — typical UI-binding cases', () => {
  const customer = {
    name: 'Acme Corp',
    contact: { email: 'sales@acme.example', phone: '555' },
    orders: [
      { id: 'o1', total: 100, status: 'paid' },
      { id: 'o2', total: 250, status: 'pending' },
      { id: 'o3', total: 50, status: 'paid' },
    ],
    metrics: {
      mrr: 5000,
      open_tickets: 3,
    },
  }

  it('binds KPI value', () => {
    expect(resolveJSONPath(customer, '$.metrics.open_tickets')).toBe(3)
  })

  it('binds heading text', () => {
    expect(resolveJSONPath(customer, '$.name')).toBe('Acme Corp')
  })

  it('binds nested contact field', () => {
    expect(resolveJSONPath(customer, '$.contact.email')).toBe('sales@acme.example')
  })

  it('binds table data via wildcard', () => {
    expect(resolveJSONPath(customer, '$.orders[*]')).toHaveLength(3)
  })

  it('binds a derived column via wildcard + property', () => {
    expect(resolveJSONPath(customer, '$.orders[*].total')).toEqual([100, 250, 50])
  })
})

describe('resolveJSONPath — invalid input', () => {
  it('throws on unclosed bracket', () => {
    expect(() => resolveJSONPath({ a: [1] }, '$.a[0')).toThrow()
  })

  it('throws on non-integer bracket index', () => {
    expect(() => resolveJSONPath({ a: [1] }, '$.a[abc]')).toThrow()
  })
})

describe('setJSONPath', () => {
  it('writes a nested property without touching the input', () => {
    const data = { book: { title: 'A', year: 1 }, other: { x: 1 } }
    const out = setJSONPath(data, '$.book.title', 'B') as typeof data
    expect(out.book).toEqual({ title: 'B', year: 1 })
    expect(data.book.title).toBe('A')
    expect(out.other).toBe(data.other)
  })

  it('writes an array index and creates missing containers', () => {
    expect(setJSONPath({ a: [1, 2] }, '$.a[1]', 5)).toEqual({ a: [1, 5] })
    expect(setJSONPath(undefined, '$.a.b', 1)).toEqual({ a: { b: 1 } })
    expect(setJSONPath({}, '$', 7)).toBe(7)
  })

  it('refuses a wildcard', () => {
    expect(() => setJSONPath({ a: [] }, '$.a[*].x', 1)).toThrow('wildcard')
  })
})
