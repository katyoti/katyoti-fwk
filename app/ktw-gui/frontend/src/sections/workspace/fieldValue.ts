// SPDX-License-Identifier: Apache-2.0
/**
 * fieldValue — how a stored field value reads in a fields tab's read mode.
 *
 * A structured value is never drawn as JSON: a list reads as a list (a list
 * of records, one line per record - its title, then its other facts: who,
 * when), an object as label/value rows, a scalar as its text. Generic, for
 * every type's fields (an information source's « Sélection » is one case).
 */

/** What a value reads as. */
export type FieldView =
  | { kind: 'text'; text: string }
  | { kind: 'list'; items: FieldView[] }
  | { kind: 'record'; title: string; facts: string[] }
  | { kind: 'rows'; rows: { label: string; value: FieldView }[] }

/** The fields that name a record, first found wins. */
const titleKeys = ['title', 'name', 'label', 'key', 'id']

const isPlain = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v)

/** An RFC 3339 date-time reads as `YYYY-MM-DD HH:MM`. */
const dateTime = /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})/

/** A field name reads as words: `kept_by` → `kept by`. */
export function fieldLabel(name: string): string {
  return name.replace(/[_-]+/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2').toLowerCase()
}

/** A scalar, or an actor-like object, as one short text; undefined when it is not one. */
function shortText(v: unknown): string | undefined {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') {
    const m = dateTime.exec(v)
    return m ? `${m[1]} ${m[2]}` : v
  }
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  if (isPlain(v)) {
    for (const k of titleKeys) {
      if (typeof v[k] === 'string' && v[k]) return v[k] as string
    }
  }
  return undefined
}

/** A record of a list: its title and its other facts on one line. */
function recordView(v: Record<string, unknown>): FieldView | undefined {
  const titleKey = titleKeys.find((k) => typeof v[k] === 'string' && v[k])
  if (!titleKey) return undefined
  const facts: string[] = []
  for (const [k, x] of Object.entries(v)) {
    if (k === titleKey || (titleKey !== 'key' && k === 'key')) continue
    if (x === true) { facts.push(fieldLabel(k)); continue }
    if (x === false || x === null || x === undefined || x === '') continue
    const s = shortText(x)
    if (s === undefined) return undefined // a nested structure: not a flat record
    facts.push(s)
  }
  return { kind: 'record', title: v[titleKey] as string, facts }
}

/** What a stored value reads as. */
export function fieldView(v: unknown): FieldView {
  if (Array.isArray(v)) {
    return {
      kind: 'list',
      items: v.map((x) => (isPlain(x) ? recordView(x) ?? fieldView(x) : fieldView(x))),
    }
  }
  if (isPlain(v)) {
    return {
      kind: 'rows',
      rows: Object.entries(v).map(([k, x]) => ({ label: fieldLabel(k), value: fieldView(x) })),
    }
  }
  return { kind: 'text', text: shortText(v) ?? String(v) }
}
