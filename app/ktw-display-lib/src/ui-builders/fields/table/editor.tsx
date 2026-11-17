// SPDX-License-Identifier: Apache-2.0
// table content editor — an array field with content
// `table` is edited as a table in a form: its columns the item fields (or
// its ui-table extension's), each row read through the cells' view
// renderers with a pencil; the pencil turns the row's cells into inputs
// with OK / Annuler (Annuler puts the row's snapshot back), « Ajouter »
// appends an empty row opened for editing, a delete per row. A string
// array is one column. The field's check message lands on its row and cell
// when the check answers a `path` (form-line checkPath) - the row stays
// open, its cell red with the message under it -, under the table when not.
// One row is open at a time across the form (RowEditingContext), and the
// form's `save: change` waits while it is open.

import { ActionIcon, Box, Button, Group, Stack, Table, Text, TextInput } from '@mantine/core'
import { IconPencil, IconPlus, IconTrash } from '@tabler/icons-react'
import { useRef } from 'react'
import { useFormContext } from 'react-hook-form'

import { SchemaQuery } from '../../../schema/schema-query.ts'
import type { ObjectType, SchemaType } from '../../../schema/schema-types.ts'
import type { ContentType, FieldOptionList } from '../../../schema/schema-contents.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { UITexts } from '../../../ui-core/i18n/types.ts'
import { resolveArrayExt } from '../_array-display.tsx'
import { cellError, listError, rowError, tableRowActions, tableRowKey, useRowEditing, type EditForm } from '../editing.ts'
import type { CrudFieldInfo } from '../info.ts'
import { innerFields } from '../object/renderer.tsx'
import { getFactory } from '../orchestrator.ts'
import type { FormFieldRendererFactory } from '../renderer.ts'
import type { UIFieldDef } from '../types.ts'

/** A column of the table editor: the item field it binds (`_` for the item
 *  itself, a string array's one column) and its header. */
export type TableEditorColumn = { name: string; label: string; options?: FieldOptionList }

/** tableEditorColumns says the editor's columns: the ui-table extension's
 *  when the field names one (a column's `options` kept: its enum read and
 *  picked through their labels), else every item field; one column `_` for
 *  an array of strings. */
export function tableEditorColumns(info: CrudFieldInfo): TableEditorColumn[] {
  if (info.type.type !== 'object') return [{ name: '_', label: info.label }]
  const fields = innerFields(info.schema, info.type as ObjectType)
  const labelOf = (n: string) => fields.find((f) => f.name === n)?.label ?? n
  const ext = resolveArrayExt(info, 'ui-table') as { columns?: unknown }
  if (Array.isArray(ext.columns) && ext.columns.length > 0) {
    return ext.columns.flatMap((c): TableEditorColumn[] => {
      if (typeof c === 'string') return [{ name: c, label: labelOf(c) }]
      if (!c || typeof c !== 'object') return []
      const col = c as { id?: unknown; name?: unknown; label?: unknown; options?: unknown }
      const name = typeof col.name === 'string' ? col.name : typeof col.id === 'string' ? col.id : ''
      if (name === '') return []
      const options = Array.isArray(col.options) && col.options.length > 0 ? (col.options as FieldOptionList) : undefined
      return [{ name, label: typeof col.label === 'string' ? col.label : labelOf(name), ...(options ? { options } : {}) }]
    })
  }
  return fields.map((f) => ({ name: f.name, label: f.label }))
}

const say = (texts: UITexts, key: string, fallback: string): string => texts.t(key, { defaultValue: fallback }) || fallback

/** A cell read: the column field's own view renderer on the row. */
function CellView({ info, texts, column, options, row }: { info: CrudFieldInfo; texts: UITexts; column: string; options?: FieldOptionList; row: unknown }) {
  if (column === '_') {
    return row === undefined || row === null || row === '' ? <Text c="dimmed">—</Text> : <Text size="sm">{String(row)}</Text>
  }
  const def: UIFieldDef = { id: column, ...(options ? { options } : {}) }
  try {
    const { factory, info: leaf } = getFactory(info.schema, column, def, info.type)
    return <>{factory.view({ def, info: leaf, texts })((row ?? {}) as JsonObject)}</>
  } catch {
    const v = (row as JsonObject | undefined)?.[column]
    return <Text size="sm">{v === undefined || v === null ? '—' : String(v)}</Text>
  }
}

/** A cell edited: the column field's own form renderer, bound to the row's
 *  cell path (`<field>.<row>.<column>`), drawn without a label. */
function CellInput({ info, texts, column, options, path, invalid }: { info: CrudFieldInfo; texts: UITexts; column: string; options?: FieldOptionList; path: string; invalid: boolean }) {
  const form = useFormContext()
  if (column === '_') {
    return <TextInput size="xs" {...form.register(path)} error={invalid || undefined} aria-invalid={invalid || undefined} />
  }
  const def: UIFieldDef = { id: path, label: '', description: '', ...(options ? { options } : {}) }
  const { factory, info: leaf } = getFactory(info.schema, column, def, info.type)
  return <>{factory.form({ def, info: { ...leaf, id: path, name: path }, texts })({}, false, false)}</>
}

/** TableEditor edits an array field as a table, its rows in place. */
export function TableEditor({ info, texts, disabled }: { info: CrudFieldInfo; texts: UITexts; disabled: boolean }) {
  const form = useFormContext()
  const { editing, setEditing } = useRowEditing()
  const snapshot = useRef<{ index: number; row: unknown } | null>(null)
  const name = info.name
  const columns = tableEditorColumns(info)
  const rows = (form.watch(name) as unknown[] | undefined) ?? []
  const errors = form.formState.errors
  const base = `form.table.${name}`
  const seed = () => (info.type.type === 'object' ? ((SchemaQuery.getDefaultValue(info.schema, info.type) as JsonObject) ?? {}) : '')
  const act = tableRowActions(form as unknown as EditForm, editing, setEditing, name, snapshot, seed)
  const under = listError(errors, name)

  return (
    <Stack gap="xs" data-testid={base}>
      <Box>
        <Text size="sm" fw={500}>{info.label}</Text>
        {info.description && <Text size="xs" c="dimmed">{info.description}</Text>}
      </Box>
      <Table withTableBorder verticalSpacing={6}>
        <Table.Thead>
          <Table.Tr>
            {columns.map((c) => <Table.Th key={c.name}>{c.label}</Table.Th>)}
            <Table.Th style={{ width: 110 }} />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {rows.map((row, index) => {
            const key = tableRowKey(name, index)
            const bad = rowError(errors, name, index, columns.map((c) => c.name))
            const open = editing === key || bad !== undefined
            const rid = `${base}.row.${index}`
            return (
              <Table.Tr key={index} data-testid={rid} data-editing={open ? 'true' : 'false'} data-error={bad ? 'true' : undefined}>
                {columns.map((c) => {
                  const path = c.name === '_' ? `${name}.${index}` : `${name}.${index}.${c.name}`
                  const message = open ? cellError(errors, name, index, c.name) : undefined
                  return (
                    <Table.Td key={c.name} style={message ? { background: 'var(--mantine-color-red-light)' } : undefined}>
                      {open ? <CellInput info={info} texts={texts} column={c.name} options={c.options} path={path} invalid={!!message} />
                        : <CellView info={info} texts={texts} column={c.name} options={c.options} row={row} />}
                      {message && <Text size="xs" c="red" role="alert" data-testid={`${rid}.error`}>{message}</Text>}
                    </Table.Td>
                  )
                })}
                <Table.Td style={{ textAlign: 'right' }}>
                  {open ? (
                    <Group gap={4} wrap="nowrap" justify="flex-end">
                      <Button size="compact-xs" data-testid={`${rid}.ok`} disabled={bad !== undefined}
                        onClick={act.ok}>
                        {say(texts, 'fields.ok', 'OK')}
                      </Button>
                      <Button size="compact-xs" variant="subtle" color="gray" data-testid={`${rid}.cancel`} onClick={() => act.cancel(index)}>
                        {say(texts, 'buttons.cancel', 'Annuler')}
                      </Button>
                    </Group>
                  ) : (
                    <Group gap={2} wrap="nowrap" justify="flex-end">
                      <ActionIcon size="sm" variant="subtle" color="gray" data-testid={`${rid}.edit`}
                        aria-label={say(texts, 'buttons.edit', 'Modifier')}
                        disabled={disabled || editing !== null} onClick={() => act.edit(index)}>
                        <IconPencil size={15} />
                      </ActionIcon>
                      <ActionIcon size="sm" variant="subtle" color="red" data-testid={`${rid}.delete`}
                        aria-label={say(texts, 'fields.remove', 'Retirer')}
                        disabled={disabled || editing !== null} onClick={() => act.remove(index)}>
                        <IconTrash size={14} />
                      </ActionIcon>
                    </Group>
                  )}
                </Table.Td>
              </Table.Tr>
            )
          })}
        </Table.Tbody>
      </Table>
      {under && <Text size="xs" c="red" role="alert" data-testid={`${base}.error`}>{under}</Text>}
      <Button size="compact-xs" variant="subtle" style={{ alignSelf: 'flex-start' }} leftSection={<IconPlus size={13} />}
        data-testid={`${base}.add`} disabled={disabled || editing !== null} onClick={act.add}>
        {say(texts, 'fields.add', 'Ajouter')}
      </Button>
    </Stack>
  )
}

/** The table content type's form: the table editor. */
export const tableForm: FormFieldRendererFactory<SchemaType, ContentType> = ({ info, texts, def }) =>
  (_item, _isNew, isSubmitting) => (
    <TableEditor info={info} texts={texts} disabled={(def as { disabled?: boolean }).disabled === true || isSubmitting} />
  )
