// SPDX-License-Identifier: Apache-2.0
// type-style — derive a stable icon + color from any type string.
//
// The CWM type system is open: workspace owners can add new node, item,
// and edge types (see `.katyoti/cwm/_kinds/*.yaml`). The GUI MUST NOT
// hardcode a "workspace | module | project | task | note" enum — new
// types should render with the same visual conventions as built-ins.
//
// Strategy: a type draws the icon it declares (else one default icon) and
// its declared colour, else a Mantine palette colour hashed from its name, so the
// same type always looks the same across the UI without any per-type
// registration. When CWM eventually exposes a type-discovery method
// (`cwm:/cwm/NodeTypeList`, etc.) we can switch over to real metadata
// (icon names, descriptions, colors from the kind YAML).
//
// Per user-feedback 2026-05-19: "attention to not hardcode node type,
// item type, edge types, they must be provided allowing new types to
// be handled".

import { IconFile } from '@tabler/icons-react'
import type { TablerIcon } from '@tabler/icons-react'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'

/** What a type declares of its look: a tabler icon
 *  name, a palette colour and its label. A type declaring none is drawn with the
 *  default icon and a colour hashed from its name below. */
export interface DeclaredTypeStyle {
  icon?: string
  color?: string
  label?: string
  /** Its one-line description - a « + New » item's second line. */
  description?: string
  /** The task that creates one, as an assistant. */
  createTask?: string
  /** The level its capabilities give it: project or module. */
  level?: 'project' | 'module'
  /** The solution a project of this type needs. */
  solution?: string
  /** Its plural label - a tree group's header. */
  labelPlural?: string
  /** The child types in the order the tree groups them. */
  childOrder?: string[]
}

// Mantine palette colors that read well on both light and dark themes.
// Order kept stable so type → color mapping doesn't shift between
// releases (a workspace owner expects "task" to always be the same
// color in their UI).
const COLORS = [
  'blue',
  'teal',
  'green',
  'lime',
  'yellow',
  'orange',
  'red',
  'pink',
  'grape',
  'violet',
  'indigo',
  'cyan',
]

/** FNV-1a 32-bit. Tiny, deterministic, no deps. */
function hash(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = (h * 0x01000193) >>> 0
  }
  return h
}

/** The icon a type is drawn with: the one it declares, else one default icon
 *  - never an icon hashed from its name: a renamed
 *  type keeps its look by declaring it. */
export function iconForType(_type: string, declared?: DeclaredTypeStyle): TablerIcon {
  // The library resolves the name against the same tabler set (its icon type
  // is declared apart from the GUI's own).
  return declared?.icon ? tablerIcon(declared.icon, IconFile as never) as unknown as TablerIcon : IconFile
}

/** The Mantine colour a type is drawn in: the one it declares, else a stable
 *  one hashed from its name. */
export function colorForType(type: string, declared?: DeclaredTypeStyle): string {
  if (declared?.color) return declared.color
  if (!type) return 'gray'
  return COLORS[hash(type) % COLORS.length]
}

/** The name a type is shown by: the label it declares, else its name. */
export function labelForType(type: string, declared?: DeclaredTypeStyle): string {
  return declared?.label || type
}
