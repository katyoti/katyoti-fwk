// SPDX-License-Identifier: Apache-2.0
// A ui-form group declared `edit: row` draws its fields as
// read rows - label and hint, the value read, a pencil per row (none on a
// readOnly field); the pencil opens that field's editor with OK / Annuler,
// Annuler puts the value back, and `save: change` waits while a row is open.

import { beforeAll, describe, expect, test } from 'bun:test'

import { readRowActions } from '../src/ui-builders/fields/editing.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { shouldSave } from '../src/ui-builders/form/form-line.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { byId, drawForm, liveForm } from './_form-harness.tsx'
import { NoteLine } from '../src/ui-builders/form/layouts/_shared/read-row.tsx'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

const schema = (edit: 'group' | 'form' | 'none' = 'group') => ({
  name: 'ctx', version: 1, root: 'Ctx',
  types: {
    Ctx: {
      type: 'object', idField: ['code'], titleField: ['title'],
      fields: {
        title: { type: 'string', label: 'Titre' },
        code: { type: 'string', label: 'Code' },
        kw: { type: 'array', items: 'string', label: 'Mots-clés', optional: true },
      },
      extensions: [{ type: 'ui-form', name: 'default', save: 'change', ...(edit === 'form' ? { edit: 'row' } : {}), groups: [{
        title: 'Général', ...(edit === 'group' ? { edit: 'row' } : {}),
        fields: ['title', { id: 'code', readOnly: true, description: 'fixé à la création' }, { id: 'kw', description: 'les sources proposent', note: { uri: 'ktw-example:/kw-hint' } }],
      }] }],
    },
  },
}) as unknown as Schema

const values = { title: 'Facturation', code: 'facturation', kw: ['SAP', 'ERP'] }
const form = (edit?: 'group' | 'form' | 'none') => buildUIForms({ schema: schema(edit) })['default']

describe('a form group read as rows', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('draws each field as a row: label, hint, the value read, a pencil - none on a readOnly field', () => {
    const html = drawForm(form(), values)
    expect(html).toContain('data-form-rows=""')
    for (const f of ['title', 'code', 'kw']) expect(html).toContain(`data-testid="form.row.${f}"`)
    expect(html).toContain('>Titre<')
    expect(html).toContain('fixé à la création')
    expect(html).toContain('Facturation')
    expect(html).toContain('SAP')
    expect(html).toContain('ERP')
    expect(html).toContain('data-testid="form.row.title.edit"')
    expect(html).toContain('data-testid="form.row.kw.edit"')
    expect(html).not.toContain('data-testid="form.row.code.edit"')
    expect(html).not.toMatch(/<input[^>]*value="Facturation"/)
  })

  test('a form declared edit: row reads every group as rows; undeclared, the fields are inputs as before', () => {
    expect(drawForm(form('form'), values)).toContain('data-testid="form.row.title.edit"')
    const plain = drawForm(form('none'), values)
    expect(plain).not.toContain('form.row.')
    expect(plain).toMatch(/<input[^>]*value="Facturation"/)
  })

  test('an open row draws its editor with OK / Annuler, and the other pencils wait', () => {
    const html = drawForm(form(), values, 'row:title')
    expect(byId(html, 'form.row.title')).toContain('data-editing="true"')
    expect(html).toMatch(/<input[^>]*value="Facturation"/)
    expect(html).toContain('data-testid="form.row.title.ok"')
    expect(html).toContain('data-testid="form.row.title.cancel"')
    expect(html).not.toContain('data-testid="form.row.title.edit"')
    expect(byId(html, 'form.row.kw.edit')).toMatch(/^<button[^>]*disabled=""/)
  })

  test('OK is disabled while the open field shows an error on its line', () => {
    const html = drawForm(form(), values, 'row:title', { title: 'déjà pris' })
    expect(byId(html, 'form.row.title.ok')).toMatch(/^<button[^>]*disabled=""/)
  })

  test('the pencil keeps a snapshot, Annuler puts it back, OK keeps the change', () => {
    const { formControl } = liveForm({ ...values })
    let editing: string | null = null
    const snap = { current: undefined as unknown }
    const act = readRowActions(formControl, (k) => { editing = k }, 'kw', snap)
    act.edit()
    expect(editing).toBe('row:kw')
    formControl.setValue('kw', ['SAP'], { shouldDirty: true })
    act.cancel()
    expect(editing).toBeNull()
    expect(formControl.getValues('kw')).toEqual(['SAP', 'ERP'])
    act.edit()
    formControl.setValue('kw', ['SAP', 'ERP', 'CRM'], { shouldDirty: true })
    act.ok()
    expect(editing).toBeNull()
    expect(formControl.getValues('kw')).toEqual(['SAP', 'ERP', 'CRM'])
  })

  test('save: change waits while a row is open, and saves once it is closed', () => {
    expect(shouldSave({ title: 'x' }, '{}', true, 0, true)).toBe(false)
    expect(shouldSave({ title: 'x' }, '{}', true, 0, false)).toBe(true)
  })

  test('a field note shows under the open editor only, nothing without a text', () => {
    const line = (text: string | undefined) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(NoteLine, { id: 'form.row.kw.note', text })))
    expect(line('Deux pages trouvées.')).toMatch(/data-testid="form\.row\.kw\.note"[^>]*>Deux pages trouvées\.</)
    expect(line(undefined)).not.toContain('form.row.kw.note')
    expect(drawForm(form(), values)).not.toContain('form.row.kw.note')
  })
})

// A field's `read` draws the closed row's value: a worded template, a tone
// pill with a muted relative note, a brand mark, a muted lead before chips.
const readSchema = () => ({
  name: 'src', version: 1, root: 'Src',
  types: {
    Conn: { type: 'object', fields: { state: { type: 'string' }, tested_at: { type: 'string' } } },
    Src: {
      type: 'object', idField: ['name'], titleField: ['name'],
      fields: {
        name: { type: 'string', label: 'Nom' },
        reads: { type: 'string', label: 'Ce qu\'elle lit' },
        max: { type: 'number', label: 'Propositions', optional: true },
        kw: { type: 'array', items: 'string', label: 'Cherche d\'après', optional: true },
        conn: { type: 'Conn', label: 'Connexion', optional: true },
      },
      extensions: [{ type: 'ui-form', name: 'default', save: 'change', edit: 'row', groups: [{
        title: 'Réglages',
        fields: [
          { id: 'reads', readOnly: true, read: { mark: { text: 'Example Repo', color: '#2b6cb0' } } },
          { id: 'kw', read: { lead: 'les mots-clés de l\'étude' } },
          { id: 'max', read: { template: 'Au plus {value} applications' } },
          { id: 'conn', readOnly: true, read: { pill: { path: 'state', tones: { Joignable: 'teal', Injoignable: 'red' } }, after: { path: 'tested_at', as: 'relative-time', prefix: 'testée' } } },
        ],
      }] }],
    },
  },
}) as unknown as Schema

describe('a read row drawn as its field declares', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })
  const readForm = () => buildUIForms({ schema: readSchema() })['default']
  const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString()
  const src = { name: 'repo', reads: 'Example Repo', max: 20, kw: ['SAP', 'ERP'], conn: { state: 'Injoignable', tested_at: fiveMinAgo } }

  test('a template words the value; the editor behind the pencil stays a number', () => {
    expect(byId(drawForm(readForm(), src), 'form.row.max')).toContain('Au plus 20 applications')
    const open = drawForm(readForm(), src, 'row:max')
    expect(byId(open, 'form.row.max')).not.toContain('Au plus')
    expect(open).toMatch(/<input[^>]*value="20"/)
  })

  test('an empty value reads plain, never an empty template', () => {
    expect(drawForm(readForm(), { ...src, max: undefined })).not.toContain('Au plus')
  })

  test('a pill takes its tone from the word, a muted relative note after it', () => {
    const row = byId(drawForm(readForm(), src), 'form.row.conn')
    expect(byId(row, 'form.row.conn.pill')).toMatch(/data-tone="red"[^>]*>[\s\S]*Injoignable/)
    expect(byId(row, 'form.row.conn.after')).toContain('testée')
    expect(byId(row, 'form.row.conn.after')).toMatch(/5\s*min/)
    expect(byId(drawForm(readForm(), { ...src, conn: { state: 'Inconnu' } }), 'form.row.conn.pill')).toContain('data-tone="gray"')
  })

  test('a brand mark sits before the value, a lead before the chips', () => {
    const reads = byId(drawForm(readForm(), src), 'form.row.reads')
    expect(reads.indexOf('form.row.reads.mark')).toBeGreaterThan(-1)
    expect(byId(reads, 'form.row.reads.mark')).toMatch(/>ER</)
    expect(reads.indexOf('form.row.reads.mark')).toBeLessThan(reads.indexOf('Example Repo<'))
    const kw = byId(drawForm(readForm(), src), 'form.row.kw')
    expect(kw).toContain('les mots-clés de l&#x27;étude')
    expect(kw.indexOf('form.row.kw.lead')).toBeLessThan(kw.indexOf('SAP'))
  })
})
