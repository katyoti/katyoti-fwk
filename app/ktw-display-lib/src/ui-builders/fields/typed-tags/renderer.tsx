// SPDX-License-Identifier: Apache-2.0
// typed-tags content renderer - tags that each carry a type
// (an application, a domain, a question…): found through the lookup method
// the field names while typing, called through the host's callMethod,
// or typed as free text when the field allows it. The library
// knows no lookup, only the method URI it is given; without a host that calls
// methods the tags are free text only.

import { useEffect, useState } from 'react'
import { ActionIcon, Badge, Group, Paper, Stack, Text, TextInput, UnstyledButton } from '@mantine/core'
import { IconX } from '@tabler/icons-react'
import { Controller, useFormContext } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { ArrayType } from '../../../schema/schema-types.ts'
import { useHostServices } from '../../../ui-core/host/index.tsx'
import type { TagType, TypedTag, TypedTagsContent } from './types.ts'

// =================================================================================================
// The value - pure, tested apart from rendering
// =================================================================================================

/** asTags reads a field value as tags: a tag object or a bare string (a free
 *  tag); anything else is dropped. */
export function asTags(value: unknown): TypedTag[] {
  if (!Array.isArray(value)) return []
  const out: TypedTag[] = []
  for (const v of value) {
    if (typeof v === 'string' && v.trim()) out.push({ label: v.trim() })
    else if (v && typeof v === 'object' && typeof (v as TypedTag).label === 'string' && (v as TypedTag).label.trim()) out.push(v as TypedTag)
  }
  return out
}

/** sameTag: one tag twice is the same key of the same type, or - for free
 *  tags - the same label, whatever its case. */
export function sameTag(a: TypedTag, b: TypedTag): boolean {
  if (a.key || b.key) return a.key === b.key && a.type === b.type
  return !a.type && !b.type && a.label.trim().toLowerCase() === b.label.trim().toLowerCase()
}

/** addTag adds a tag once. */
export function addTag(tags: TypedTag[], tag: TypedTag): TypedTag[] {
  return tags.some((t) => sameTag(t, tag)) ? tags : [...tags, tag]
}

/** removeTag removes the tag at an index. */
export function removeTag(tags: TypedTag[], index: number): TypedTag[] {
  return tags.filter((_, i) => i !== index)
}

/** typeOf is a tag's declared type, its name when undeclared, none for a
 *  free tag. */
export function typeOf(types: TagType[] | undefined, name: string | undefined): TagType | undefined {
  if (!name) return undefined
  return types?.find((t) => t.name === name) ?? { name }
}

// =================================================================================================
// Renderers
// =================================================================================================

function TagChip({ tag, types, onRemove, disabled }: { tag: TypedTag; types?: TagType[]; onRemove?: () => void; disabled?: boolean }) {
  const t = typeOf(types, tag.type)
  return (
    <Badge variant={t ? 'light' : 'outline'} color={t?.color ?? 'gray'} size="md" data-guide="typed-tags.tag" data-guide-name={tag.key ?? tag.label}
      rightSection={onRemove && !disabled ? (
        <ActionIcon size="xs" variant="transparent" color="gray" aria-label="×" onClick={onRemove}><IconX size={10} /></ActionIcon>
      ) : undefined}>
      {t ? `${t.label ?? t.name} · ${tag.label}` : tag.label}
    </Badge>
  )
}

function TagList({ value, types, testId }: { value: unknown; types?: TagType[]; testId?: string }) {
  const tags = asTags(value)
  if (tags.length === 0) return <Text c="dimmed">—</Text>
  return <Group gap={4} data-testid={testId}>{tags.map((t, i) => <TagChip key={i} tag={t} types={types} />)}</Group>
}

/** TypedTagsInput edits the tags: the chips, and an input whose text is
 *  looked up while typing (two letters at least); Enter adds the text as a
 *  free tag when allowed. */
export function TypedTagsInput({ content, value, onChange, disabled, placeholder }: {
  content: TypedTagsContent
  value: TypedTag[]
  onChange: (next: TypedTag[]) => void
  disabled?: boolean
  placeholder?: string
}) {
  const host = useHostServices()
  const call = content.lookup ? host.callMethod : undefined
  const free = content.free !== false
  const [text, setText] = useState('')
  const [found, setFound] = useState<TypedTag[]>([])
  useEffect(() => {
    const q = text.trim()
    if (!call || q.length < 2) { setFound([]); return }
    let live = true
    const timer = setTimeout(() => {
      call(content.lookup!, { query: q, ...(content.object ? { object: content.object } : {}) })
        .then((out) => { if (live) setFound(asTags((out as { items?: unknown } | undefined)?.items)) })
        .catch(() => { if (live) setFound([]) })
    }, 250)
    return () => { live = false; clearTimeout(timer) }
  }, [call, text, content.lookup, content.object])
  const take = (tag: TypedTag) => { onChange(addTag(value, tag)); setText(''); setFound([]) }
  return (
    <Stack gap={4}>
      {value.length > 0 && (
        <Group gap={4}>{value.map((t, i) => <TagChip key={i} tag={t} types={content.types} disabled={disabled} onRemove={() => onChange(removeTag(value, i))} />)}</Group>
      )}
      <TextInput value={text} disabled={disabled} placeholder={placeholder} data-guide="typed-tags.input"
        onChange={(e) => setText(e.currentTarget.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            if (found.length > 0) take(found[0])
            else if (free && text.trim()) take({ label: text.trim() })
          }
        }} />
      {found.length > 0 && (
        <Paper withBorder p={4} data-guide="typed-tags.found">
          <Stack gap={2}>
            {found.map((t, i) => (
              <UnstyledButton key={i} onClick={() => take(t)} data-guide="typed-tags.option" data-guide-name={t.key ?? t.label}>
                <TagChip tag={t} types={content.types} />
              </UnstyledButton>
            ))}
          </Stack>
        </Paper>
      )}
    </Stack>
  )
}

const view: ViewFieldRendererFactory<ArrayType, TypedTagsContent> = ({ info }) => {
  return (item: JsonObject) => <TagList value={item[info.name]} types={info.contentType.types} testId={'view.' + info.name} />
}

const form: FormFieldRendererFactory<ArrayType, TypedTagsContent> = ({ def, info }) => {
  const required = def.required ?? !info.type.optional
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={required ? { validate: (v) => asTags(v).length > 0 || `${info.label} is required` } : undefined}
        render={({ field, fieldState }) => (
          <Stack gap={4} data-testid={'input.' + info.name}>
            <Text size="sm" fw={500}>{info.label}</Text>
            {info.description && <Text size="xs" c="dimmed">{info.description}</Text>}
            <TypedTagsInput
              content={info.contentType}
              value={asTags(field.value)}
              onChange={(next) => field.onChange(next)}
              disabled={def.disabled === true || isSubmitting}
            />
            {fieldState.error?.message && <Text size="xs" c="red">{fieldState.error.message}</Text>}
          </Stack>
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<ArrayType, TypedTagsContent> = ({ info }) => {
  return (item: JsonObject) => <Text size="sm">{asTags(item[info.name]).map((t) => t.label).join(', ')}</Text>
}

const TypedTagsRenderer: FieldRendererFactory<ArrayType, TypedTagsContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default TypedTagsRenderer
