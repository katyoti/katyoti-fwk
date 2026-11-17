// SPDX-License-Identifier: Apache-2.0
// The one answer's ops: a pulled section, a live patch and a
// panel event change a page by node id, in the order of seq.

import { describe, expect, test } from 'bun:test'

import { applyPageOps, isNamedSpec, isSection, pageState, type PageOp } from '../../../ktw-display-lib/src/ui-builders/page/answer'

const page = () => ({
  kind: 'vbox',
  children: [
    { kind: 'section', title: 'A', children: [{ kind: 'text', id: 's1', content: '…', load: { method: 'ktw-x:views/a' } }] },
    { kind: 'tabs', tabs: [{ id: 'tab:b', label: 'B', content: { kind: 'list', id: 'rows', data: [1], item: { kind: 'text', content: '${item}' } } }] },
    { kind: 'text', id: 'gone', content: 'x' },
  ],
})

const find = (v: any, id: string): any => {
  if (!v || typeof v !== 'object') return undefined
  if (v.id === id) return v
  for (const k of Object.keys(v)) {
    const r = find(v[k], id)
    if (r) return r
  }
  return undefined
}

describe('page ops by node id', () => {
  test('replace, append, merge, remove, fail and complete', () => {
    const ops: PageOp[] = [
      { seq: 1, op: 'replace', target: 's1', node: { kind: 'text', content: 'loaded' } },
      { seq: 2, op: 'append', target: 'rows', items: [2, 3] },
      { seq: 3, op: 'merge', target: 'rows', value: { pageSize: 10 } },
      { seq: 4, op: 'remove', target: 'gone' },
      { seq: 5, op: 'fail', target: 's1', message: 'boom' },
      { seq: 6, op: 'complete' },
    ]
    const s = applyPageOps(pageState(page()), ops)
    expect(find(s.tree, 's1')).toEqual({ kind: 'text', content: 'loaded', id: 's1' })
    expect(find(s.tree, 'rows').data).toEqual([1, 2, 3])
    expect(find(s.tree, 'rows').pageSize).toBe(10)
    expect(find(s.tree, 'gone')).toBeUndefined()
    expect(s.failed).toEqual({ s1: 'boom' })
    expect(s.complete).toBe(true)
    expect(s.seq).toBe(6)
  })

  test('ops apply in the order of seq, not of arrival', () => {
    const late: PageOp[] = [
      { seq: 2, op: 'replace', target: 's1', node: { kind: 'text', content: 'second' } },
      { seq: 1, op: 'replace', target: 's1', node: { kind: 'text', content: 'first' } },
    ]
    expect(find(applyPageOps(pageState(page()), late).tree, 's1').content).toBe('second')
    const appends: PageOp[] = [
      { seq: 2, op: 'append', target: 'rows', items: ['b'] },
      { seq: 1, op: 'append', target: 'rows', items: ['a'] },
    ]
    expect(find(applyPageOps(pageState(page()), appends).tree, 'rows').data).toEqual([1, 'a', 'b'])
  })

  test('an op already applied is skipped', () => {
    const s = applyPageOps(pageState(page()), [{ seq: 4, op: 'append', target: 'rows', items: [2] }])
    const again = applyPageOps(s, [{ seq: 4, op: 'append', target: 'rows', items: [2] }])
    expect(find(again.tree, 'rows').data).toEqual([1, 2])
  })

  test('an op for an id not drawn yet waits, then applies once it is', () => {
    const s = applyPageOps(pageState(page()), [{ seq: 1, op: 'append', target: 'later', items: ['x'] }])
    expect(s.pending.length).toBe(1)
    const drawn = applyPageOps(s, [{ seq: 2, op: 'replace', target: 'gone', node: { kind: 'vbox', children: [{ kind: 'list', id: 'later', data: [], item: { kind: 'text', content: '' } }] } }])
    // seq 1 waited, seq 2 drew its target: the next apply takes it
    const next = applyPageOps(drawn, [])
    expect(find(next.tree, 'later').data).toEqual(['x'])
    expect(next.pending.length).toBe(0)
  })

  test('a successful op on a failed node clears its failure', () => {
    const s = applyPageOps(pageState(page()), [{ seq: 1, op: 'fail', target: 's1', message: 'boom' }])
    const retried = applyPageOps(s, [{ seq: 2, op: 'replace', target: 's1', node: { kind: 'text', content: 'ok' } }])
    expect(retried.failed).toEqual({})
  })

  test('the state given is never changed', () => {
    const tree = page()
    const before = JSON.stringify(tree)
    applyPageOps(pageState(tree), [{ seq: 1, op: 'remove', target: 'gone' }])
    expect(JSON.stringify(tree)).toBe(before)
  })
})

describe('specs and sections', () => {
  test('only {uri} or {uri, hash} names a part', () => {
    expect(isNamedSpec({ uri: 'ktw-x:/data-type/r', hash: 'h' })).toBe(true)
    expect(isNamedSpec({ uri: 'ktw-x:/data-type/r' })).toBe(true)
    expect(isNamedSpec({ kind: 'link', uri: 'x' })).toBe(false)
    expect(isNamedSpec({ kind: 'text', content: 'x' })).toBe(false)
  })
  test('a section has its id and its load method', () => {
    expect(isSection({ kind: 'card', id: 'a', load: { method: 'm' } })).toBe(true)
    expect(isSection({ kind: 'card', load: { method: 'm' } })).toBe(false)
    expect(isSection({ kind: 'card', id: 'a', load: {} })).toBe(false)
  })
})
