// SPDX-License-Identifier: Apache-2.0
// rating content renderer — Mantine Rating widget for both form and column;
// view shows filled/empty unicode stars for read-only display.

import { Input, Rating, Text } from '@mantine/core'
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
import type { RatingContent } from './types.ts'

const view: ViewFieldRendererFactory<NumberType, RatingContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = typeof item[info.name] === 'number' ? (item[info.name] as number) : 0
    const max = info.contentType.max ?? 5
    return (
      <Rating
        value={value}
        count={max}
        readOnly
        fractions={info.contentType.allowHalf ? 2 : 1}
        data-testid={'view.' + info.name}
      />
    )
  }
}

const form: FormFieldRendererFactory<NumberType, RatingContent> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const max = info.contentType.max ?? 5
    const disabled = def.readOnly === true || def.disabled === true || isSubmitting
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
            <div data-testid={'input.' + info.name} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Rating
                value={(field.value as number) ?? 0}
                onChange={field.onChange}
                count={max}
                fractions={info.contentType.allowHalf ? 2 : 1}
                readOnly={disabled}
              />
              {/* Test handles — invisible buttons exposing per-star testids that
                  delegate to `field.onChange(n)`. Browser automation scripts
                  click these instead of trying to compute Mantine's internal SVG
                  star geometry. Width/height 1 so they're pointer-event-friendly
                  but visually unobtrusive. */}
              <span aria-hidden="true" style={{ display: 'inline-flex' }}>
                {Array.from({ length: max }, (_, i) => i + 1).map((n) => (
                  <button
                    key={n}
                    type="button"
                    onClick={() => field.onChange(n)}
                    disabled={disabled}
                    data-testid={`star.${info.name}.${n}`}
                    style={{
                      width: 1,
                      height: 1,
                      padding: 0,
                      margin: 0,
                      border: 0,
                      background: 'transparent',
                      overflow: 'hidden',
                      opacity: 0,
                    }}
                    tabIndex={-1}
                  />
                ))}
              </span>
            </div>
          </Input.Wrapper>
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<NumberType, RatingContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = typeof item[info.name] === 'number' ? (item[info.name] as number) : 0
    const max = info.contentType.max ?? 5
    if (value === 0) return <Text c="dimmed">—</Text>
    return (
      <Rating
        value={value}
        count={max}
        readOnly
        fractions={info.contentType.allowHalf ? 2 : 1}
        size="sm"
        data-testid={'column.' + info.name}
      />
    )
  }
}

const RatingRenderer: FieldRendererFactory<NumberType, RatingContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default RatingRenderer
