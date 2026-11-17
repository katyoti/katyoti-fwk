// SPDX-License-Identifier: Apache-2.0
// panel-ref content renderer — a reference to one panel of an object, its
// value `<id>#<panel>`.
//
// The form picks the object (ObjectPicker, filtered by the field's
// ObjectFilter) then one of its declared panels (host.objectPanels); the
// view and the column show it as a link opening the object on that panel.
// With no host the form is a text input (`<id>#<panel>`) and the value plain
// text.

import { Group, Select, TextInput } from '@mantine/core'
import { useEffect, useState } from 'react'
import { Controller, useFormContext } from 'react-hook-form'
import {
  columnStyles,
  columnText,
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { StringType } from '../../../schema/schema-types.ts'
import {
  refFilterOf, refTarget, useHostServices, type PanelChoice, type RefFilter,
} from '../../../ui-core/host/index.tsx'
import { ObjectPicker, RefLink } from '../object-ref/renderer.tsx'
import type { PanelRefContent } from './types.ts'

/** panelRef writes a panel reference: `<id>#<panel>`, or the id alone while
 *  no panel is chosen. */
export function panelRef(object: string | null, panel: string | null): string {
  if (!object) return ''
  return panel ? `${object}#${panel}` : object
}

/** PanelPicker picks an object, then one of its panels. */
function PanelPicker({ value, onChange, filter, label, description, required, disabled, error, testId }: {
  value: string
  onChange: (ref: string) => void
  filter: RefFilter
  label?: string
  description?: string
  required?: boolean
  disabled?: boolean
  error?: string
  testId?: string
}) {
  const host = useHostServices()
  const target = refTarget(value)
  const object = target?.object ?? null
  const [panels, setPanels] = useState<PanelChoice[]>([])
  useEffect(() => {
    if (!object || !host.objectPanels) { setPanels([]); return }
    let cancelled = false
    host.objectPanels(object).then((ps) => { if (!cancelled) setPanels(ps) }).catch(() => { if (!cancelled) setPanels([]) })
    return () => { cancelled = true }
  }, [object, host.objectPanels])
  const data = panels.map((p) => ({ value: p.id, label: p.name || p.id }))
  if (target?.panel && !data.some((d) => d.value === target.panel)) data.unshift({ value: target.panel, label: target.panel })
  return (
    <Group grow align="flex-start" gap="xs" data-testid={testId}>
      <ObjectPicker value={object} onChange={(id) => onChange(panelRef(id, null))} filter={filter}
        label={label} description={description} required={required} disabled={disabled} error={error} />
      <Select label="Panel" placeholder={object ? 'Choose a panel' : 'Choose the object first'}
        data={data} value={target?.panel ?? null} disabled={disabled || !object}
        onChange={(p) => onChange(panelRef(object, p))} />
    </Group>
  )
}

const view: ViewFieldRendererFactory<StringType, PanelRefContent> = ({ info }) => {
  return (item: JsonObject) => <RefLink value={item[info.name]} testId={'view.' + info.name} />
}

const form: FormFieldRendererFactory<StringType, PanelRefContent> = ({ def, info }) => {
  const filter = refFilterOf(info.contentType as unknown as Record<string, unknown>)
  const required = def.required ?? !info.type.optional
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const host = useHostServices()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={required ? {
          required: `${info.label} is required`,
          validate: (v: unknown) => !v || !!refTarget(v)?.panel || `${info.label}: choose a panel of the object`,
        } : undefined}
        render={({ field, fieldState }) => host.pickObjects ? (
          <PanelPicker value={field.value == null ? '' : String(field.value)} onChange={field.onChange}
            filter={filter} label={info.label} description={info.description} required={required}
            disabled={def.disabled === true || isSubmitting} error={fieldState.error?.message}
            testId={'input.' + info.name} />
        ) : (
          <TextInput
            {...field}
            value={field.value ?? ''}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder ?? '<object id>#<panel>'}
            required={required}
            disabled={def.disabled === true || isSubmitting}
            readOnly={def.readOnly === true}
            error={fieldState.error?.message}
            data-testid={'input.' + info.name}
          />
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<StringType, PanelRefContent> = ({ info }) => {
  return (item: JsonObject) => (
    <span style={columnStyles(info)}><RefLink value={columnText(info, item)} size="sm" testId={'column.' + info.name} /></span>
  )
}

const PanelRefRenderer: FieldRendererFactory<StringType, PanelRefContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default PanelRefRenderer
