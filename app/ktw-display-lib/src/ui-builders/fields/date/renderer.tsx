// SPDX-License-Identifier: Apache-2.0
// date content renderer — Mantine DateInput / DateTimePicker / TimeInput
// depending on `content.pattern`. The value carried in the form state is
// always the ISO 8601 string the schema declares (no Date object); the
// Mantine inputs handle the on-screen formatting + parsing.

import { Text, TextInput } from '@mantine/core'
import { DateInput } from '@mantine/dates'
import dayjs from 'dayjs'
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
import type { StringType } from '../../../schema/schema-types.ts'
import type { DateContent, DatePattern } from './types.ts'

/** ISO sub-format the value carries — default `date` per the canonical
 *  schema convention (YYYY-MM-DD). An explicit `pattern` wins; otherwise the
 *  variant is inferred from the content-type discriminator so the shorthand
 *  forms `string@date-time` / `string@time` (content.type carries the variant,
 *  no pattern param) render the right input instead of falling back to text. */
function patternOf(content: DateContent): DatePattern {
  if (content.pattern) return content.pattern
  const t = (content as { type?: string }).type
  if (t === 'date-time' || t === 'time') return t
  return 'date'
}

/** Mantine `valueFormat` for the DateInput's display layer. Locale-friendly
 *  by default — the underlying value (`onChange` callback) is still an ISO
 *  string. Authors can override via `content.valueFormat`. */
function displayFormat(content: DateContent): string {
  if (content.valueFormat) return content.valueFormat
  switch (patternOf(content)) {
    case 'date':      return 'YYYY-MM-DD'
    case 'date-time': return 'YYYY-MM-DD HH:mm'
    case 'time':      return 'HH:mm'
  }
}

/** Convert an ISO-string form value to a Date for Mantine's DateInput.
 *  Returns null on falsy / unparseable values; Mantine treats null as
 *  the "no date selected" state. */
function isoToDate(value: unknown): Date | null {
  if (value === undefined || value === null || value === '') return null
  if (typeof value !== 'string') return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

/** Convert the Date Mantine emits back to an ISO string of the right
 *  sub-format. For 'date' we keep YYYY-MM-DD (drop the time part); for
 *  the others we keep the full ISO string. */
function dateToIso(d: Date | null, content: DateContent): string {
  if (d === null) return ''
  if (patternOf(content) === 'date') return d.toISOString().slice(0, 10)
  return d.toISOString()
}

/** Format an ISO value with the content's display format (locale-friendly),
 *  falling back to the raw string if it can't be parsed. Shared by view +
 *  column so a `string@date-time` field never shows a raw `…T07:57:57Z`. */
function formatValue(value: unknown, content: DateContent): string {
  const d = isoToDate(value)
  if (!d) return String(value)
  return dayjs(d).format(displayFormat(content))
}

const view: ViewFieldRendererFactory<StringType, DateContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null || value === '') return <Text c="dimmed">—</Text>
    return <Text data-testid={'view.' + info.name}>{formatValue(value, info.contentType)}</Text>
  }
}

const form: FormFieldRendererFactory<StringType, DateContent> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const pattern = patternOf(info.contentType)
    // 'time' currently falls back to a plain text input — Mantine's
    // TimeInput requires a different value shape and most artifact schemas
    // don't use it. The 'date' and 'date-time' paths use DateInput.
    if (pattern === 'time') {
      return (
        <Controller
          key={info.id}
          name={info.name}
          control={control}
          render={({ field, fieldState }) => (
            <TextInput
              {...field}
              label={info.label}
              description={info.description}
              placeholder={def.placeholder ?? 'HH:mm'}
              required={def.required === true}
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={fieldState.error?.message}
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
          <DateInput
            value={isoToDate(field.value)}
            onChange={(d) => field.onChange(dateToIso(d as Date | null, info.contentType))}
            onBlur={field.onBlur}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder}
            valueFormat={displayFormat(info.contentType)}
            required={def.required === true}
            disabled={def.disabled === true || isSubmitting}
            readOnly={def.readOnly === true}
            error={fieldState.error?.message}
            minDate={isoToDate(info.contentType.min) ?? undefined}
            maxDate={isoToDate(info.contentType.max) ?? undefined}
            data-testid={'input.' + info.name}
            clearable
          />
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<StringType, DateContent> = ({ info }) => {
  return (item: JsonObject) => {
    const raw = item[info.name]
    const text = raw == null || raw === '' ? '' : formatValue(raw, info.contentType)
    return (
      <Text style={columnStyles(info)} data-testid={'column.' + info.name}>
        {text || '—'}
      </Text>
    )
  }
}

const DateRenderer: FieldRendererFactory<StringType, DateContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default DateRenderer
