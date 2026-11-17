// SPDX-License-Identifier: Apache-2.0
// slider content renderer — Mantine Slider for form, NumericText for view/column.

import { Input, Slider, Text } from '@mantine/core'
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
import type { SliderContent } from './types.ts'

const view: ViewFieldRendererFactory<NumberType, SliderContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    return (
      <Text data-testid={'view.' + info.name}>
        {String(value)}{' '}
        <Text component="span" size="xs" c="dimmed">
          ({info.contentType.min ?? 0}–{info.contentType.max ?? 100})
        </Text>
      </Text>
    )
  }
}

const form: FormFieldRendererFactory<NumberType, SliderContent> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const min = info.contentType.min ?? 0
    const max = info.contentType.max ?? 100
    const step = info.contentType.step ?? 1
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
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <div style={{ flex: 1 }}>
                <Slider
                  value={typeof field.value === 'number' ? field.value : min}
                  onChange={field.onChange}
                  onChangeEnd={field.onBlur}
                  min={min}
                  max={max}
                  step={step}
                  disabled={def.disabled === true || isSubmitting || def.readOnly === true}
                  marks={
                    info.contentType.showMarks
                      ? [
                          { value: min, label: String(min) },
                          { value: max, label: String(max) },
                        ]
                      : undefined
                  }
                />
              </div>
              {/* Test-handle: a real <input type="number"> that mirrors the
                  slider value. A browser automation `fill` writes here; the slider
                  reflects the change because both are controlled by the
                  same react-hook-form `field`. Visually small but functional. */}
              <input
                type="number"
                value={typeof field.value === 'number' ? field.value : min}
                onChange={(e) => {
                  const v = e.currentTarget.value === '' ? min : Number(e.currentTarget.value)
                  field.onChange(v)
                }}
                min={min}
                max={max}
                step={step}
                disabled={def.disabled === true || isSubmitting || def.readOnly === true}
                data-testid={'input.' + info.name}
                style={{ width: 64, padding: '4px 6px', fontSize: 12 }}
              />
            </div>
          </Input.Wrapper>
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<NumberType, SliderContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    return <Text size="sm" data-testid={'column.' + info.name}>{String(value)}</Text>
  }
}

const SliderRenderer: FieldRendererFactory<NumberType, SliderContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default SliderRenderer
