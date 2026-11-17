// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

/**
 * Icon-flag presets supported by `editAs: 'flag'` / view-mode flag
 * rendering. Each preset maps the true / false states to a
 * pre-curated (icon, color) pair so authors don't have to specify
 * every Tabler icon by hand. Ported from the old
 * `_ktw_workspace/old_ui_main/.../boolean.tsx` `BooleanIconTypes`
 * enum.
 *
 *  - `protected` : shield-lock (true, orange) / shield (false, green)
 *  - `warning`   : bulb       (true, orange) / check  (false, green)
 *  - `checked`   : check      (true, orange) / shield (false, green)
 *  - `switch`    : toggle-right (true, orange) / toggle-left (false, green)
 *
 * The exact icons + colors are locked at the renderer (`renderer.tsx`)
 * — adding a new preset = a single map entry there.
 */
export type BooleanFlagPreset = 'protected' | 'warning' | 'checked' | 'switch'

export interface BooleanContent extends ContentType {
  type: 'boolean'
  /**
   * How to render the editor in form mode:
   *  - `'checkbox'` (default) — Mantine Checkbox.
   *  - `'switch'`             — Mantine Switch.
   *  - `'flag'`               — pair of icons (true / false visuals)
   *                             toggled by click. Pair chosen by
   *                             `flagIcon` (defaults to `'switch'`).
   *                             Compact, good for inline "is_*" /
   *                             "has_*" booleans on row-dense
   *                             surfaces. cwm-v3 US-4 Tier 1.
   */
  editAs?: 'checkbox' | 'switch' | 'flag'
  /**
   * Icon-pair preset for `editAs: 'flag'`. Ignored for other
   * `editAs` values. Each preset bundles a (true → icon+color,
   * false → icon+color) pair — see `BooleanFlagPreset` godoc above.
   */
  flagIcon?: BooleanFlagPreset
  /** Optional secondary label below the main label. */
  subLabel?: string
}
