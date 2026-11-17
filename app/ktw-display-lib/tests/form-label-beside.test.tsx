// SPDX-License-Identifier: Apache-2.0
// A field's description beside its label (`descriptionAt: beside`):
// « Code — facultatif · … » on one line, the editor below without a label
// of its own; the default keeps it below the label.

import { beforeAll, describe, expect, test } from 'bun:test'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { byId, drawForm } from './_form-harness.tsx'

const schema = (at?: string) => ({
  name: 'study', root: 'Study',
  types: { Study: { type: 'object', fields: {
    code: { type: 'string', label: 'Code', description: 'facultatif · rempli du titre', optional: true },
    keywords: { type: 'string', label: 'Mots-clés', description: 'le contexte de l étude', optional: true },
  }, extensions: [{ type: 'ui-form', name: 'default', groups: [{ fields: [{ id: 'code', ...(at ? { descriptionAt: at } : {}) }, 'keywords'] }] }] } },
}) as unknown as Schema

const draw = (at?: string) => drawForm(buildUIForms({ schema: schema(at) })['default'], { code: '', keywords: '' })

describe('a description beside the label', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('beside: the label line carries the description; the editor has no label of its own', () => {
    const html = draw('beside')
    const label = byId(html, 'label.code')
    const line = label.slice(0, label.indexOf('</p>'))
    expect(line).toContain('Code')
    expect(line).toContain(' — facultatif · rempli du titre')
    expect(line).toContain('dimmed')
    // The input's own Mantine label and description are gone.
    const field = html.slice(html.indexOf('data-field-line="code"'), html.indexOf('data-testid="input.code"'))
    expect(field).not.toContain('mantine-InputWrapper-label')
    expect(field).not.toContain('mantine-InputWrapper-description')
    // Per field: the other keeps its description below.
    expect(byId(html, 'label.keywords')).toBe('')
    expect(html).toContain('le contexte de l étude')
  })

  test('default: the description below the label, no label line drawn', () => {
    const html = draw()
    expect(byId(html, 'label.code')).toBe('')
    expect(html).not.toContain('data-field-line="code"')
    expect(html).toContain('mantine-InputWrapper-description')
  })
})
