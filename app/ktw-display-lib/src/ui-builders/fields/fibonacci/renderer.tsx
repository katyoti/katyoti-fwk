// SPDX-License-Identifier: Apache-2.0
// fibonacci content renderer — a modified-Fibonacci Select for relative
// estimation (story points / WSJF primitives). Mantine Select for form,
// plain numeric Text for view/column.

import { Input, Select, Text } from '@mantine/core'
import { Controller, useFormContext } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { NumberType } from '../../../schema/schema-types.ts'
import type { FibonacciContent } from './types.ts'

const BASE = [1, 2, 3, 5, 8, 13, 20]

function optionsFor(content: FibonacciContent) {
  const scale = content.allowZero ? [0, ...BASE] : BASE
  return scale.map((v) => ({ value: String(v), label: String(v) }))
}

const view: ViewFieldRendererFactory<NumberType, FibonacciContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null || value === '') return <Text c="dimmed">—</Text>
    return <Text data-testid={'view.' + info.name}>{String(value)}</Text>
  }
}

const form: FormFieldRendererFactory<NumberType, FibonacciContent> = ({ def, info }) => {
  const data = optionsFor(info.contentType)
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        render={({ field, fieldState }) => (
          <Input.Wrapper
            label={info.label}
            description={info.description}
            required={def.required === true}
            error={fieldState.error?.message}
          >
            <Select
              data={data}
              value={field.value === undefined || field.value === null || field.value === '' ? null : String(field.value)}
              onChange={(v) => field.onChange(v === null ? null : Number(v))}
              onBlur={field.onBlur}
              placeholder="—"
              allowDeselect={false}
              disabled={def.disabled === true || isSubmitting || def.readOnly === true}
              data-testid={'input.' + info.name}
              w={120}
              comboboxProps={{ withinPortal: true }}
            />
          </Input.Wrapper>
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<NumberType, FibonacciContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null || value === '') return <Text c="dimmed">—</Text>
    return <Text size="sm" data-testid={'column.' + info.name}>{String(value)}</Text>
  }
}

const FibonacciRenderer: FieldRendererFactory<NumberType, FibonacciContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default FibonacciRenderer
