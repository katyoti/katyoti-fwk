// SPDX-License-Identifier: Apache-2.0
// `slots` view-layout — a compact card or header whose
// fields are placed in named slots rather than listed with labels:
//
//   eyebrow   a small mono line (an id)
//   title     bold, with titleId beside it (mono, dimmed) and the badges
//   meta      one dimmed line, the values that are set joined by « · »
//   footer    full-width rows (a stepper, gate dots)
//   actions   the element's actions inline: a choice as a compact select,
//             a prompt or plain action as a button (a `primary` one filled,
//             the `menu` ones under ⋯) - a row under the rest, or at the
//             header's top right with `actionsAt: top`
//
// A slot names a field by its path (`release.gates` reaches into a nested
// object), or by `{field, tone}` to draw its text in a palette tone (`plan`,
// or a status word: `planned`); the field is drawn by its content renderer's view, without the
// label row. A plain-text field is drawn as text in the slot's typography —
// a badge as a neutral chip. The layout draws no frame: the host (a lane, a
// timeline card, a page card) frames it.

import { Fragment, type CSSProperties, type ReactNode } from 'react'

import type { JsonObject } from '../../../../types/json-types.ts'
import { getFactory } from '../../../fields/index.ts'
import type { UIFieldDef } from '../../../fields/index.ts'
import { Chip, IdText, MONO } from '../../../fields/_planning/marks.tsx'
import { color } from '../../../fields/_planning/palette.ts'
import { ActionBar, type UIActionEntry } from '../../../page/nodes/choice.tsx'
import type { ViewLayoutContext } from '../factory.ts'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'
import { isEmptyValue, resolveSlotPath, slotEntries, type SlotEntry, valueAt } from './slots.ts'

/** A slot's field, ready to draw: its value in an item, and how it draws. */
interface SlotField {
  path: string
  /** The palette token its text is drawn in, if any. */
  tone?: string
  value: (item: unknown) => unknown
  /** The field's content-type name (`text` for a plain string). */
  content: string
  draw: (item: unknown) => ReactNode
}

const PLAIN = new Set(['text', 'json'])

function buildSlotField(ctx: ViewLayoutContext, entry: SlotEntry): SlotField {
  const { path, tone } = entry
  const ref = resolveSlotPath(ctx.schema, path)
  const value = (item: unknown) => valueAt(item, [...ref.prefix, ref.name])
  const holder = (item: unknown) => (valueAt(item, ref.prefix) ?? {}) as JsonObject
  try {
    const def: UIFieldDef = { id: path, name: ref.name }
    const { factory, info } = getFactory(ctx.schema, ref.name, def, ref.parentType)
    const enumerated = Array.isArray((info.type as { enum?: unknown[] }).enum)
    const content = enumerated ? 'enum' : info.contentType.type
    const inner = factory.view({ def, info, texts: ctx.texts })
    return { path, tone, value, content, draw: (item) => inner(holder(item)) }
  } catch {
    return { path, tone, value, content: 'text', draw: (item) => String(value(item) ?? '') }
  }
}

const NEUTRAL = { fg: color('muted'), border: color('line'), bg: 'transparent' }

/** A field drawn in a slot: plain text as text (a chip among badges, a mono
 *  id when the slot is an id slot), any other content by its renderer. */
function drawIn(slot: 'eyebrow' | 'title' | 'titleId' | 'badge' | 'meta' | 'footer', f: SlotField, item: unknown): ReactNode {
  const v = f.value(item)
  if (isEmptyValue(v)) return null
  if (f.tone && slot === 'badge' && PLAIN.has(f.content)) {
    const fg = color(f.tone)
    return <Chip look={{ fg, border: fg, bg: color(`${f.tone}-bg`) }} kind="badge">{String(v)}</Chip>
  }
  const node = drawPlain(slot, f, v, item)
  // A toned entry's text is drawn in its tone's colour.
  return f.tone ? <span data-tone={f.tone} style={{ color: color(f.tone) }}>{node}</span> : node
}

function drawPlain(slot: 'eyebrow' | 'title' | 'titleId' | 'badge' | 'meta' | 'footer', f: SlotField, v: unknown, item: unknown): ReactNode {
  if (slot === 'eyebrow' || slot === 'titleId') {
    if (f.content === 'object-id') return <IdText value={v} testId={`slot.${slot}`} />
    return <span style={{ fontFamily: MONO, fontSize: 11, fontWeight: 400, color: color('muted') }}>{String(v)}</span>
  }
  if (PLAIN.has(f.content)) {
    if (slot === 'badge') return <Chip look={NEUTRAL} kind="badge">{String(v)}</Chip>
    return <>{typeof v === 'object' ? JSON.stringify(v) : String(v)}</>
  }
  return f.draw(item)
}

/** keyed drops the empty slots' nulls and keys what is left. */
function keyed(nodes: ReactNode[]): ReactNode[] {
  return nodes.filter((n) => n !== null && n !== undefined && n !== false)
    .map((n, i) => <Fragment key={i}>{n}</Fragment>)
}

const slotsFactory =(_groups: UIViewGroups, ctx: ViewLayoutContext): UIViewRenderer => {
  const slots = ctx.definition.slots
  const names = slotEntries(slots)
  const fields = Object.fromEntries(
    Object.entries(names).map(([k, entries]) => [k, entries.map((e) => buildSlotField(ctx, e))]),
  ) as Record<keyof typeof names, SlotField[]>
  const actions = (slots?.actions ?? []) as unknown as UIActionEntry[]
  const top = slots?.actionsAt === 'top'

  return ({ item }) => {
    const eyebrow = keyed(fields.eyebrow.map((f) => drawIn('eyebrow', f, item)))
    const title = keyed(fields.title.map((f) => drawIn('title', f, item)))
    const titleId = keyed(fields.titleId.map((f) => drawIn('titleId', f, item)))
    const badges = keyed(fields.badges.map((f) => drawIn('badge', f, item)))
    const meta = keyed(fields.meta.map((f) => drawIn('meta', f, item)))
    const footer = keyed(fields.footer.map((f) => drawIn('footer', f, item)))
    const line: CSSProperties = { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }
    const bar = actions.length > 0 ? <ActionBar entries={actions} item={item} justify={top ? 'flex-end' : undefined} /> : null
    const head = (
      <>
        {eyebrow.length > 0 && <div data-slot="eyebrow" style={line}>{eyebrow}</div>}
        {(title.length > 0 || titleId.length > 0 || badges.length > 0) && (
          <div data-slot="title" style={line}>
            {title.length > 0 && <span style={{ fontWeight: 700, fontSize: 14 }}>{title}</span>}
            {titleId}
            {badges}
          </div>
        )}
        {meta.length > 0 && (
          <div data-slot="meta" style={{ ...line, gap: 0, color: color('muted') }}>
            {meta.map((m, i) => <span key={i}>{i > 0 ? <span style={{ padding: '0 5px' }}>·</span> : null}{m}</span>)}
          </div>
        )}
      </>
    )
    return (
      <div data-ktw-view="slots" style={{ display: 'flex', flexDirection: 'column', gap: 4, fontSize: 12.5, minWidth: 0 }}>
        {top && bar
          ? (
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: '1 1 auto', minWidth: 0 }}>{head}</div>
              <div data-slot="actions" data-at="top" style={{ flex: '0 0 auto' }}>{bar}</div>
            </div>
          )
          : head}
        {footer.map((f, i) => <div key={i} data-slot="footer" style={{ width: '100%' }}>{f}</div>)}
        {!top && bar && <div data-slot="actions">{bar}</div>}
      </div>
    )
  }
}

export default slotsFactory
