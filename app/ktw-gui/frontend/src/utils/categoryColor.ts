// SPDX-License-Identifier: Apache-2.0
/**
 * Deterministic color picker for task categories.
 *
 * Same category → same Mantine palette color, every render. Used by the task
 * gallery to tint category badges so the user can scan groups visually.
 *
 * Excludes `dark` and `gray` — those are reserved for neutrals/disabled UI.
 * 11 colored palettes give us enough range for typical project category
 * counts (~7-10) while keeping collisions rare.
 */
const PALETTE = [
  'blue',
  'cyan',
  'green',
  'grape',
  'indigo',
  'lime',
  'orange',
  'pink',
  'red',
  'teal',
  'violet',
  'yellow',
] as const

export type CategoryColor = (typeof PALETTE)[number] | 'gray'

export function categoryToColor(category: string | undefined | null): CategoryColor {
  if (!category) return 'gray'
  let h = 0
  for (const c of category) {
    h = (h * 31 + c.charCodeAt(0)) >>> 0
  }
  return PALETTE[h % PALETTE.length]
}
