// SPDX-License-Identifier: Apache-2.0
// In a ui-form drawn as tabs, each tab shows after its title
// the number of rows of its array fields (nothing when there are none), and
// an alert mark instead when one of its fields shows an error.

import { beforeAll, describe, expect, test } from 'bun:test'

import { tabCount, tabInError } from '../src/ui-builders/fields/editing.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import { FormProvider } from 'react-hook-form'
import { TabMark } from '../src/ui-builders/form/layouts/tabs/renderer.tsx'
import { drawForm, liveForm } from './_form-harness.tsx'

const schema = {
  name: 'scope', version: 1, root: 'Scope',
  types: {
    Scope: {
      type: 'object', idField: ['per'], titleField: ['per'],
      fields: {
        per: { type: 'string', label: 'En une phrase' },
        scope_in: { type: 'array', items: 'Item', label: 'Inclus', optional: true },
        scope_out: { type: 'array', items: 'Item', label: 'Exclu', optional: true },
        risks: { type: 'array', items: 'Item', label: 'Risques', optional: true },
      },
      extensions: [{ type: 'ui-form', name: 'default', layout: 'tabs', groups: [
        { title: 'Général', fields: ['per'] },
        { title: 'Périmètre', fields: ['scope_in', 'scope_out'] },
        { title: 'Risques', fields: ['risks'] },
      ] }],
    },
    Item: { type: 'object', fields: { item: { type: 'string', label: 'Élément', optional: true }, why: { type: 'string', label: 'Pourquoi', optional: true } } },
  },
} as unknown as Schema

const values = { per: 'La facturation', scope_in: [{ item: 'AP1' }, { item: 'AP2' }], scope_out: [{ item: 'AP3' }], risks: [] }
const form = () => buildUIForms({ schema })['default']

describe('a tab counts its rows and marks its errors', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('tabCount sums the rows of the array fields; none or empty is nothing', () => {
    expect(tabCount(values, ['scope_in', 'scope_out'])).toBe(3)
    expect(tabCount(values, ['risks'])).toBeUndefined()
    expect(tabCount(values, [])).toBeUndefined()
  })

  test('tabInError says whether one of the fields shows an error, a nested one included', () => {
    expect(tabInError({ scope_out: { 1: { why: { message: 'requis' } } } }, ['scope_in', 'scope_out'])).toBe(true)
    expect(tabInError({ title: { message: 'x' } }, ['scope_in', 'scope_out'])).toBe(false)
    expect(tabInError({}, ['scope_in'])).toBe(false)
  })

  test('a tab with array fields shows their rows after its title; the others nothing', () => {
    const html = drawForm(form(), values)
    expect(html).toContain('data-testid="form.tab.1.count"')
    expect(html).toMatch(/data-testid="form\.tab\.1\.count"[^>]*>3</)
    expect(html).not.toContain('form.tab.0.count')
    expect(html).not.toContain('form.tab.2.count')
    expect(html).not.toContain('.alert"')
  })

  test('a tab with a field in error shows the alert mark instead of its count', () => {
    const html = drawForm(form(), values, null, { 'scope_out.0.why': 'Pourquoi exclu : requis.' })
    expect(html).toContain('data-testid="form.tab.1.alert"')
    expect(html).not.toContain('form.tab.1.count')
    expect(html).not.toContain('form.tab.2.alert')
  })

  test('a declared count names the list to count (a nested path), or none', () => {
    const dated = { ...values, dates: { marks: [{ item: 'M1' }, { item: 'M2' }] } }
    expect(tabCount(dated, ['scope_in'], 'dates.marks')).toBe(2)
    expect(tabCount(dated, ['scope_in', 'scope_out'], false)).toBeUndefined()
    expect(tabCount(dated, ['scope_in', 'scope_out'], undefined)).toBe(3)
    const declared = {
      ...schema,
      types: { ...schema.types, Scope: { ...schema.types.Scope,
        fields: { ...(schema.types.Scope as { fields: object }).fields, dates: { type: 'Dates', label: 'Dates', optional: true } },
        extensions: [{ type: 'ui-form', name: 'default', layout: 'tabs', groups: [
          { title: 'Général', fields: ['per'], count: 'dates.marks' },
          { title: 'Périmètre', fields: ['scope_in', 'scope_out'], count: false },
          { title: 'Risques', fields: ['risks'] },
        ] }] },
        Dates: { type: 'object', fields: { marks: { type: 'array', items: 'Item', label: 'Jalons', optional: true } } } },
    } as unknown as Schema
    const html = drawForm(buildUIForms({ schema: declared })['default'], dated)
    expect(html).toMatch(/data-testid="form\.tab\.0\.count"[^>]*>2</)
    expect(html).not.toContain('form.tab.1.count')
  })

  test('the count follows the live form values, not the item it was opened with', () => {
    const { formControl } = liveForm(structuredClone(values))
    const mark = () => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(FormProvider, { ...formControl, formState: formControl.control._formState },
        createElement(TabMark, { group: { fields: ['scope_in', 'scope_out'], arrayFields: ['scope_in', 'scope_out'] }, index: 1, values }))))
    expect(mark()).toMatch(/>3</)
    formControl.setValue('scope_in', [...values.scope_in, { item: 'AP4' }])
    expect(mark()).toMatch(/data-testid="form\.tab\.1\.count"[^>]*>4</)
    formControl.setValue('scope_out', [])
    expect(mark()).toMatch(/data-testid="form\.tab\.1\.count"[^>]*>3</)
  })
})
