// SPDX-License-Identifier: Apache-2.0
// email content renderer — Mantine TextInput type=email + mailto link in view.

import { Anchor, Text, TextInput } from '@mantine/core'
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
import type { EmailContent } from './types.ts'

const view: ViewFieldRendererFactory<StringType, EmailContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (!value) return <Text c="dimmed">—</Text>
    const v = String(value)
    return (
      <Anchor href={`mailto:${v}`} data-testid={'view.' + info.name}>
        {v}
      </Anchor>
    )
  }
}

const form: FormFieldRendererFactory<StringType, EmailContent> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={{
          pattern: {
            value: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
            message: 'Invalid email address',
          },
        }}
        render={({ field, fieldState }) => (
          <TextInput
            {...field}
            type="email"
            label={info.label}
            description={info.description}
            placeholder={def.placeholder}
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
}

const column: ColumnRendererFactory<StringType, EmailContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = columnText(info, item)
    if (!value) return <Text style={columnStyles(info)}>—</Text>
    return (
      <Anchor
        href={`mailto:${value}`}
        size="sm"
        style={columnStyles(info)}
        data-testid={'column.' + info.name}
      >
        {value}
      </Anchor>
    )
  }
}

const EmailRenderer: FieldRendererFactory<StringType, EmailContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default EmailRenderer
