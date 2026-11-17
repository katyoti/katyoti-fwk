// SPDX-License-Identifier: Apache-2.0
// A select whose options carry a description shows the picked option's
// description under it, following each change.

import { beforeAll, describe, expect, test } from 'bun:test'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { pickedOption } from '../src/ui-builders/fields/rich-options.tsx'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { byId, drawForm } from './_form-harness.tsx'

const options = [
  { value: 'standard', label: 'Standard — 4 sections', description: 'Contexte · Applications · Flux · Risques' },
  { value: 'light', label: 'Allégé — 2 sections', description: 'Contexte · Applications' },
]
const schema = (opts: unknown[]) => ({
  name: 'study', root: 'Study',
  types: { Study: { type: 'object', fields: {
    template: { type: 'string', label: 'Modèle', enum: ['standard', 'light'], content: { type: 'text', options: opts } },
  }, extensions: [{ type: 'ui-form', name: 'default', groups: [{ fields: ['template'] }] }] } },
}) as unknown as Schema

const form = (opts: unknown[] = options) => buildUIForms({ schema: schema(opts) })['default']
const picked = (html: string) => { const p = byId(html, 'picked.template'); return p.slice(0, p.indexOf('</p>')) }

describe('the picked option described under a select', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('pickedOption finds the option a value holds', () => {
    expect(pickedOption(options, 'light')?.label).toBe('Allégé — 2 sections')
    expect(pickedOption(options, '')).toBeUndefined()
    expect(pickedOption(options, 'other')).toBeUndefined()
    expect(pickedOption([{ value: 2, label: 'Deux' }], '2')?.label).toBe('Deux')
  })

  test('the description of the option picked, under the select, following the value', () => {
    expect(picked(drawForm(form(), { template: 'standard' }))).toContain('Contexte · Applications · Flux · Risques')
    const light = picked(drawForm(form(), { template: 'light' }))
    expect(light).toContain('Contexte · Applications')
    expect(light).not.toContain('Flux')
    expect(light).toContain('dimmed')
  })

  test('nothing when none is picked or the options carry no description', () => {
    expect(byId(drawForm(form(), { template: '' }), 'picked.template')).toBe('')
    expect(byId(drawForm(form([{ value: 'standard', label: 'Standard' }, { value: 'light', label: 'Allégé' }]), { template: 'light' }), 'picked.template')).toBe('')
  })
})
