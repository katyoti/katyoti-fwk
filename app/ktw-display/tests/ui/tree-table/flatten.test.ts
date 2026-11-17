// SPDX-License-Identifier: Apache-2.0
// tree-table flatten() — hierarchy derivation from a flat, parent-id-linked
// node list. The cases below are the ones that used to recurse forever and blow
// the render stack ("Maximum call stack size exceeded").

import { describe, expect, it } from 'bun:test'

import { flatten } from '../../../../ktw-display-lib/src/ui-builders/tree-table/ui-tree-table.view.tsx'

type Row = { id?: string; parent_id?: string; title: string }

const NONE = new Set<string>()

const run = (rows: Row[], collapsed = NONE) => flatten<Row>(rows, 'id', 'parent_id', collapsed)

describe('flatten — hierarchy', () => {
  it('orders depth-first with a depth per level', () => {
    const rows = run([
      { id: '1', title: 'root' },
      { id: '2', parent_id: '1', title: 'child' },
      { id: '3', parent_id: '2', title: 'grandchild' },
    ])
    expect(rows.map((r) => [r.row.title, r.depth])).toEqual([
      ['root', 0],
      ['child', 1],
      ['grandchild', 2],
    ])
    expect(rows[0].hasChildren).toBe(true)
    expect(rows[2].hasChildren).toBe(false)
  })

  it('treats an unknown parent as a root', () => {
    const rows = run([{ id: '2', parent_id: 'gone', title: 'orphan' }])
    expect(rows.map((r) => r.depth)).toEqual([0])
  })

  it('hides the children of a collapsed node but keeps its chevron', () => {
    const rows = run(
      [
        { id: '1', title: 'root' },
        { id: '2', parent_id: '1', title: 'child' },
      ],
      new Set(['1']),
    )
    expect(rows.map((r) => r.row.title)).toEqual(['root'])
    expect(rows[0].hasChildren).toBe(true)
  })
})

describe('flatten — degenerate data terminates', () => {
  it('renders an id-less row as a leaf instead of recursing into the roots', () => {
    // A confluence page that exists only locally has no page id. The empty key
    // addresses the ROOT bucket, so treating it as a parent key made the row
    // its own ancestor.
    const rows = run([
      { id: '6325895301', title: 'STOREWAY' },
      { id: '', parent_id: '6325895301', title: 'nouvelle page' },
    ])
    expect(rows.map((r) => r.row.title)).toEqual(['STOREWAY', 'nouvelle page'])
    expect(rows[1].depth).toBe(1)
    expect(rows[1].hasChildren).toBe(false)
    expect(rows[1].id).not.toBe('')
  })

  it('handles a missing id field the same way', () => {
    const rows = run([{ title: 'no id at all' }, { title: 'nor here' }])
    expect(rows).toHaveLength(2)
    expect(rows.every((r) => !r.hasChildren)).toBe(true)
  })

  it('breaks a two-node parent cycle', () => {
    const rows = run([
      { id: 'a', parent_id: 'b', title: 'A' },
      { id: 'b', parent_id: 'a', title: 'B' },
    ])
    // Unreachable from any root — surfaced flat rather than dropped or looped.
    expect(rows.map((r) => r.row.title).sort()).toEqual(['A', 'B'])
  })

  it('breaks a self-parenting node', () => {
    const rows = run([
      { id: '1', title: 'root' },
      { id: 'x', parent_id: 'x', title: 'self' },
    ])
    expect(rows.map((r) => r.row.title).sort()).toEqual(['root', 'self'])
  })

  it('keeps every row when ids collide', () => {
    const rows = run([
      { id: 'dup', title: 'first' },
      { id: 'dup', title: 'second' },
    ])
    expect(rows).toHaveLength(2)
  })
})
