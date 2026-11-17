// SPDX-License-Identifier: Apache-2.0
// What a form edits in place: a field read as a row and
// edited behind its pencil (ui-form `edit: row`), a row of an array of
// objects edited inline in its table (content `table`). One line is open at
// a time, shared through RowEditingContext so the form holds `save: change`
// while it is open - OK closes it and the change is saved then.
// Kept free of JSX so it is tested on its own (tests/form-read-rows.test.tsx,
// tests/form-array-table.test.tsx).

import { createContext, useContext, useState } from 'react'

/** The line open for editing (its key) and how to open / close one. */
export type RowEditing = {
  editing: string | null
  setEditing: (key: string | null) => void
}

/** The form's shared open line; UIFormView provides it. Without a
 *  provider each editor keeps its own (useRowEditing). */
export const RowEditingContext = createContext<RowEditing | null>(null)

/** The open line of the form, or the editor's own when no form shares one. */
export function useRowEditing(): RowEditing {
  const shared = useContext(RowEditingContext)
  const [editing, setEditing] = useState<string | null>(null)
  return shared ?? { editing, setEditing }
}

/** The key of a field's read row. */
export const rowKey = (field: string): string => `row:${field}`

/** The key of a table's row. */
export const tableRowKey = (field: string, index: number): string => `table:${field}.${index}`

/** An array without its row `index`. */
export function withoutRow<T>(rows: readonly T[] | undefined, index: number): T[] {
  return (rows ?? []).filter((_, i) => i !== index)
}

/** An array with `row` put at `index` (a cancelled edit puts its snapshot
 *  back), or without it when `row` is undefined (a cancelled add). */
export function withRow<T>(rows: readonly T[] | undefined, index: number, row: T | undefined): T[] {
  if (row === undefined) return withoutRow(rows, index)
  const out = [...(rows ?? [])]
  out[index] = row
  return out
}

type ErrorNode = { message?: unknown; root?: { message?: unknown } } & Record<string, unknown>

function at(errors: unknown, path: Array<string | number>): ErrorNode | undefined {
  let e: unknown = errors
  for (const k of path) {
    if (!e || typeof e !== 'object') return undefined
    e = (e as Record<string, unknown>)[String(k)]
  }
  return e && typeof e === 'object' ? (e as ErrorNode) : undefined
}

const messageOf = (e: ErrorNode | undefined): string | undefined =>
  typeof e?.message === 'string' && e.message !== '' ? e.message : undefined

/** The message of a table's cell in error (`<field>.<row>.<column>`), or
 *  of a one-column row (`<field>.<row>`, column `_`). */
export function cellError(errors: unknown, field: string, index: number, column: string): string | undefined {
  const path = field.split('.')
  return column === '_' ? messageOf(at(errors, [...path, index])) : messageOf(at(errors, [...path, index, column]))
}

/** Whether a row of a table holds an error, and on which column. */
export function rowError(errors: unknown, field: string, index: number, columns: string[]): { column: string; message: string } | undefined {
  for (const column of columns) {
    const message = cellError(errors, field, index, column)
    if (message) return { column, message }
  }
  return undefined
}

/** The message on the table itself (a check answer with no path), shown
 *  under the table. */
export function listError(errors: unknown, field: string): string | undefined {
  const e = at(errors, field.split('.'))
  return messageOf(e) ?? messageOf(e?.root as ErrorNode | undefined)
}

/** The value at a dotted path of an item (`need.summary`), undefined
 *  when a step is missing. */
export function valueAt(item: unknown, path: string): unknown {
  let v: unknown = item
  for (const k of path.split('.')) {
    if (!v || typeof v !== 'object') return undefined
    v = (v as Record<string, unknown>)[k]
  }
  return v
}

/** The item a view renderer reads a nested field from: the value at the
 *  dotted path laid under the path itself (renderers read `item[name]`). */
export function withPathValue<T>(item: T, path: string): T {
  if (!path.includes('.')) return item
  return { ...(item ?? {}), [path]: valueAt(item, path) } as T
}

/** The count a tab shows after its title: the rows of its array fields,
 *  undefined when it has none or they are empty. A group's declared
 *  `count` wins: a field path (dotted) whose list length is shown, or
 *  `false` for no count. */
export function tabCount(values: unknown, arrayFields: readonly string[] | undefined, count?: string | false): number | undefined {
  if (count === false) return undefined
  if (typeof count === 'string' && count !== '') {
    const v = valueAt(values, count)
    return Array.isArray(v) && v.length > 0 ? v.length : undefined
  }
  if (!arrayFields || arrayFields.length === 0 || !values || typeof values !== 'object') return undefined
  let n = 0
  for (const f of arrayFields) {
    const v = valueAt(values, f)
    if (Array.isArray(v)) n += v.length
  }
  return n > 0 ? n : undefined
}

/** Whether one of a tab's fields shows an error (the tab's alert mark). */
export function tabInError(errors: unknown, fields: readonly string[] | undefined): boolean {
  if (!fields || !errors || typeof errors !== 'object') return false
  return fields.some((f) => at(errors, f.split('.')) !== undefined)
}

/** The part of a react-hook-form an editor acts through. */
export type EditForm = {
  getValues: (name: string) => unknown
  setValue: (name: string, value: unknown, options?: { shouldDirty?: boolean }) => void
  clearErrors: (name?: string) => void
}

/** What a read row's pencil, OK and Annuler do: the pencil keeps a
 *  snapshot of the field and opens its row, Annuler puts the snapshot back
 *  and closes it, OK closes it (the form saves then). */
export function readRowActions(form: EditForm, setEditing: (key: string | null) => void, field: string, snapshot: { current: unknown }) {
  return {
    edit: () => {
      snapshot.current = structuredClone(form.getValues(field))
      setEditing(rowKey(field))
    },
    cancel: () => {
      form.setValue(field, snapshot.current, { shouldDirty: true })
      form.clearErrors(field)
      setEditing(null)
    },
    ok: () => setEditing(null),
  }
}

/** What a table's pencil, OK, Annuler, « Ajouter » and delete do on an
 *  array field: the pencil keeps a snapshot of the row and opens it,
 *  Annuler puts it back (removes the row an add appended), « Ajouter »
 *  appends `seed` and opens it, delete removes the row. */
export function tableRowActions(
  form: EditForm,
  editing: string | null,
  setEditing: (key: string | null) => void,
  field: string,
  snapshot: { current: { index: number; row: unknown } | null },
  seed: () => unknown,
) {
  const rows = () => ((form.getValues(field) as unknown[] | undefined) ?? [])
  const put = (next: unknown[]) => form.setValue(field, next, { shouldDirty: true })
  return {
    edit: (index: number) => {
      snapshot.current = { index, row: structuredClone(rows()[index]) }
      setEditing(tableRowKey(field, index))
    },
    ok: () => {
      snapshot.current = null
      setEditing(null)
    },
    cancel: (index: number) => {
      const snap = snapshot.current
      put(withRow(rows(), index, snap && snap.index === index ? snap.row : undefined))
      form.clearErrors(field)
      snapshot.current = null
      setEditing(null)
    },
    add: () => {
      const index = rows().length
      put([...rows(), seed()])
      snapshot.current = { index, row: undefined }
      setEditing(tableRowKey(field, index))
    },
    remove: (index: number) => {
      put(withoutRow(rows(), index))
      form.clearErrors(field)
      if (editing === tableRowKey(field, index)) setEditing(null)
    },
  }
}
