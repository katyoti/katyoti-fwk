// SPDX-License-Identifier: Apache-2.0
// A ui-form group field may name a nested field by its dotted path
// (`brief.summary`): it is drawn as its own read row - the leaf's label, the
// value at the path - and a nested array (`plan.steps`) as its in-place
// table; editing the row changes that sub-field only.

import { beforeAll, describe, expect, test } from 'bun:test'

import { readRowActions, tabInError, valueAt } from '../src/ui-builders/fields/editing.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import { SchemaQuery, type Schema } from '../src/schema/index.ts'
import { byId, drawForm, liveForm } from './_form-harness.tsx'

const schema = {
  name: 'note', version: 1, root: 'Note',
  types: {
    Note: {
      type: 'object', idField: ['code'], titleField: ['code'],
      fields: {
        code: { type: 'string', label: 'Code' },
        brief: { type: 'Brief', label: 'Résumé', optional: true },
        plan: { type: 'Plan', label: 'Plan', optional: true },
      },
      extensions: [{ type: 'ui-form', name: 'default', groups: [{
        title: 'Général', edit: 'row', fields: ['brief.summary', 'plan.steps'],
      }] }],
    },
    Brief: { type: 'object', fields: {
      summary: { type: 'string', label: 'En une phrase', optional: true },
      problem: { type: 'string', label: 'Le problème', optional: true },
    } },
    Plan: { type: 'object', fields: {
      steps: { type: 'array', items: 'Step', content: 'table', label: 'Étapes', optional: true },
    } },
    Step: { type: 'object', fields: {
      what: { type: 'string', label: 'Quoi', optional: true },
      when: { type: 'string', label: 'Quand', optional: true },
    } },
  },
} as unknown as Schema

const values = {
  code: 'n1',
  brief: { summary: 'Ranger le grenier', problem: 'Trop de cartons' },
  plan: { steps: [{ what: 'Trier', when: 'lundi' }, { what: 'Donner', when: 'mardi' }] },
}
const form = () => buildUIForms({ schema })['default']

describe('a nested field drawn as its own row or table', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('the path resolves to the leaf field', () => {
    expect(SchemaQuery.getFieldInfoAt(schema, undefined, 'brief.summary')?.fieldType).toMatchObject({ label: 'En une phrase' })
    expect(SchemaQuery.getFieldInfoAt(schema, undefined, 'brief.nothing')).toBeUndefined()
    expect(valueAt(values, 'plan.steps')).toHaveLength(2)
  })

  test('the row shows the leaf label and the value at the path; the nested array is its table', () => {
    const html = drawForm(form(), values)
    const row = byId(html, 'form.row.brief.summary')
    expect(row).toContain('>En une phrase<')
    expect(row).toContain('Ranger le grenier')
    expect(html).not.toContain('Trop de cartons')
    expect(html).toContain('data-testid="form.table.plan.steps"')
    expect(html).toContain('data-testid="form.table.plan.steps.row.1"')
    expect(html).toContain('Donner')
    expect(html).toContain('>Étapes<')
  })

  test('the open row edits the sub-field only', () => {
    const html = drawForm(form(), values, 'row:brief.summary')
    expect(html).toMatch(/<input[^>]*value="Ranger le grenier"/)
    const { formControl } = liveForm(structuredClone(values))
    let editing: string | null = null
    const act = readRowActions(formControl, (k) => { editing = k }, 'brief.summary', { current: undefined })
    act.edit()
    expect(editing).toBe('row:brief.summary')
    formControl.setValue('brief.summary', 'Vider la cave', { shouldDirty: true })
    act.ok()
    expect(formControl.getValues('brief')).toEqual({ summary: 'Vider la cave', problem: 'Trop de cartons' })
  })

  test('a tab marks the error of its nested field, not of its sibling', () => {
    expect(tabInError({ brief: { summary: { message: 'requis' } } }, ['brief.summary'])).toBe(true)
    expect(tabInError({ brief: { problem: { message: 'requis' } } }, ['brief.summary'])).toBe(false)
  })
})
