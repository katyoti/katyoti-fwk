// SPDX-License-Identifier: Apache-2.0
// What a pane's header says of its object beyond its name
// (the object-levels mock-up): its chips - the level its type gives it, the
// solution it requires, its parent - and the capabilities its type carries,
// listed in the header's « Capabilities » box. Aspects stay chips of their
// own (dashed), removable where they are drawn.

import type { DeclaredTypeStyle } from './type-style'

/** One chip of the header: what it says, and the object it opens (a parent). */
export interface HeaderChip {
  kind: 'level' | 'requires' | 'parent'
  value: string
  opens?: string
}

/** The header's chips, in the mock-up's order: level, requires, parent. */
export function headerChips(detail: { parentId?: string }, style?: DeclaredTypeStyle): HeaderChip[] {
  const chips: HeaderChip[] = []
  if (style?.level) chips.push({ kind: 'level', value: style.level })
  if (style?.solution) chips.push({ kind: 'requires', value: style.solution })
  if (detail.parentId) chips.push({ kind: 'parent', value: detail.parentId, opens: detail.parentId })
  return chips
}

type CapInfo = { id: string; source?: string }

/** The capabilities the object's type carries - the « Capabilities » box; an
 *  aspect's or the object's own stay badges beside its aspects. */
export function typeCarries<T extends CapInfo>(infos: T[]): T[] {
  return infos.filter((c) => !c.source || c.source === 'type')
}

/** The capabilities an aspect, the object itself or an attachment adds. */
export function addedCapabilities<T extends CapInfo>(infos: T[]): T[] {
  return infos.filter((c) => !!c.source && c.source !== 'type')
}

/** What the generic header draws besides the type's icon, its type, the
 *  object's name, its state pill and the pane's actions. */
export interface HeaderParts {
  /** The type as an engineer's eyebrow (capitals, monospace), else as the
   *  plain words of mock-up v6 (« Étude de l'existant »). */
  eyebrowCaps: boolean
  /** The chips of its level, the solution it requires, its parent. */
  chips: boolean
  /** Its description line. */
  description: boolean
  /** The id line: its id, project, module, tags, dates, added capabilities
   *  and « + Aspect ». */
  idLine: boolean
}

/** The header's parts at the person's display level: at Essentiel the
 *  non-expert header of mock-up v6 - its type in plain words, its name and
 *  its state pill, nothing more (no level / parent chips, no description, no
 *  id line; the aspects and the « Capacités » box are already non-essential
 *  elements); at Standard and Expert the object-levels header, whole. */
export function headerParts(level: 'essential' | 'standard' | 'expert'): HeaderParts {
  const full = level !== 'essential'
  return { eyebrowCaps: full, chips: full, description: full, idLine: full }
}
