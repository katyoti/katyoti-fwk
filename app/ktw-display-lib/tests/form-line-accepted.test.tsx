// SPDX-License-Identifier: Apache-2.0
// A checked field's answer under its line: an accepted text green with a
// check icon, a derived field telling « rempli d'après le titre » or
// « modifié par vous », a note muted; a refusal stays the field's error.

import { beforeAll, describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIForms, UIFormView } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import type { UITexts } from '../src/ui-core/i18n/types.ts'
import fr from '../src/ui-core/action/i18n/fr.json'
import { byId, drawForm } from './_form-harness.tsx'

/** The form's texts read in the French resource, as the GUI loads it. */
const frTexts: UITexts = {
  language: 'fr',
  t(key, options) {
    const v = key.split('.').reduce<unknown>((at, k) => (at as Record<string, unknown> | undefined)?.[k], fr)
    const s = typeof v === 'string' ? v : (options?.defaultValue ?? key)
    return s.replace(/\{\{(\w+)\}\}/g, (_, k: string) => String(options?.[k] ?? ''))
  },
  withNamespace() { return frTexts },
}

const schema = {
  name: 'study', root: 'Study',
  types: { Study: { type: 'object', fields: { title: { type: 'string', label: 'Titre' }, code: { type: 'string', label: 'Code', optional: true } },
    extensions: [{ type: 'ui-form', name: 'default', groups: [
      { fields: ['title', { id: 'code', derive: { from: 'title', fromLabel: 'le titre' }, check: { uri: 'ktw-example:study/code-free', whenEmpty: true } }] },
    ] }] } },
} as unknown as Schema

const form = () => buildUIForms({ schema, texts: frTexts })['default']
const line = (html: string) => byId(html, 'line.code').slice(0, 1200)

describe('a checked field shows its answer on its line', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('an accepted text: green, a check icon, « rempli d’après le titre » while derived', () => {
    const html = drawForm(form(), { title: 'Prévoyance', code: 'prevoyance' }, null, undefined,
      { code: { tone: 'accepted', text: 'Libre', derived: 'derived' } })
    const l = line(html)
    expect(l).toContain('data-tone="accepted"')
    expect(l).toContain('data-line-icon="check"')
    expect(l).toContain('Libre')
    expect(l).toContain('teal')
    expect(l).toContain(' · rempli d&#x27;après le titre')
  })

  test('once the person edits it: « modifié par vous »', () => {
    const html = drawForm(form(), { title: 'Prévoyance', code: 'prev' }, null, undefined,
      { code: { tone: 'accepted', text: 'Libre', derived: 'edited' } })
    expect(line(html)).toContain(' · modifié par vous')
    expect(line(html)).not.toContain('rempli')
  })

  test('a note is muted, without the derived mark; nothing without an answer', () => {
    const html = drawForm(form(), { title: '', code: '' }, null, undefined,
      { code: { tone: 'note', text: "Vide : l'identifiant sera tiré du titre." } })
    const l = line(html)
    expect(l).toContain('data-tone="note"')
    expect(l).toContain('dimmed')
    expect(l).toContain('Vide : l&#x27;identifiant sera tiré du titre.')
    expect(l).not.toContain('data-line-icon')
    expect(byId(drawForm(form(), { title: 'x', code: 'x' }), 'line.code')).toBe('')
  })

  test('a refusal stays the field’s error, no line drawn', () => {
    const html = drawForm(form(), { title: 'x', code: 'pris' }, null, { code: 'Déjà pris.' })
    expect(html).toContain('Déjà pris.')
    expect(byId(html, 'line.code')).toBe('')
  })

  test('the form view provides the line answers around its fields', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(UIFormView, { form: form(), item: { title: '' }, isNew: true, onSubmit: () => {} })))
    expect(html).toContain('data-field-line="code"')
    expect(html).not.toContain('data-field-line="title"')
  })
})
