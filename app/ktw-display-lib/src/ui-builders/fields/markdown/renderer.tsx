// SPDX-License-Identifier: Apache-2.0
// markdown content renderer — Tabs editor (write/preview) for form, ReactMarkdown for view.

import { Text } from '@mantine/core'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Controller, useFormContext } from 'react-hook-form'
import {
  columnText,
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import { MarkdownEditor } from '../../../ui-core/components/editors/index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { StringType } from '../../../schema/schema-types.ts'
import type { MarkdownContent } from './types.ts'

const view: ViewFieldRendererFactory<StringType, MarkdownContent> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (!value) return <Text c="dimmed">—</Text>
    return (
      <div data-testid={'view.' + info.name}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{String(value)}</ReactMarkdown>
      </div>
    )
  }
}

const form: FormFieldRendererFactory<StringType, MarkdownContent> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const maxLength = info.contentType.maxLength
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={
          maxLength !== undefined
            ? { maxLength: { value: maxLength, message: `Max ${maxLength} characters` } }
            : undefined
        }
        render={({ field, fieldState }) => (
          <MarkdownEditor
            value={(field.value as string) || ''}
            onChange={field.onChange}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder}
            required={def.required === true}
            disabled={def.disabled === true || isSubmitting}
            readOnly={def.readOnly === true}
            error={fieldState.error?.message}
            height={300}
            testId={'input.' + info.name}
          />
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<StringType, MarkdownContent> = ({ info }) => {
  return (item: JsonObject) => {
    // Columns show plain text (truncated), not the rendered markdown.
    const value = columnText(info, item)
    if (!value) return <Text c="dimmed">—</Text>
    const truncated = value.length > 80 ? value.slice(0, 77) + '…' : value
    return <Text size="sm" data-testid={'column.' + info.name}>{truncated}</Text>
  }
}

const MarkdownRenderer: FieldRendererFactory<StringType, MarkdownContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default MarkdownRenderer
