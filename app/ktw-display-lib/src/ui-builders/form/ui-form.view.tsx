// SPDX-License-Identifier: Apache-2.0
// UIFormView — React component that renders a built UIForm.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/ui-form.view.tsx during
// gui-form Subtask 3 (2026-05-03). Adaptations vs the source:
//
//   - i18n: legacy `texts.detailTitle` / `texts.buttons.new` (text-bag
//     properties) → `texts.t('detailTitle', { defaultValue: 'Details' })`
//     / `texts.t('buttons.new', { defaultValue: 'New' })`.
//
//   - Imports: `@core/cpm-core/forms` → `'./_shared/forms.tsx'`,
//     `'../ui-action'` → `'../../ui-core/action/index.ts'`.
//
// The component reads the form's `getDefaultValue` for initial values,
// wires react-hook-form via `useForm`, mounts the optional toolbar above
// the rendered groups, and emits standard cancel/submit/reset actions
// through an action processor that delegates unknown action ids to the
// caller-provided `actionProcessor`.

import { Group, Stack, Title } from '@mantine/core'
import { useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import type { UIActionParams, UIActionProcessor } from '../../ui-core/action/index.ts'
import { RHFFormProvider, useForm, useFormContext } from './_shared/forms.tsx'
import { useHostServices } from '../../ui-core/host/index.tsx'
import { RowEditingContext, type RowEditing } from '../fields/editing.ts'
import { FieldLineContext } from '../fields/field-line.tsx'
import { argFields, asksCheck, checkAnswer, checkPath, lineAnswers, lineArgs, lineFields, nextDerived, savesOnChange, shouldSave } from './form-line.ts'
import type { UIFormDef } from './ui-form.schema.ts'
import { DismissContext } from './dismiss.ts'
import { formResetOptions } from './form-reset.ts'
import type { UIForm } from './ui-form.types.ts'

export type UIFormViewProps<T = any> = {
  form: UIForm<T>
  item?: T
  isNew?: boolean
  onSubmit?: (data: T) => Promise<void> | void
  onCancel?: () => void
  /** Fires on every field-value change. Receives the current full form
   *  values + RHF's dirty/valid flags. The host's UIFormHarness wires
   *  this to a `change` postMessage so the host sees live edits. */
  onChange?: (values: T, meta: { dirty: boolean; valid: boolean }) => void
  actionProcessor?: UIActionProcessor
  /** Suppress the form-level title + toolbar row. Used by surfaces that
   *  provide their own outer chrome (e.g. <UIWizardView> wraps the form
   *  with a Stepper + step-section title and doesn't want the redundant
   *  "Details : <rowTitle>" header). Default: false. */
  hideTitle?: boolean
  /** Replace the computed `Details : <rowTitle>` heading with custom
   *  content (icon + title + description, breadcrumbs, etc.) rendered
   *  inline in the title slot. The toolbar row still mounts to the
   *  right of it. Use this when the form is the only surface that
   *  identifies the entity — the caller drops their own outer header
   *  card and reuses this slot instead. */
  customTitle?: React.ReactNode
  /** Server-side validation errors keyed by field path (e.g. `"name"`,
   *  `"address.city"`). Applied via RHF `setError` whenever the map
   *  identity changes (previous external errors are cleared first), so
   *  substrate validators (SchemaService.ValidateAgainst) can surface
   *  per-field messages after a rejected submit. Paths that match no
   *  field are silently ignored here — hosts should render them in
   *  their own catch-all surface. */
  externalErrors?: Record<string, string>
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
  /** A new `item` keeps every field the person changed since and takes the
   *  rest (react-hook-form's keepDirtyValues, formResetOptions) - a page
   *  form's value pushed again, as a question asked again with its refused
   *  answer, never wipes what the person typed. Default: false (a new item
   *  resets the whole form). */
  keepEdits?: boolean
}

/**
 * UIFormView — renders a `UIForm` (built via `buildUIForm()`) into a
 * complete react-hook-form-backed form surface. Handles default-value
 * resolution, isNew toggling between create/edit, and submit/cancel/reset
 * action wiring on top of the user-supplied `actionProcessor`.
 */
export function UIFormView<T = any>({
  form,
  item,
  isNew = false,
  onSubmit,
  onCancel,
  onChange,
  actionProcessor,
  hideTitle = false,
  customTitle,
  externalErrors,
  attrs,
  keepEdits = false,
}: UIFormViewProps<T>) {
  const { getDefaultValue, texts } = form
  // « Annuler » with no onCancel leaves the modal or the drawer holding the
  // form.
  const dismiss = useContext(DismissContext)
  const cancel = onCancel ?? dismiss

  const memoDefaultValues = useMemo(() => getDefaultValue(item), [getDefaultValue, item])

  // The line open for editing - a read row, a table row:
  // one at a time, and `save: change` waits while it is open.
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const rowEditing = useMemo<RowEditing>(() => ({ editing: editingKey, setEditing: setEditingKey }), [editingKey])

  const resetOptions = formResetOptions(keepEdits)
  const methods = useForm({
    values: item as any,
    defaultValues: memoDefaultValues as any,
    resetOptions,
  })

  const title = useMemo(() => {
    const detailTitle = texts.t('detailTitle', { defaultValue: 'Details' })
    const newLabel = texts.t('buttons.new', { defaultValue: 'New' })
    // Discriminator: in create mode show the "New" placeholder. In
    // edit mode show the row title — but only when `item` actually has
    // a resolvable id (an empty `{}` is truthy and would stringify
    // missing fields as the literal "undefined"). Falls back to the
    // detailTitle alone when there's no item to title.
    if (isNew) return `${detailTitle} : ${newLabel}`
    if (item && form.rowId(item)) return `${detailTitle} : ${form.rowTitle(item)}`
    return detailTitle
  }, [form, item, isNew, texts])

  const {
    reset,
    watch,
    setError,
    clearErrors,
    formState: { isSubmitting, isDirty, isValid },
  } = methods

  // Apply server-side errors when the map changes. clearErrors() also
  // drops client-side errors, but RHF re-derives those on the next
  // interaction/submit, so the worst case is a momentarily-clean field.
  useEffect(() => {
    clearErrors()
    if (!externalErrors) return
    for (const [path, message] of Object.entries(externalErrors)) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setError(path as any, { type: 'server', message })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [externalErrors])

  const values = watch() as T

  // Reset form when isNew toggles or item identity changes (load new record),
  // keeping the person's edits when asked (keepEdits).
  useEffect(() => {
    reset(memoDefaultValues as any, resetOptions)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isNew, item])

  // Emit change events on every value mutation. `watch` already keeps
  // `values` reactive — JSON-stringifying it as the dependency lets React
  // skip emits when the shape didn't actually change (RHF's watch returns
  // a fresh object on every render even when content is identical).
  useEffect(() => {
    if (!onChange) return
    onChange(values, { dirty: isDirty, valid: isValid })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(values), isDirty, isValid])

  const handleSubmit = async (data: T) => {
    if (onSubmit) {
      await onSubmit(data)
    }
  }

  const enhancedActionProcessor = useMemo(() => {
    return async (params: UIActionParams) => {
      switch (params.action.id) {
        case 'cancel':
          cancel?.()
          break
        case 'submit':
          if (params.action.scope === 'item' && 'item' in params) {
            await handleSubmit(params.item as T)
          }
          break
        case 'reset':
          reset()
          break
      }
      await actionProcessor?.(params)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [actionProcessor, cancel])

  return (
    <RHFFormProvider methods={methods} onSubmit={handleSubmit} attrs={attrs}>
      <RowEditingContext.Provider value={rowEditing}>
      <FormLineBehaviours def={form.definition} onSave={onSubmit} editing={editingKey !== null}>
      <Stack gap="md">
        {!hideTitle && (
          <Group justify="space-between">
            {customTitle ?? <Title order={1}>{title}</Title>}
            {form.toolbar?.({
              isNew,
              isSubmitting,
              item: values,
              actionProcessor: enhancedActionProcessor,
            })}
          </Group>
        )}
        {form.renderer({
          isNew,
          isSubmitting,
          item: values,
          actionProcessor: enhancedActionProcessor,
        })}
      </Stack>
      </FormLineBehaviours>
      </RowEditingContext.Provider>
    </RHFFormProvider>
  )
}

/** FormLineBehaviours runs what a form does on a field's line while the
 *  person types (form-line.ts): a derived field follows its
 *  source until the person edits it, a checked field shows its host check's
 *  refusal as its error, an accepted text or a note under it (drawn by
 *  field-line.tsx through FieldLineContext, never the form's errors, so
 *  saving reads only refusals), and a form with `save: change` saves each change a
 *  moment after the typing stops, while no line shows an error and no line
 *  is open for editing. A check answer naming a `path` sets its message on
 *  that path (a table's cell) rather than on the field. */
function FormLineBehaviours<T>({ def, onSave, editing = false, children }: { def: UIFormDef; onSave?: (data: T) => Promise<void> | void; editing?: boolean; children?: ReactNode }) {
  const { watch, getValues, setValue, setError, clearErrors, getFieldState, formState } = useFormContext()
  const host = useHostServices()
  const fields = useMemo(() => lineFields(def), [def])
  const values = watch() as Record<string, unknown>
  const lastDerived = useRef<Record<string, string>>({})
  const lastSaved = useRef<string | undefined>(undefined)
  const errorAt = useRef<Record<string, string>>({})
  const [answers, setAnswers] = useState<Record<string, { tone: 'accepted' | 'note'; text: string }>>({})
  const answer = (name: string, a?: { tone: 'accepted' | 'note'; text: string }) => setAnswers((cur) => {
    const was = cur[name]
    if (!a && !was) return cur
    if (a && was && was.tone === a.tone && was.text === a.text) return cur
    const next = { ...cur }
    if (a) next[name] = a
    else delete next[name]
    return next
  })

  // A derived field follows its source while it holds what was derived last.
  const sources = JSON.stringify(fields.map((f) => (f.derive ? values[f.derive.from] : null)))
  useEffect(() => {
    for (const f of fields) {
      if (!f.derive) continue
      const next = nextDerived(getValues(f.name), lastDerived.current[f.name], getValues(f.derive.from), f.derive.as)
      if (next === undefined) continue
      lastDerived.current[f.name] = next
      setValue(f.name, next, { shouldDirty: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sources])

  // A checked field is asked a moment after the person stops typing in it
  // or in a field its args name (`${code}`).
  const checked = JSON.stringify(fields.map((f) => (f.check ? [values[f.name], ...argFields(f.check.args).map((n) => values[n])] : null)))
  useEffect(() => {
    const call = host.callMethod
    if (!call) return
    let live = true
    const timer = setTimeout(() => {
      for (const f of fields) {
        if (!f.check) continue
        const value = getValues(f.name)
        const clear = () => {
          const at = errorAt.current[f.name] ?? f.name
          if (getFieldState(at).error?.type === 'check') clearErrors(at)
          delete errorAt.current[f.name]
        }
        if (!asksCheck(f.check, value)) { clear(); answer(f.name); continue }
        call(f.check.uri, lineArgs(f.check.args, getValues() as Record<string, unknown>, value))
          .then((out) => {
            if (!live) return
            const a = checkAnswer(out)
            if (a?.tone !== 'refused') {
              clear()
              answer(f.name, a ? { tone: a.tone, text: a.text } : undefined)
              return
            }
            answer(f.name)
            const at = checkPath(out) ?? f.name
            if (errorAt.current[f.name] && errorAt.current[f.name] !== at) clear()
            errorAt.current[f.name] = at
            setError(at, { type: 'check', message: a.text })
          })
          .catch(() => { /* a check that cannot run leaves the line as it is */ })
      }
    }, 400)
    return () => { live = false; clearTimeout(timer) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [checked, host.callMethod])

  // `save: change`: each change saved once the typing stops.
  const saves = savesOnChange(def) && !!onSave
  const json = JSON.stringify(values)
  const lineErrors = Object.keys(formState.errors ?? {}).length
  useEffect(() => {
    if (!saves) return
    if (lastSaved.current === undefined && !formState.isDirty) { lastSaved.current = json; return }
    const timer = setTimeout(() => {
      if (!shouldSave(getValues(), lastSaved.current, formState.isDirty, lineErrors, editing)) return
      lastSaved.current = JSON.stringify(getValues())
      void onSave?.(getValues() as T)
    }, 600)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saves, json, lineErrors, formState.isDirty, editing])
  const lines = lineAnswers(fields, answers, values, lastDerived.current)
  return <FieldLineContext.Provider value={lines}>{children}</FieldLineContext.Provider>
}
