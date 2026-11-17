// SPDX-License-Identifier: Apache-2.0
// An array field with content `table` is edited in a form as
// a table - its rows read with a pencil each, the open row's cells inputs
// with OK / Annuler, « Ajouter » appending a row opened for editing, a
// delete per row; the field's check message lands on its row and cell when
// the check answers a `path`, under the table when not. A string array is
// one column.

import { beforeAll, describe, expect, test } from 'bun:test'

import { cellError, listError, rowError, tableRowActions } from '../src/ui-builders/fields/editing.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { checkPath } from '../src/ui-builders/form/form-line.ts'
import { getInfo } from '../src/ui-builders/fields/info.ts'
import { resolveFieldOptions, toSelectData } from '../src/ui-builders/fields/rich-options.tsx'
import { tableEditorColumns } from '../src/ui-builders/fields/table/editor.tsx'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { byId, drawForm, liveForm } from './_form-harness.tsx'

const schema = {
  name: 'scope', version: 1, root: 'Scope',
  types: {
    Scope: {
      type: 'object', idField: ['per'], titleField: ['per'],
      fields: {
        per: { type: 'string', label: 'En une phrase' },
        risks: { type: 'array', items: 'Risk', label: 'Risques', optional: true, content: { type: 'table', extension: 'edit' } },
        milestones: { type: 'array', items: 'string', label: 'Jalons clés', optional: true, content: 'table' },
      },
      extensions: [{ type: 'ui-form', name: 'default', groups: [{ fields: ['risks', 'milestones'] }] }],
    },
    Risk: {
      type: 'object',
      fields: {
        id: { type: 'string', label: 'Id', optional: true },
        risk: { type: 'string', label: 'Risque', optional: true },
        mitigation: { type: 'string', label: 'Mitigation', optional: true },
        owner: { type: 'string', label: 'Porteur', optional: true },
        level: { type: 'string', enum: ['high', 'low'], label: 'Niveau', optional: true },
      },
      extensions: [{ type: 'ui-table', name: 'edit', columns: ['id', 'risk', 'mitigation',
        { name: 'level', options: [{ value: 'high', label: 'Élevée' }, { value: 'low', label: 'Faible' }] }] }],
    },
  },
} as unknown as Schema

const values = {
  per: 'x',
  risks: [{ id: 'R1', risk: 'Données manquantes', mitigation: 'Relance', owner: 'PO', level: 'high' }, { id: 'R2', risk: 'Retard', mitigation: '' }],
  milestones: ['Cadrage', 'Restitution'],
}
const form = () => buildUIForms({ schema })['default']

describe('an array of objects edited as a table', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('draws the rows read, its extension columns only, a pencil and a delete per row, « Ajouter »', () => {
    const html = drawForm(form(), values)
    const table = byId(html, 'form.table.risks')
    expect(table).toContain('>Risque<')
    expect(table).toContain('>Mitigation<')
    expect(table).not.toContain('>Porteur<')
    expect(table).toContain('Données manquantes')
    for (const i of [0, 1]) {
      expect(html).toContain(`data-testid="form.table.risks.row.${i}.edit"`)
      expect(html).toContain(`data-testid="form.table.risks.row.${i}.delete"`)
    }
    expect(html).toContain('data-testid="form.table.risks.add"')
    expect(html).not.toMatch(/<input[^>]*value="Données manquantes"/)
  })

  test('a string array with content table is one column', () => {
    const html = drawForm(form(), values)
    expect(byId(html, 'form.table.milestones')).toContain('Restitution')
    expect(html).toContain('data-testid="form.table.milestones.row.1.edit"')
  })

  test('the open row draws its cells as inputs with OK / Annuler; the other pencils and « Ajouter » wait', () => {
    const html = drawForm(form(), values, 'table:risks.0')
    expect(html).toMatch(/<input[^>]*value="Données manquantes"/)
    expect(html).toContain('data-testid="form.table.risks.row.0.ok"')
    expect(html).toContain('data-testid="form.table.risks.row.0.cancel"')
    expect(byId(html, 'form.table.risks.row.1.edit')).toMatch(/^<button[^>]*disabled=""/)
    expect(byId(html, 'form.table.risks.add')).toMatch(/^<button[^>]*disabled=""/)
  })

  test('a check answer with a path marks its row and cell, the row kept open', () => {
    expect(checkPath({ ok: false, message: 'm', path: 'risks[1].mitigation' })).toBe('risks.1.mitigation')
    expect(checkPath({ ok: false, message: 'm' })).toBeUndefined()
    const message = "Mitigation : requise. Rien n'est enregistré tant qu'elle manque."
    const html = drawForm(form(), values, null, { 'risks.1.mitigation': message })
    const row = byId(html, 'form.table.risks.row.1')
    expect(row).toMatch(/^<tr[^>]*data-editing="true"[^>]*data-error="true"/)
    expect(html).toContain('data-testid="form.table.risks.row.1.error"')
    expect(html).toContain('Mitigation : requise.')
    expect(byId(html, 'form.table.risks.row.1.ok')).toMatch(/^<button[^>]*disabled=""/)
    expect(byId(html, 'form.table.risks.row.0')).toMatch(/^<tr[^>]*data-editing="false"/)
    expect(html).not.toContain('data-testid="form.table.risks.error"')
  })

  test('a check answer without a path shows under the table', () => {
    const html = drawForm(form(), values, null, { risks: 'Au moins un risque.' })
    expect(html).toContain('data-testid="form.table.risks.error"')
    expect(html).not.toContain('row.0.error')
  })

  test('the error readers find the cell, the row and the table message', () => {
    const errors = { risks: { 1: { mitigation: { message: 'requise' } } }, milestones: { 0: { message: 'vide' } } }
    expect(cellError(errors, 'risks', 1, 'mitigation')).toBe('requise')
    expect(rowError(errors, 'risks', 1, ['id', 'mitigation'])).toEqual({ column: 'mitigation', message: 'requise' })
    expect(rowError(errors, 'risks', 0, ['id', 'mitigation'])).toBeUndefined()
    expect(cellError(errors, 'milestones', 0, '_')).toBe('vide')
    expect(listError({ risks: { message: 'x' } }, 'risks')).toBe('x')
    expect(listError(errors, 'risks')).toBeUndefined()
  })

  test('add appends a row opened for editing, Annuler removes it; edit + Annuler puts the row back; delete removes it', () => {
    const { formControl } = liveForm(structuredClone(values))
    let editing: string | null = null
    const set = (k: string | null) => { editing = k }
    const snap = { current: null as { index: number; row: unknown } | null }
    const act = () => tableRowActions(formControl, editing, set, 'risks', snap, () => ({ id: '', risk: '', mitigation: '' }))

    act().add()
    expect(editing).toBe('table:risks.2')
    expect(formControl.getValues('risks')).toHaveLength(3)
    act().cancel(2)
    expect(editing).toBeNull()
    expect(formControl.getValues('risks')).toHaveLength(2)

    act().edit(0)
    formControl.setValue('risks.0.risk', 'Autre', { shouldDirty: true })
    act().cancel(0)
    expect(formControl.getValues('risks')[0].risk).toBe('Données manquantes')

    act().edit(1)
    formControl.setValue('risks.1.mitigation', 'Planifier', { shouldDirty: true })
    act().ok()
    expect(formControl.getValues('risks')[1].mitigation).toBe('Planifier')

    act().remove(0)
    expect(formControl.getValues('risks').map((r: { id: string }) => r.id)).toEqual(['R2'])
  })

  test('an enum column with options is read and picked through their labels; the value stays the key', () => {
    const read = byId(drawForm(form(), values), 'form.table.risks.row.0')
    expect(read).toContain('Élevée')
    expect(read).not.toContain('>high<')
    const open = drawForm(form(), values, 'table:risks.0')
    expect(open).toMatch(/<input[^>]*value="Élevée"/)
    const level = tableEditorColumns(getInfo(schema, 'risks', { id: 'risks' })).find((c) => c.name === 'level')
    const leaf = getInfo(schema, 'level', { id: 'level', options: level?.options }, 'Risk')
    expect(toSelectData(resolveFieldOptions(leaf.contentType, ['high', 'low']))).toEqual([
      { value: 'high', label: 'Élevée' }, { value: 'low', label: 'Faible' }])
  })
})
