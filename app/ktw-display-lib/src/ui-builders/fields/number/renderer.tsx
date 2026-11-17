// SPDX-License-Identifier: Apache-2.0
// number content renderer — Mantine NumberInput / Text. Default for kind=number.

import { NumberInput, Select, Text } from '@mantine/core'
import { Controller, useFormContext } from 'react-hook-form'
import {
  columnStyles,
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { NumberType } from '../../../schema/schema-types.ts'
import {
  coerceFromSelect,
  hasRichDetail,
  makeRenderOption,
  resolveFieldOptions,
  toSelectData,
} from '../rich-options.tsx'
import type { NumberContent } from './types.ts'

const view: ViewFieldRendererFactory<NumberType, NumberContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    return <Text data-testid={'view.' + info.name}>{String(value)}</Text>
  }
}

const form: FormFieldRendererFactory<NumberType, NumberContent> = ({ def, info }) => {
  // A number field may declare a finite option set (rich list or map) — or
  // carry a bare numeric `enum`. Either renders as a Select whose picked value
  // is coerced back to a number; otherwise a free NumberInput.
  const enumValues = (info.type.enum as number[] | undefined) ?? []
  const options = resolveFieldOptions<number>(info.contentType, enumValues)
  const selectData = options.length > 0 ? toSelectData(options) : null
  const renderOption = hasRichDetail(options) ? makeRenderOption(options) : undefined
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    if (selectData) {
      return (
        <Controller
          key={info.id}
          name={info.name}
          control={control}
          rules={def.required === true ? { required: `${info.label} is required` } : undefined}
          render={({ field, fieldState }) => (
            <Select
              value={field.value == null ? null : String(field.value)}
              onChange={(picked) => field.onChange(coerceFromSelect(options, picked))}
              onBlur={field.onBlur}
              label={info.label}
              description={info.description}
              placeholder={def.placeholder}
              required={def.required === true}
              withAsterisk={def.required === true}
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={fieldState.error?.message}
              data={selectData}
              renderOption={renderOption}
              allowDeselect={false}
              data-testid={'input.' + info.name}
            />
          )}
        />
      )
    }
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        render={({ field, fieldState }) => (
          <NumberInput
            value={field.value as number | undefined}
            onChange={(v) => field.onChange(v === '' ? null : v)}
            onBlur={field.onBlur}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder}
            required={def.required === true}
            disabled={def.disabled === true || isSubmitting}
            readOnly={def.readOnly === true}
            error={fieldState.error?.message}
            min={info.contentType.min}
            max={info.contentType.max}
            step={info.contentType.step}
            decimalScale={info.contentType.decimals}
            thousandSeparator={info.contentType.thousandSeparator ? ',' : undefined}
            data-testid={'input.' + info.name}
          />
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<NumberType, NumberContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    return (
      <Text style={columnStyles(info)} data-testid={'column.' + info.name}>
        {String(value)}
      </Text>
    )
  }
}

const NumberRenderer: FieldRendererFactory<NumberType, NumberContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default NumberRenderer
