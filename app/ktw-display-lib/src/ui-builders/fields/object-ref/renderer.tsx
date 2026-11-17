// SPDX-License-Identifier: Apache-2.0
// object-ref content renderer — a reference to an object of the project, its
// value the object's id.
//
// The form searches the objects through the host (useHostServices
// .pickObjects), filtered by the ObjectFilter the field declares (types,
// capabilities, incoming, outgoing, when); the view and the column show the
// id as a link that opens the object (host.navigate). With no host — the chat
// iframe, a test — the form is a text input and the value plain text.

import { Anchor, Select, Text, TextInput } from '@mantine/core'
import { useEffect, useMemo, useState } from 'react'
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
  refFilterOf, refTarget, useHostServices, type ObjectChoice, type RefFilter,
} from '../../../ui-core/host/index.tsx'
import type { ObjectRefContent } from './types.ts'

/** How long the picker waits after a keystroke before it searches. */
const SEARCH_DELAY_MS = 200

/** choiceLabel is how an object reads in a picker: its name, then its id
 *  and type in brackets. */
export function choiceLabel(o: ObjectChoice): string {
  const tail = [o.id, o.type].filter(Boolean).join(' · ')
  return o.name && o.name !== o.id ? `${o.name} (${tail})` : tail || o.id
}

/** pickerQuery is what the picker searches with. Once an object is chosen
 *  the select shows its label in the search box; searching with that label
 *  would find nothing and leave the list holding the bare id — so the chosen
 *  object's own label searches everything again. */
export function pickerQuery(search: string, value: string | null, known: ReadonlyMap<string, ObjectChoice>): string {
  if (value) {
    const chosen = known.get(value)
    if (search === (chosen ? choiceLabel(chosen) : value)) return ''
  }
  return search
}

/** pickerData is the list the select offers: what the search found, and the
 *  chosen object even when the search no longer returns it — under its label
 *  (name · type) when the picker has seen it, its id only until then. */
export function pickerData(found: ObjectChoice[], value: string | null, known: ReadonlyMap<string, ObjectChoice>): { value: string; label: string }[] {
  const opts = found.map((o) => ({ value: o.id, label: choiceLabel(o) }))
  if (value && !opts.some((o) => o.value === value)) {
    const chosen = known.get(value)
    opts.unshift({ value, label: chosen ? choiceLabel(chosen) : value })
  }
  return opts
}

/** ObjectPicker searches the objects a filter admits, through the host, as
 *  the person types; the current value stays a choice, under its label, even
 *  when the search no longer returns it. */
export function ObjectPicker({ value, onChange, filter, label, description, placeholder, required, disabled, error, testId }: {
  value: string | null
  onChange: (id: string | null) => void
  filter: RefFilter
  label?: string
  description?: string
  placeholder?: string
  required?: boolean
  disabled?: boolean
  error?: string
  testId?: string
}) {
  const host = useHostServices()
  const [search, setSearch] = useState('')
  const [found, setFound] = useState<ObjectChoice[]>([])
  // Every object a search returned, by id: the chosen one keeps its label
  // after the list moves on.
  const [known, setKnown] = useState<ReadonlyMap<string, ObjectChoice>>(() => new Map())
  const [failed, setFailed] = useState('')
  const filterKey = JSON.stringify(filter)
  const remember = (objs: ObjectChoice[]) => {
    if (objs.length === 0) return
    setKnown((prev) => {
      const next = new Map(prev)
      for (const o of objs) next.set(o.id, o)
      return next
    })
  }
  const query = pickerQuery(search, value, known)
  useEffect(() => {
    if (!host.pickObjects) return
    let cancelled = false
    const h = setTimeout(() => {
      host.pickObjects!(query, filter)
        .then((objs) => { if (!cancelled) { setFound(objs); remember(objs); setFailed('') } })
        .catch((e: unknown) => { if (!cancelled) setFailed(String(e instanceof Error ? e.message : e)) })
    }, SEARCH_DELAY_MS)
    return () => { cancelled = true; clearTimeout(h) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host.pickObjects, query, filterKey])
  // A value the picker has not seen (a stored form opened again): look it up
  // by its id once, so it reads as its name and type, not its id.
  useEffect(() => {
    if (!host.pickObjects || !value || known.has(value)) return
    let cancelled = false
    host.pickObjects(value, filter)
      .then((objs) => { if (!cancelled) remember(objs.filter((o) => o.id === value)) })
      .catch(() => { /* the id stays the label */ })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host.pickObjects, value, filterKey])
  const data = useMemo(() => pickerData(found, value, known), [found, value, known])
  return (
    <Select
      label={label}
      description={description}
      placeholder={placeholder ?? 'Search an object…'}
      required={required}
      disabled={disabled}
      error={error || failed || undefined}
      data={data}
      value={value}
      onChange={onChange}
      searchable
      clearable
      searchValue={search}
      onSearchChange={setSearch}
      // The host already searched: every answer it gave is shown.
      filter={({ options }) => options}
      nothingFoundMessage="No object matches"
      data-testid={testId}
    />
  )
}

/** RefLink shows a reference value: a link opening it through the host, or
 *  plain text without one. */
export function RefLink({ value, testId, size }: { value: unknown; testId?: string; size?: string }) {
  const host = useHostServices()
  const target = refTarget(value)
  if (!target) return <Text c="dimmed" size={size}>—</Text>
  const text = target.panel ? `${target.object} › ${target.panel}` : target.object
  if (!host.navigate) return <Text size={size} data-testid={testId}>{text}</Text>
  return (
    <Anchor component="button" type="button" size={size} data-testid={testId}
      onClick={(e: { stopPropagation: () => void }) => { e.stopPropagation(); host.navigate!(target) }}>
      {text}
    </Anchor>
  )
}

const view: ViewFieldRendererFactory<StringType, ObjectRefContent> = ({ info }) => {
  return (item: JsonObject) => <RefLink value={item[info.name]} testId={'view.' + info.name} />
}

const form: FormFieldRendererFactory<StringType, ObjectRefContent> = ({ def, info }) => {
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
        rules={required ? { required: `${info.label} is required` } : undefined}
        render={({ field, fieldState }) => host.pickObjects ? (
          <ObjectPicker
            value={field.value == null || field.value === '' ? null : String(field.value)}
            onChange={(id) => field.onChange(id ?? '')}
            filter={filter}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder}
            required={required}
            disabled={def.disabled === true || isSubmitting}
            error={fieldState.error?.message}
            testId={'input.' + info.name}
          />
        ) : (
          // No host to search with: the id is typed.
          <TextInput
            {...field}
            value={field.value ?? ''}
            label={info.label}
            description={info.description}
            placeholder={def.placeholder ?? 'Object id'}
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

const column: ColumnRendererFactory<StringType, ObjectRefContent> = ({ info }) => {
  return (item: JsonObject) => (
    <span style={columnStyles(info)}><RefLink value={columnText(info, item)} size="sm" testId={'column.' + info.name} /></span>
  )
}

const ObjectRefRenderer: FieldRendererFactory<StringType, ObjectRefContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default ObjectRefRenderer
