// SPDX-License-Identifier: Apache-2.0
// A form's own « Annuler » leaves the page's modal or drawer
// holding it (acme-da's « Nouvelle étude » form in « Mes études »' modal);
// a form given its own onCancel keeps it; elsewhere « Annuler » runs nothing.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { setRendererLookup, type FieldRendererFactory } from '../src/ui-builders/fields/index.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { DismissContext } from '../src/ui-builders/form/dismiss.ts'
import { UIFormView } from '../src/ui-builders/form/ui-form.view.tsx'
import type { UIActionProcessor } from '../src/ui-core/action/index.ts'
import type { Schema } from '../src/schema/index.ts'

const stubFactory: FieldRendererFactory = { view: () => () => null, form: () => () => null, header: () => () => null, column: () => () => null }
setRendererLookup(() => stubFactory)

const schema = {
  name: 'study-new',
  root: 'Study',
  types: {
    Study: {
      type: 'object',
      fields: { title: { type: 'string' } },
      extensions: [{ type: 'ui-form', name: 'default', actions: [
        { type: 'ui-action', id: 'cancel', scope: 'item', label: 'Annuler' },
        { type: 'ui-action', id: 'submit', scope: 'item', label: "Créer l'étude" },
      ] }],
    },
  },
} as unknown as Schema

/** The form drawn once, statically, under a dismiss (or none): the action
 *  processor it hands its layout, as a button of the form would call it. */
function processorOf(dismiss: (() => void) | undefined, onCancel?: () => void): UIActionProcessor {
  const built = buildUIForms({ schema })['default']
  let got: UIActionProcessor | undefined
  const form = { ...built, renderer: (p: { actionProcessor?: UIActionProcessor }) => { got = p.actionProcessor; return null } }
  const view = createElement(UIFormView as never, { form, isNew: true, hideTitle: true, ...(onCancel ? { onCancel } : {}) } as never)
  renderToStaticMarkup(createElement(MantineProvider, null, createElement(DismissContext.Provider, { value: dismiss }, view)))
  if (!got) throw new Error('the form handed its layout no action processor')
  return got
}

const cancel = { action: { id: 'cancel', scope: 'item' as const }, item: { title: 'Facturation' } }

describe('a form’s « Annuler »', () => {
  test('leaves the modal or the drawer holding it', async () => {
    let closed = 0
    await processorOf(() => { closed++ })(cancel as never)
    expect(closed).toBe(1)
  })

  test('a form given its own onCancel keeps it', async () => {
    let closed = 0
    let own = 0
    await processorOf(() => { closed++ }, () => { own++ })(cancel as never)
    expect(own).toBe(1)
    expect(closed).toBe(0)
  })

  test('outside a modal or a drawer it runs nothing', async () => {
    await processorOf(undefined)(cancel as never)
  })

  test('another action of the form never leaves it', async () => {
    let closed = 0
    await processorOf(() => { closed++ })({ action: { id: 'submit', scope: 'item' }, item: { title: 'x' } } as never)
    expect(closed).toBe(0)
  })
})
