// SPDX-License-Identifier: Apache-2.0
// JSON content renderer — pragmatic fallback for field types the lib
// doesn't have a dedicated renderer for yet (user-defined object
// types, arrays of objects, `any`/`null` shapes, etc.).
//
// Form mode: monospaced Textarea, value JSON-stringified on read +
// parsed back on change. Invalid JSON keeps the in-progress text so
// the user can fix typos without losing their edit.
// View mode: pretty-printed JSON inside a Mantine Code block.
//
// US-4 close-out (2026-06-08): wired as the kind-default for
// non-primitive content types in `info.ts` (`{type: 'json'}`
// fallback). Replaces the prior throw when getInfo encountered a
// nested type. Proper recursive nested-form renderer ships in
// ms-schema-driven-ui US-S4 — at which point this stays as the
// explicit `content: {type: 'json'}` opt-in and stops being the
// fallback.

import { Code, Stack, Table, Text, Textarea } from '@mantine/core'
import { useEffect, useState, type ReactNode } from 'react'
import { Controller, useFormContext } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject, JsonValue } from '../../../types/json-types.ts'
import type { SchemaType } from '../../../schema/schema-types.ts'
import type { ContentType } from '../../../schema/schema-contents.ts'

function safeStringify(v: unknown): string {
  if (v === undefined || v === null) return ''
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

// coerceJSON parses a JSON-looking string into structure so the view can render
// objects/arrays even when the value is stored as a JSON string (e.g. the
// action-invocation audit body). Non-JSON strings pass through unchanged.
function coerceJSON(v: unknown): unknown {
  if (typeof v !== 'string') return v
  const t = v.trim()
  if (t.startsWith('{') || t.startsWith('[')) {
    try {
      return JSON.parse(t)
    } catch {
      return v
    }
  }
  return v
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === 'object' && !Array.isArray(v)
}

function isArrayOfObjects(v: unknown): v is Array<Record<string, unknown>> {
  return Array.isArray(v) && v.length > 0 && v.every(isPlainObject)
}

function renderScalar(v: unknown): ReactNode {
  if (v === null || v === undefined || v === '') return <Text c="dimmed" span>—</Text>
  if (typeof v === 'object') return <Code style={{ fontSize: 10 }}>{safeStringify(v)}</Code>
  return <Text span style={{ fontSize: 12 }}>{String(v)}</Text>
}

// renderTable renders an array-of-objects as a Mantine table. Columns are the
// union of keys across rows — generic, no hardcoded schema.
function renderTable(rows: Array<Record<string, unknown>>): ReactNode {
  const cols: string[] = []
  for (const r of rows) for (const k of Object.keys(r)) if (!cols.includes(k)) cols.push(k)
  return (
    <Table striped withTableBorder withColumnBorders style={{ fontSize: 12 }}>
      <Table.Thead>
        <Table.Tr>{cols.map((c) => <Table.Th key={c}>{c}</Table.Th>)}</Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {rows.map((r, i) => (
          <Table.Tr key={i}>{cols.map((c) => <Table.Td key={c}>{renderScalar(r[c])}</Table.Td>)}</Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  )
}

// renderStructured: array-of-objects → table; object → labelled key/value
// (nested arrays-of-objects render as their own table); everything else →
// pretty JSON. Recursive + generic.
function renderStructured(v: unknown): ReactNode {
  if (isArrayOfObjects(v)) return renderTable(v)
  if (isPlainObject(v)) {
    return (
      <Stack gap={8}>
        {Object.entries(v).map(([k, val]) => (
          <div key={k}>
            <Text fw={600} c="dimmed" style={{ fontSize: 11 }}>{k}</Text>
            {isArrayOfObjects(val) || isPlainObject(val)
              ? renderStructured(val)
              : renderScalar(val)}
          </div>
        ))}
      </Stack>
    )
  }
  return (
    <Code block style={{ fontSize: 11, maxHeight: 240, overflow: 'auto' }}>
      {safeStringify(v)}
    </Code>
  )
}

const view: ViewFieldRendererFactory<SchemaType, ContentType> = ({ info }) => {
  return (item: JsonObject) => {
    const raw = item[info.name]
    if (raw === undefined || raw === null || raw === '') {
      return <Text c="dimmed">—</Text>
    }
    return <div data-testid={'view.' + info.name}>{renderStructured(coerceJSON(raw))}</div>
  }
}

const form: FormFieldRendererFactory<SchemaType, ContentType> = ({ def, info }) => {
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        render={({ field, fieldState }) => {
          const [text, setText] = useState(() => safeStringify(field.value))
          const [parseError, setParseError] = useState<string | null>(null)
          // Re-sync local text when the form value changes from
          // outside (form reset, initial load).
          useEffect(() => {
            setText(safeStringify(field.value))
            setParseError(null)
            // eslint-disable-next-line react-hooks/exhaustive-deps
          }, [JSON.stringify(field.value)])
          return (
            <Textarea
              data-testid={'input.' + info.name}
              value={text}
              onChange={(e) => {
                const next = e.currentTarget.value
                setText(next)
                if (next.trim() === '') {
                  setParseError(null)
                  field.onChange(undefined)
                  return
                }
                try {
                  const parsed = JSON.parse(next) as JsonValue
                  setParseError(null)
                  field.onChange(parsed)
                } catch (err) {
                  setParseError((err as Error).message)
                }
              }}
              onBlur={field.onBlur}
              autosize
              minRows={3}
              maxRows={10}
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={parseError ?? fieldState.error?.message}
              styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
              placeholder='{ "...": "..." }'
            />
          )
        }}
      />
    )
  }
}

const column: ColumnRendererFactory<SchemaType, ContentType> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) {
      return <Text c="dimmed">—</Text>
    }
    // Column mode: single-line compact summary so the table row
    // doesn't blow out. `Stack` lets the consumer override height if
    // they care.
    return (
      <Stack gap={0}>
        <Code style={{ fontSize: 11 }} data-testid={'column.' + info.name}>
          {safeStringify(value).slice(0, 80)}
        </Code>
      </Stack>
    )
  }
}

const JsonRenderer: FieldRendererFactory<SchemaType, ContentType> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default JsonRenderer
