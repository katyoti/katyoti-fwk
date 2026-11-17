// SPDX-License-Identifier: Apache-2.0
// The launch modal's input: the action's input AST without what the launch
// fills itself (node_id, the binding's pre-bound values) — nested fields
// kept, the display library's form draws them — and whether a modal is needed
// at all (a confirm gate, or any free field, nested or behind a $ref root).

import { describe, expect, it } from 'bun:test'

import { freeInput, needsLaunchDialog } from '../src/sections/workspace/ActionLaunchModal'

const ast = {
  name: 'in', root: 'FileInput',
  types: {
    FileInput: {
      type: 'object',
      fields: {
        node_id: { type: 'string' }, module: { type: 'string' },
        title: { type: 'string' }, lines: { type: 'array', optional: true },
      },
    },
  },
}

describe('ActionLaunchModal input', () => {
  it('drops node_id and the pre-bound fields, keeping the nested ones', () => {
    const got = freeInput(ast, { module: 'billing' })
    expect(Object.keys(got!.types!.FileInput.fields!)).toEqual(['title', 'lines'])
    expect(ast.types.FileInput.fields.module).toBeDefined() // the original is not changed
  })

  it('nothing left to ask is no form', () => {
    expect(freeInput(ast, { module: 'm', title: 't', lines: [] })).toBeNull()
    expect(freeInput(undefined, {})).toBeNull()
  })

  it('a modal opens for a nested-only input and for a $ref-rooted task input', () => {
    const action = { uri: 'x:y/z', label: 'z', kind: 'sync', source: '' } as never
    const nested = JSON.stringify({ type: 'object', properties: { node_id: {}, lines: { type: 'array' } } })
    expect(needsLaunchDialog(action, nested)).toBe(true)
    const task = JSON.stringify({ $ref: '#/$defs/In', $defs: { In: { type: 'object', properties: { topic: { type: 'string' } } } } })
    expect(needsLaunchDialog(action, task)).toBe(true)
    expect(needsLaunchDialog(action, JSON.stringify({ type: 'object', properties: { node_id: {} } }))).toBe(false)
  })
})
