// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

/**
 * EnumMultiContent — content-type for multi-select array fields.
 *
 * The form picks this renderer when the underlying schema is an array
 * of string OR an array of enum values. See `factory.ts` for the
 * registration that maps `content.type === 'enum-multi'` to this
 * renderer (and the fallback that auto-routes `string[]` array fields
 * here when no explicit content-type is declared).
 */
// `options?: FieldOptions` is inherited from the generic `ContentType` envelope.
// When present it defines the BOUNDED option set (the picker shows a rich row
// per option), falling back to the array items' bare `enum` keys when absent.
export interface EnumMultiContent extends ContentType {
  type: 'enum-multi'

  /**
   * When true, the user can type a value not in the enum list and
   * it becomes a new tag. Defaults to true when no `enum:` constraint
   * is declared on the schema; defaults to false when bounded.
   */
  creatable?: boolean

  /**
   * Optional cap on number of selectable values. Empty / 0 = no
   * limit.
   */
  maxSelected?: number

  /**
   * Optional placeholder text shown when nothing is selected. Falls
   * back to a generic "Select values…" when omitted.
   */
  placeholder?: string

  /**
   * How a BOUNDED enum (items have `enum: [...]`) renders for editing:
   *   - 'multiselect' (default) — Mantine MultiSelect dropdown.
   *   - 'checkboxes' — a Checkbox.Group listing every option at once
   *      (better when the option set is small + should all be visible).
   *   - 'cards' — a grid of option cards (two columns, one on a phone):
   *      a checkbox, the option's icon, its label bold and its description;
   *      a ticked card has an accent border.
   * Ignored for free-entry (no enum) fields, which always use TagsInput.
   */
  editAs?: 'multiselect' | 'checkboxes' | 'cards'
}
