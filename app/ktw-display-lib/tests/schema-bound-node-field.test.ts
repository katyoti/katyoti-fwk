// SPDX-License-Identifier: Apache-2.0
// A list whose elements hold their own node (`nodeField`) - « À
// faire »'s cards - holds the forms those nodes carry, so the host resolves
// them (their ui block laid over the schema) before the page renders;
// otherwise each card shows « Cannot render form node ». Any other `data`
// stays a node's own payload.

import { describe, expect, test } from 'bun:test'
import { hasSchemaBound } from '../src/ui-builders/page/index.ts'

const form = {
  kind: 'form',
  schema: { root: 'A', types: { A: { type: 'object' } } },
  ui: { actions: [{ type: 'ui-action', id: 'submit', label: 'Send' }] },
}
const card = { kind: 'card', children: [{ kind: 'text', content: 'Which store?' }, form] }

describe('hasSchemaBound', () => {
  test('sees a form a list element holds in its node field', () => {
    const tree = { kind: 'vbox', children: [{ kind: 'list', nodeField: 'card', data: [{ id: 'a', kind: 'approve', card }] }] }
    expect(hasSchemaBound(tree)).toBe(true)
  })

  test('keeps any other data as payload', () => {
    expect(hasSchemaBound({ kind: 'list', item: { kind: 'text', content: '' }, data: [{ id: 'a', card }] })).toBe(false)
    expect(hasSchemaBound({ kind: 'list', nodeField: 'card', data: [{ id: 'a', other: card }] })).toBe(false)
  })
})
