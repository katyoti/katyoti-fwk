// SPDX-License-Identifier: Apache-2.0
// A read row (ui-form `edit: row`) whose trailing control does more than
// open the field's editor: a field `action` - the pencil runs a page action
// (its `${field}` templates read in the form's values), disabled where the
// page cannot run it -, a field `button` - a button in place of the pencil,
// its method's answer drawn in the row with the check grammar -, and field
// `choices` - the options a host method answers, asked again when a field
// its args name changes, the declared ones kept when the call fails.

import { beforeAll, describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import { FormProvider, useForm } from 'react-hook-form'

import { RowEditingContext } from '../src/ui-builders/fields/editing.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { createRowRenderers } from '../src/ui-builders/fields/index.ts'
import { argFields, askChoices, choicesList, pressButton, rowAction } from '../src/ui-builders/form/form-line.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import { RowAnswer } from '../src/ui-builders/form/layouts/_shared/read-row.tsx'
import { DataContext, type PageContext } from '../src/ui-builders/page/page-context.tsx'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'
import type { Schema } from '../src/schema/index.ts'
import type { UIFieldDef } from '../src/ui-builders/fields/types.ts'
import { byId } from './_form-harness.tsx'

const fields: UIFieldDef[] = [
  { id: 'keywords', action: { navigate: { object: '${holder}', panel: 'keywords' } } },
  { id: 'conn', button: { label: 'Tester', uri: 'ktw-example:/test', args: { holder: '${holder}' } } },
  { id: 'space', choices: { uri: 'ktw-example:/spaces', args: { conn: '${conn}' } }, options: [{ value: 'ARCH', label: 'Architecture' }] },
  'holder',
]

const schema = {
  name: 'src', version: 1, root: 'Src',
  types: {
    Src: {
      type: 'object', idField: ['holder'],
      fields: {
        holder: { type: 'string', label: 'Porteur' },
        keywords: { type: 'string', label: "Cherche d'après" },
        conn: { type: 'string', label: 'Connexion' },
        space: { type: 'string', label: 'Espace' },
      },
      extensions: [{ type: 'ui-form', name: 'default', save: 'change', groups: [{ title: 'Réglages', edit: 'row', fields }] }],
    },
  },
} as unknown as Schema

const values = { holder: 'demo.F-001', keywords: 'SAP', conn: 'prod', space: 'ARCH' }

function Harness({ page, host, editing }: { page: PageContext; host: HostServices; editing: string | null }) {
  const methods = useForm({ defaultValues: values })
  const body = buildUIForms({ schema })['default'].renderer({ isNew: false, isSubmitting: false, item: values })
  return createElement(FormProvider, methods as never,
    createElement(HostServicesContext.Provider, { value: host },
      createElement(DataContext.Provider, { value: page },
        createElement(RowEditingContext.Provider, { value: { editing, setEditing: () => {} } }, body))))
}

const draw = (page: PageContext = {}, host: HostServices = {}, editing: string | null = null) =>
  renderToStaticMarkup(createElement(MantineProvider, null, createElement(Harness, { page, host, editing })))

const disabled = (html: string, id: string) => /^<button[^>]*disabled=""/.test(byId(html, id))

describe('read rows that act', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('a field action keeps the pencil, runs the action with its templates read in the form values, disabled without a runner', () => {
    expect(byId(draw(), 'form.row.keywords.edit')).toContain('data-row-action=""')
    expect(disabled(draw(), 'form.row.keywords.edit')).toBe(true)
    const runner: PageContext = { onAction: () => {}, hasHost: true }
    expect(disabled(draw(runner), 'form.row.keywords.edit')).toBe(false)
    expect(rowAction((fields[0] as UIFieldDef).action!, values).navigate)
      .toEqual({ object: 'demo.F-001', panel: 'keywords' })
    // A tab action runs on a page with no host: the page shows its own tabs.
    expect(rowAction({ tab: 'take' }, values)).toEqual({ tab: 'take' })
  })

  test('a field button replaces the pencil; its answer is read with the check grammar', async () => {
    const html = draw({}, { callMethod: async () => ({}) })
    expect(byId(html, 'form.row.conn.button')).toContain('Tester')
    // Mock-up v6 draws the row button quiet (transparent, muted), never a bordered button.
    expect(byId(html, 'form.row.conn.button')).toMatch(/^<button[^>]*data-variant="subtle"/)
    expect(html).not.toContain('data-testid="form.row.conn.edit"')
    expect(disabled(html, 'form.row.conn.button')).toBe(false)
    expect(disabled(draw(), 'form.row.conn.button')).toBe(true)

    let asked: unknown
    const call = (answer: unknown) => async (_uri: string, input?: unknown) => { asked = input; return answer }
    expect(await pressButton(call({ ok: false, message: 'Jeton refusé' }), fields[1].button, 'prod', values)).toEqual({ tone: 'refused', text: 'Jeton refusé' })
    expect(asked).toEqual({ holder: 'demo.F-001', value: 'prod' })
    expect(await pressButton(call({ ok: true, text: 'Connexion établie' }), fields[1].button, 'prod', values)).toEqual({ tone: 'accepted', text: 'Connexion établie' })
    expect(await pressButton(call({ text: '3 espaces' }), fields[1].button, 'prod', values)).toEqual({ tone: 'note', text: '3 espaces' })
    expect(await pressButton(async () => { throw new Error('hors ligne') }, fields[1].button, 'prod', values)).toEqual({ tone: 'refused', text: 'hors ligne' })

    const line = (a: Parameters<typeof RowAnswer>[0]['answer']) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(RowAnswer, { id: 'form.row.conn.answer', answer: a })))
    expect(line({ tone: 'refused', text: 'Jeton refusé' })).toContain('data-tone="refused"')
    expect(line({ tone: 'accepted', text: 'Connexion établie' })).toContain('data-tone="accepted"')
    expect(line({ tone: 'note', text: '3 espaces' })).toContain('data-tone="note"')
    expect(line(undefined)).not.toContain('form.row.conn.answer')
  })

  test("field choices list the method's options, are asked again when an arg field changes, and keep the declared ones on a failure", async () => {
    let asked: unknown
    const call = async (_uri: string, input?: unknown) => { asked = input; return { options: [{ value: 'OPS', label: 'Exploitation' }, 'DEV'] } }
    const got = await askChoices(call, fields[2].choices, values)
    expect(asked).toEqual({ conn: 'prod' })
    expect(got.options).toEqual([{ value: 'OPS', label: 'Exploitation' }, { value: 'DEV', label: 'DEV' }])
    expect(argFields(fields[2].choices!.args)).toEqual(['conn'])
    expect(choicesList({})).toBeUndefined()
    expect(await askChoices(async () => { throw new Error('connexion perdue') }, fields[2].choices, values)).toEqual({ error: 'connexion perdue' })
    expect(await askChoices(undefined, fields[2].choices, values)).toEqual({})

    // The value is read by the label the method answers; without an answer, the declared one.
    const row = createRowRenderers(schema, 'space', fields[2] as UIFieldDef, { t: (_k: string, o?: { defaultValue?: string }) => o?.defaultValue ?? '' } as never)
    const view = (r: typeof row, v: Record<string, unknown>) => renderToStaticMarkup(createElement(MantineProvider, null, r.view(v)))
    expect(view(row, { space: 'ARCH' })).toContain('Architecture')
    expect(view(row.withOptions!(got.options!), { space: 'OPS' })).toContain('Exploitation')
    // The form draws the row; its trailing control stays the pencil.
    expect(draw()).toContain('data-testid="form.row.space.edit"')
  })
})
