// SPDX-License-Identifier: Apache-2.0
// A form drawn statically for a test: its react-hook-form, the line open
// for editing preset (RowEditingContext) and errors set before it draws.

import { createElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import { createFormControl, FormProvider, useForm } from 'react-hook-form'

import { RowEditingContext } from '../src/ui-builders/fields/editing.ts'
import { FieldLineContext, type FieldLineAnswer } from '../src/ui-builders/fields/field-line.tsx'
import type { UIForm } from '../src/ui-builders/form/ui-form.types.ts'

type HarnessProps = { form: UIForm; values: Record<string, unknown>; editing: string | null; errors?: Record<string, string>; lines?: Record<string, FieldLineAnswer> }

function Harness({ form, values, editing, errors, lines }: HarnessProps) {
  const methods = useForm({ defaultValues: values, errors: nested(errors) })
  const body: ReactNode = form.renderer({ isNew: false, isSubmitting: false, item: values })
  return createElement(FormProvider, methods as never,
    createElement(RowEditingContext.Provider, { value: { editing, setEditing: () => {} } },
      createElement(FieldLineContext.Provider, { value: lines ?? {} }, body)))
}

/** Errors by dotted path (`risks.2.mitigation`) as react-hook-form holds them. */
function nested(errors: Record<string, string> | undefined): Record<string, unknown> | undefined {
  if (!errors) return undefined
  const out: Record<string, unknown> = {}
  for (const [path, message] of Object.entries(errors)) {
    const keys = path.split('.')
    let at = out
    for (const k of keys.slice(0, -1)) at = (at[k] ??= {}) as Record<string, unknown>
    at[keys[keys.length - 1]] = { type: 'check', message }
  }
  return out
}

/** A form's control outside React, mounted (its values read live). */
export function liveForm(values: Record<string, unknown>): { formControl: any } {
  const r = createFormControl({ defaultValues: values }) as unknown as { formControl: any; control: { _state: { mount: boolean } } }
  r.control._state.mount = true
  return r
}

/** Draws a form; `lines` are the line answers its checked fields show
 *  (FieldLineContext, as the form's line behaviours provide them). */
export function drawForm(form: UIForm, values: Record<string, unknown>, editing: string | null = null, errors?: Record<string, string>, lines?: Record<string, FieldLineAnswer>): string {
  return renderToStaticMarkup(createElement(MantineProvider, null, createElement(Harness, { form, values, editing, errors, lines })))
}

/** The markup from the element carrying data-testid `id` on (its opening tag first). */
export function byId(html: string, id: string): string {
  const at = html.indexOf(`data-testid="${id}"`)
  return at < 0 ? '' : html.slice(html.lastIndexOf('<', at))
}
