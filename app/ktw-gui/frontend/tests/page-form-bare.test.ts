// SPDX-License-Identifier: Apache-2.0
// A form answering a question inside a page (the Request panel's answer, a
// deliverable verified by hand) shows its fields and one button: no heading of
// its own — `hideTitle` on the page's form node, its section titles it — and
// no Delete, Cancel or Reset — a `ui` block naming its one action, submit.

import { describe, expect, test } from 'bun:test'

import { formViewOptions } from '../../../ktw-display-lib/src/ui-builders/page/nodes/schema-bound'
import { buildUIForms } from '../../../ktw-display-lib/src/ui-builders/form/index'
import { wireRegistries } from '../../../ktw-display-lib/src/registry-wire'
import type { Schema } from '../../../ktw-display-lib/src/schema/index'

const schema = (extensions?: unknown[]): Schema => ({
  root: 'ApproveOrSendBack',
  types: {
    ApproveOrSendBack: {
      type: 'object',
      fields: { decision: { type: 'string' }, note: { type: 'string', optional: true } },
      ...(extensions ? { extensions } : {}),
    },
  },
}) as unknown as Schema

describe('a form node drawn bare', () => {
  test('goes without its heading when the node says hideTitle, and only then', () => {
    expect(formViewOptions({ kind: 'form', mode: 'edit', hideTitle: true }).hideTitle).toBe(true)
    expect(formViewOptions({ kind: 'form', mode: 'edit' }).hideTitle).toBe(false)
    expect(formViewOptions({ kind: 'form', mode: 'create' }).isNew).toBe(true)
  })

  test('offers its submit only when its ui block names it, with no Delete in its toolbar', () => {
    wireRegistries()
    const answer = buildUIForms({
      schema: schema([{ type: 'ui-form', name: 'default', actions: [{ type: 'ui-action', id: 'submit', scope: 'item', label: 'Answer', icon: 'send' }] }]),
    }).default
    expect(answer.actions.map((a) => a.id)).toEqual(['submit'])
    expect(answer.actions[0].label).toBe('Answer')
    expect(answer.toolbar).toBeUndefined()

    // Without it, a form is an object's editor: Delete in its toolbar.
    const editor = buildUIForms({ schema: schema() }).default
    expect(editor.actions.map((a) => a.id)).toContain('delete')
    expect(editor.toolbar).toBeDefined()
  })
})
