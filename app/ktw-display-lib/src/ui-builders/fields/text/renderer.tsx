// SPDX-License-Identifier: Apache-2.0
// text content renderer — Mantine TextInput / Text. Default for kind=string.
//
// Enum-aware: when the schema type declares `enum: [...]`, the form
// path swaps the free-text TextInput for a Mantine Select bound to
// those choices. View + column paths stay textual — the rendered
// value is just the chosen string, no special treatment needed.

import { Badge, Select, Stack, Text, TextInput } from '@mantine/core'
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
import { hasDescriptions, hasRichDetail, makeRenderOption, pickedOption, resolveFieldOptions, toSelectData } from '../rich-options.tsx'
import type { TextContent } from './types.ts'

const view: ViewFieldRendererFactory<StringType, TextContent> = ({ info }) => {
  // enum-aware (US-S5): an enum-constrained string renders as a
  // coloured Badge (colour from the content-type's `color_by_value`
  // map when present), mirroring the form-mode Select. Plain strings
  // stay as Text.
  const isEnum = Array.isArray(info.type.enum) && info.type.enum.length > 0
  const colorByValue = (info.contentType as { color_by_value?: Record<string, string> })
    .color_by_value
  const declared = resolveFieldOptions(info.contentType, [])
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null || value === '') return <Text c="dimmed">—</Text>
    if (isEnum) {
      const v = String(value)
      return (
        <Badge size="sm" variant="light" color={colorByValue?.[v]} data-testid={'view.' + info.name}>
          {info.options?.[v] ?? v}
        </Badge>
      )
    }
    // A plain string with options (declared on the field, or answered by
    // its choices method) is read by its option's label.
    return <Text data-testid={'view.' + info.name}>{pickedOption(declared, value)?.label ?? String(value)}</Text>
  }
}

const form: FormFieldRendererFactory<StringType, TextContent> = ({ def, info }) => {
  const enumValues = info.type.enum
  // Rich options (label + description + icon) declared on the content-type win;
  // otherwise fall back to the bare `enum` keys (value === label).
  const options = resolveFieldOptions(
    info.contentType,
    enumValues && enumValues.length > 0 ? enumValues.map(String) : [],
  )
  const selectData = options.length > 0 ? toSelectData(options) : null
  const renderOption = hasRichDetail(options) ? makeRenderOption(options) : undefined
  // Options carrying a description: the picked one's is shown under the
  // select (« Modèle » and the sections it writes), following each change.
  const describes = hasDescriptions(options)
  // Required resolution: ui-form ext's per-field override wins; otherwise
  // fall back to the schema's `optional` flag (required by default unless
  // explicitly opted out). Matches the Go canonical default — `Type.Optional`
  // is false unless set.
  const required = def.required ?? !info.type.optional
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        // RHF validation: when required, an empty/null value fails submit
        // and the per-field error shows in the Mantine `error` slot below.
        rules={required ? { required: `${info.label} is required` } : undefined}
        render={({ field, fieldState }) => {
          if (selectData) {
            // Enum-restricted string — surface as a Mantine Select so the
            // user can only pick a valid value. `allowDeselect={false}`
            // keeps the field non-empty once a default lands.
            const select = (
              <Select
                {...field}
                value={field.value == null ? null : String(field.value)}
                onChange={(value) => field.onChange(value)}
                label={info.label}
                description={info.description}
                placeholder={def.placeholder}
                required={required}
                withAsterisk={required}
                disabled={def.disabled === true || isSubmitting}
                readOnly={def.readOnly === true}
                error={fieldState.error?.message}
                data={selectData}
                renderOption={renderOption}
                allowDeselect={false}
                data-testid={'input.' + info.name}
              />
            )
            if (!describes) return select
            const picked = pickedOption(options, field.value)
            return (
              <Stack gap={4}>
                {select}
                {picked?.description ? (
                  <Text size="xs" c="dimmed" data-testid={'picked.' + info.name}>{picked.description}</Text>
                ) : null}
              </Stack>
            )
          }
          return (
            <TextInput
              {...field}
              label={info.label}
              description={info.description}
              placeholder={def.placeholder}
              required={required}
              withAsterisk={required}
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={fieldState.error?.message}
              minLength={info.contentType.minLength}
              maxLength={info.contentType.maxLength}
              data-testid={'input.' + info.name}
            />
          )
        }}
      />
    )
  }
}

const column: ColumnRendererFactory<StringType, TextContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = columnText(info, item)
    return (
      <Text style={columnStyles(info)} data-testid={'column.' + info.name}>
        {value || '—'}
      </Text>
    )
  }
}

const TextRenderer: FieldRendererFactory<StringType, TextContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default TextRenderer
