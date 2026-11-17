// SPDX-License-Identifier: Apache-2.0
// The hard delete is offered only when nothing points to the
// object - no link to it, no child -, never while that is being asked or when
// the question failed.

import { describe, expect, test } from 'bun:test'
import { hardDeleteBlock } from '../src/sections/workspace/retire'

describe('the hard delete', () => {
  test('is offered only when nothing points to the object', () => {
    expect(hardDeleteBlock([], '', 0)).toBeUndefined()
    expect(hardDeleteBlock(['b.F-1'], '', 0)).toBe('linked')
    expect(hardDeleteBlock([], '', 2)).toBe('children')
    expect(hardDeleteBlock(undefined, '', 0)).toBe('checking')
    expect(hardDeleteBlock(undefined, 'hub down', 0)).toBe('unknown')
    expect(hardDeleteBlock([], 'hub down', 0)).toBe('unknown')
  })
})
