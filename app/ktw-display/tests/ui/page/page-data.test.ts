// SPDX-License-Identifier: Apache-2.0
// The page's working copy of its data — a form and a view of the same data
// in sync (app/ktw-display-lib/src/ui-builders/page/page-data.ts).
import { describe, expect, it } from 'bun:test'

import {
  edit,
  initialState,
  rebase,
} from '../../../../ktw-display-lib/src/ui-builders/page/page-data.ts'

const book = (title: string, author = 'Ann') => ({ book: { title, author }, loans: 3 })

describe('page working copy', () => {
  it("a form's first values are not an edit", () => {
    const s = initialState(book('Dune'))
    expect(edit(s, '$.book', { title: 'Dune', author: 'Ann' })).toBe(s)
  })

  it('an edit reaches what bound nodes read, not what forms read', () => {
    const s = edit(initialState(book('Dune')), '$.book', { title: 'Dune II', author: 'Ann' })
    expect([...s.edits]).toEqual([['$.book', { title: 'Dune II', author: 'Ann' }]])
    expect(s.seed).toEqual(book('Dune'))
  })

  it('new producer data keeps the edited path and says it changed upstream', () => {
    let s = edit(initialState(book('Dune')), '$.book', { title: 'Dune II', author: 'Ann' })
    s = rebase(s, { book: { title: 'Dune', author: 'Bob' }, loans: 4 })
    expect(s.seed).toEqual({ book: { title: 'Dune II', author: 'Ann' }, loans: 4 })
    expect([...s.upstream]).toEqual(['$.book'])
  })

  it('new producer data that leaves the edited path alone is not a conflict', () => {
    let s = edit(initialState(book('Dune')), '$.book', { title: 'Dune II', author: 'Ann' })
    s = rebase(s, { ...book('Dune'), loans: 9 })
    expect(s.upstream.size).toBe(0)
    expect(s.seed).toEqual({ book: { title: 'Dune II', author: 'Ann' }, loans: 9 })
  })

  it("going back to the producer's value drops the edit and the conflict", () => {
    let s = edit(initialState(book('Dune')), '$.book', { title: 'X', author: 'Ann' })
    s = rebase(s, book('Dune', 'Bob'))
    s = edit(s, '$.book', { title: 'Dune', author: 'Bob' })
    expect(s.edits.size).toBe(0)
    expect(s.upstream.size).toBe(0)
  })
})
