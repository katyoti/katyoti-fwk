// SPDX-License-Identifier: Apache-2.0
// The `slots` view layout's pure helpers: which fields fill which slot, how a
// dotted path reaches its field through the schema, and the meta line.

import type { Schema, SchemaType, SchemaTypeRef } from '../../../../schema/index.ts'
import { SchemaQuery } from '../../../../schema/index.ts'
import type { UIViewSlots } from '../../ui-view.schema.ts'
import { PALETTE, statusTone, TONES, type Tone } from '../../../fields/_planning/palette.ts'

/** The slot names, in drawing order. */
export const SLOT_NAMES = ['eyebrow', 'title', 'titleId', 'badges', 'meta', 'footer', 'actions'] as const

/** A slot entry: a field's path and the tone its text is drawn in, if any. */
export interface SlotEntry { path: string; tone?: Tone | string }

/** slotEntry reads an entry written as a path (`summary`) or as a path with
 *  a tone (`{field: planned_text, tone: planned}`); anything else is none. */
export function slotEntry(v: unknown): SlotEntry | undefined {
  if (typeof v === 'string') return v.trim() ? { path: v.trim() } : undefined
  if (!v || typeof v !== 'object') return undefined
  const { field, tone } = v as { field?: unknown; tone?: unknown }
  if (typeof field !== 'string' || !field.trim()) return undefined
  const t = slotTone(tone)
  return t ? { path: field.trim(), tone: t } : { path: field.trim() }
}

/** slotTone reads an entry's tone: a palette tone or token as itself, a
 *  status word as its tone (`planned` → plan); none when absent. */
export function slotTone(tone: unknown): string | undefined {
  if (typeof tone !== 'string' || !tone.trim()) return undefined
  const t = tone.trim().toLowerCase()
  if ((TONES as readonly string[]).includes(t) || PALETTE[t] !== undefined) return t
  return statusTone(t).tone
}

/** slotEntries lists the entries a view's slots name, per slot (the single
 *  ones as one-element lists; actions are not fields). */
export function slotEntries(slots: UIViewSlots | undefined): Record<'eyebrow' | 'title' | 'titleId' | 'badges' | 'meta' | 'footer', SlotEntry[]> {
  const one = (v: unknown) => { const e = typeof v === 'string' ? slotEntry(v) : undefined; return e ? [e] : [] }
  const many = (v: unknown) => (Array.isArray(v) ? v.map(slotEntry).filter((e): e is SlotEntry => e !== undefined) : [])
  return {
    eyebrow: one(slots?.eyebrow),
    title: one(slots?.title),
    titleId: one(slots?.titleId),
    badges: many(slots?.badges),
    meta: many(slots?.meta),
    footer: many(slots?.footer),
  }
}

/** slotFields lists the field paths a view's slots name, per slot. */
export function slotFields(slots: UIViewSlots | undefined): Record<'eyebrow' | 'title' | 'titleId' | 'badges' | 'meta' | 'footer', string[]> {
  const e = slotEntries(slots)
  return Object.fromEntries(Object.entries(e).map(([k, v]) => [k, v.map((x) => x.path)])) as ReturnType<typeof slotFields>
}

/** Where a dotted path's field lives: the type declaring it, its own name,
 *  and the path of the object holding it (`release` for `release.gates`). */
export interface SlotFieldRef { parentType: SchemaTypeRef; name: string; prefix: string[] }

/** resolveSlotPath walks a dotted path through the schema — object fields
 *  and named types — from the root type: the last segment is the field, the
 *  others the objects holding it. A segment the schema does not declare
 *  stops the walk there (the field then reads as text). */
export function resolveSlotPath(schema: Schema, path: string): SlotFieldRef {
  const segs = path.split('.').filter(Boolean)
  const name = segs[segs.length - 1] ?? path
  let parentType: SchemaTypeRef = schema.root
  const prefix = segs.slice(0, -1)
  for (const seg of prefix) {
    let info: ReturnType<typeof SchemaQuery.getFieldInfo>
    try { info = SchemaQuery.getFieldInfo(schema, parentType, seg) } catch { info = undefined }
    const t = info?.fieldType as SchemaType | undefined
    if (!t) break
    if (schema.types?.[t.type] !== undefined) parentType = t.type
    else if (t.type === 'object') parentType = t as unknown as SchemaTypeRef
    else break
  }
  return { parentType, name, prefix }
}

/** valueAt reads a dotted path in an item. */
export function valueAt(item: unknown, path: string[]): unknown {
  let cur: unknown = item
  for (const seg of path) {
    if (cur === null || cur === undefined || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[seg]
  }
  return cur
}

/** isEmptyValue: what a slot leaves out — nothing, an empty text, an empty list. */
export function isEmptyValue(v: unknown): boolean {
  return v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
}

/** metaLine joins the meta texts that are set by « · ». */
export function metaLine(values: unknown[]): string {
  return values.filter((v) => !isEmptyValue(v)).map(String).join(' · ')
}
