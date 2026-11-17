// SPDX-License-Identifier: Apache-2.0
// A form saved at each change (`save: change`) draws no « Enregistrer »,
// « Annuler » nor « Réinitialiser », and its list fields speak the GUI's
// language: a sub-object's fields under their declared labels (« Élément »,
// not « Item »), « Ajouter » and the empty line in French.

import { afterAll, beforeAll, describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import i18next from 'i18next'

import { formActionsOf } from '../src/ui-builders/form/form-line.ts'
import { buildUIForms, UIFormView } from '../src/ui-builders/form/index.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import { initI18n } from '../src/ui-core/i18n/index.ts'
import type { Schema } from '../src/schema/index.ts'
import type { UIFormDef } from '../src/ui-builders/form/ui-form.schema.ts'

const schema = (save?: 'change') => ({
  name: 'scope', version: 1, root: 'Scope',
  types: {
    Scope: {
      type: 'object', idField: ['title'], titleField: ['title'],
      fields: {
        title: { type: 'string', label: 'Titre' },
        in: { type: 'array', items: 'ScopeItem', label: 'Inclus', optional: true },
        out: { type: 'array', items: 'ScopeItem', label: 'Exclu', optional: true },
      },
      extensions: [{ type: 'ui-form', name: 'default', ...(save ? { save } : {}), groups: [{ title: 'Périmètre', fields: ['title', 'in', 'out'] }] }],
    },
    ScopeItem: {
      type: 'object',
      fields: {
        item: { type: 'string', label: 'Élément', optional: true },
        why: { type: 'string', label: 'Pourquoi inclus', optional: true },
      },
    },
  },
}) as unknown as Schema

const ids = (save?: 'change') => buildUIForms({ schema: schema(save) })['default'].actions.map((a) => a.id)

describe('a form saved at each change', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })
  afterAll(async () => { await i18next.changeLanguage('en') })

  test('keeps only the actions that are not its save bar', () => {
    const acts = [{ id: 'submit' }, { id: 'cancel' }, { id: 'reset' }, { id: 'delete' }, { id: 'publish' }]
    const def = { type: 'ui-form', save: 'change' } as UIFormDef
    expect(formActionsOf(def, acts).map((a) => a.id)).toEqual(['delete', 'publish'])
    expect(formActionsOf({ type: 'ui-form' } as UIFormDef, acts)).toHaveLength(5)
  })

  test('is built with no submit, cancel nor reset; a form saved on submit keeps them', () => {
    expect(ids('change')).not.toContain('submit')
    expect(ids('change')).not.toContain('cancel')
    expect(ids('change')).not.toContain('reset')
    expect(ids()).toEqual(expect.arrayContaining(['submit', 'cancel', 'reset']))
  })

  test('draws in French no save bar, its list rows under their declared labels', async () => {
    await initI18n({ language: 'fr' })
    await i18next.changeLanguage('fr')
    const form = buildUIForms({ schema: schema('change') })['default']
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(UIFormView, { form, item: { title: 'Facturation', in: [{ item: 'AP0042' }] }, onSubmit: () => {} })))
    for (const gone of ['Enregistrer', 'Annuler', 'Réinitialiser', '>Item<', '>Why<', '>Add<', 'No entries yet']) {
      expect(html).not.toContain(gone)
    }
    expect(html).toContain('Élément')
    expect(html).toContain('Pourquoi inclus')
    expect(html).toContain('Ajouter')
    expect(html).toContain('Aucune entrée')
  })
})
