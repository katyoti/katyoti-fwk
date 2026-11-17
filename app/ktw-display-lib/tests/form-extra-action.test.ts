// SPDX-License-Identifier: Apache-2.0
// A page form's extra ui-action carrying a verb (uri) runs
// through the host once - its input the form's value with the action's args
// laid over, `${person}` resolved by the host - while the submit still runs
// only the node's submit action ('s « Créer et faire l'étude »).

import { describe, expect, test } from 'bun:test'

import { setRendererLookup, type FieldRendererFactory } from '../src/ui-builders/fields/index.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { formActionProcessor } from '../src/ui-builders/page/nodes/schema-bound.tsx'
import { resolveAction, type UIPageAction } from '../src/ui-builders/page/ui-page.interactive.ts'
import type { Schema } from '../src/schema/index.ts'

const stubFactory: FieldRendererFactory = { view: () => () => null, form: () => () => null, header: () => () => null, column: () => () => null }
setRendererLookup(() => stubFactory)

const STUDY_NEW = 'ktw-example:example/study-new'

// The schema as the host hands it: the node's ui block laid over as its
// ui-form extension, the extra action keeping its uri and args.
const schema = {
  name: 'study-new',
  root: 'Study',
  types: {
    Study: {
      type: 'object',
      fields: { title: { type: 'string' }, scopes: { type: 'array', items: 'string' } },
      extensions: [{
        type: 'ui-form', name: 'default', actions: [
          { type: 'ui-action', id: 'submit', scope: 'item', label: 'Create the study', uri: STUDY_NEW },
          { type: 'ui-action', id: 'create-and-do', scope: 'item', label: 'Create and do the study', uri: STUDY_NEW, args: { run: true, by: '${person}' } },
          { type: 'ui-action', id: 'nothing-to-run', scope: 'item', label: 'No verb' },
        ],
      }],
    },
  },
} as unknown as Schema

const person = { id: 'jdoe', kind: 'person' }

/** A page's runOf that records what it hands the host, as the host resolves it. */
function recorder() {
  const sent: { action: UIPageAction; args?: Record<string, unknown> }[] = []
  const runOf = (action: UIPageAction | undefined) => action?.uri
    ? (item?: unknown, extra?: Record<string, unknown>) => {
      sent.push({ action, args: resolveAction(action, item, { person, id: 'acme-da', ...(extra ?? {}) }).args })
    }
    : undefined
  return { sent, runOf }
}

function setup() {
  const form = buildUIForms({ schema })['default']
  const { sent, runOf } = recorder()
  const processor = formActionProcessor(form.definition.actions, runOf)
  const actionOf = (id: string) => form.actions.find((a) => a.id === id)!
  return { sent, processor, actionOf }
}

describe('a page form runs its extra ui-actions through the host', () => {
  test('the extra action calls the host once, with the form value and its args laid over', async () => {
    const { sent, processor, actionOf } = setup()
    expect(processor).toBeDefined()
    const value = { title: 'As-is billing', scopes: ['app:crm', 'dom:sales'] }
    await processor!({ action: actionOf('create-and-do'), item: value })
    expect(sent.length).toBe(1)
    expect(sent[0].action.uri).toBe(STUDY_NEW)
    expect(sent[0].args).toEqual({ title: 'As-is billing', scopes: ['app:crm', 'dom:sales'], run: true, by: person })
  })

  test('an arg named like a field wins over the typed value', async () => {
    const { sent, processor, actionOf } = setup()
    await processor!({ action: actionOf('create-and-do'), item: { title: 'x', run: false } })
    expect(sent[0].args?.run).toBe(true)
  })

  test('what the person typed is laid as it is, never templated', async () => {
    const { sent, processor, actionOf } = setup()
    await processor!({ action: actionOf('create-and-do'), item: { title: 'about ${person} and ${id}' } })
    expect(sent[0].args?.title).toBe('about ${person} and ${id}')
  })

  test('the submit and an action naming no method run nothing here', async () => {
    const { sent, processor, actionOf } = setup()
    await processor!({ action: actionOf('submit'), item: { title: 'x' } })
    await processor!({ action: actionOf('nothing-to-run'), item: { title: 'x' } })
    await processor!({ action: actionOf('cancel') ?? { id: 'cancel', scope: 'item' }, item: { title: 'x' } })
    expect(sent.length).toBe(0)
  })

  test('a form with no extra action that names a method gets no processor', () => {
    const { runOf } = recorder()
    expect(formActionProcessor([{ type: 'ui-action', id: 'submit', scope: 'item' } as never], runOf)).toBeUndefined()
    expect(formActionProcessor(undefined, runOf)).toBeUndefined()
  })
})
