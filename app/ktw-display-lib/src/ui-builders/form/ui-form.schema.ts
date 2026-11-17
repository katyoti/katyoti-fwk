// SPDX-License-Identifier: Apache-2.0
// UIFormDef — the DSL `ui-form` extension shape (per modules/framework/schema/dsl-ui-extensions.md).
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/ui-form.schema.ts during
// gui-form Subtask 3 (2026-05-03). DSL-aligned renames vs the source:
//   - `submitLabel` → `submit_label`
//   - `cancelLabel` → `cancel_label`
//   - `showErrorSummary` → `show_error_summary`
//   - `nbCols` → `cols` (group-level)
//   - `defaultCollapsed` → `collapsed` (group-level)
//   - `notNew` / `notEdit` (group-level) → `visibility?: SchemaFieldVisibility[]`
//
// Field-level visibility on `UIFieldDef` is already handled by
// `src/ui-core/fields/types.ts` (positive `visibility[]` already in place
// since the M7 cleanup).

import type { SchemaFieldVisibility, SchemaTypeExtension } from '../../schema/index.ts'
import type { UIFieldRef } from '../fields/index.ts'
import type { UIActionRef } from '../../ui-core/action/index.ts'

/**
 * Group definition — one section/tab/step inside a form.
 * Schema-author-facing shape (matches DSL §9.1's `groups[]` items).
 */
export type UIFormGroupDef = {
  title?: string
  description?: string

  /** Mantine Grid span (1-12) — how wide this group sits in the form's grid. */
  md?: number

  /** Number of columns inside this group's SimpleGrid (was `nbCols`). */
  cols?: number

  /** Which CRUD modes show this group. Replaces the legacy `notNew`/`notEdit`
   *  booleans. Aligns with the schema-side `SchemaFieldVisibility[]`. */
  visibility?: SchemaFieldVisibility[]

  /** Field refs in this group — string (id) or full UIFieldDef. */
  fields?: UIFieldRef[]

  /** Renders the group as a collapsible section (only honored by layouts
   *  that support it — `groups` does, `single`/`tabs` may not). */
  collapsible?: boolean

  /** Initial collapsed state when `collapsible: true`. Was `defaultCollapsed`. */
  collapsed?: boolean

  /** `row`: each field read as a row - its label and hint, its value read,
   *  a pencil opening its editor with OK / Annuler (a `readOnly` field has
   *  none); an array shown as a table edits its rows in place.
   *  Overrides the form's `edit`. */
  edit?: 'row'

  /** What the group's tab counts after its title: a field path (dotted,
   *  `plan.steps`) whose list length it shows, `false` for no count.
   *  Omitted: the rows of the group's array fields. */
  count?: string | false
}

/**
 * UIFormDef — the schema `ui-form` extension. Authors attach via
 * `extensions: [{ type: 'ui-form', name: 'default', layout: 'groups', ... }]`.
 */
export type UIFormDef = SchemaTypeExtension<{
  type: 'ui-form'

  /** Form name in case of multi-form. Defaults to `'default'` when omitted —
   *  the form widget exposes `?form=<name>` query param to pick a non-default
   *  form, falling back to `'default'`. */
  name?: string

  /** Layout strategy. Looked up against `getFormLayoutRegistry()`; default
   *  comes from `LAYOUT_DEFAULTS.default` ('groups'). */
  layout?: 'single' | 'groups' | 'tabs' | 'wizard'

  /** Form sections. When omitted, the builder synthesizes a default group
   *  containing all fields. */
  groups?: UIFormGroupDef[]

  /** Action refs (string id, UIActionDef, or full UIAction). When omitted,
   *  the form's standard action set is used (defaultFormActions). */
  actions?: UIActionRef[]

  /** Show the form-level error summary above the submit button. */
  show_error_summary?: boolean

  /** Override for the submit button label (otherwise from i18n
   *  `buttons.save` or per-action `label`). */
  submit_label?: string

  /** Override for the cancel button label. */
  cancel_label?: string

  /** `change`: the form saves each change as it is made - its submit run
   *  with the value a moment after the person stops typing, while no field
   *  shows an error on its line. */
  save?: 'change'

  /** `row`: every group's fields read as rows edited behind a pencil (see
   *  UIFormGroupDef.edit); a group may set its own. */
  edit?: 'row'
}>

/** UIFormRef — schema attaches this to ui-form extension lookups.
 *  - undefined: pick the default form
 *  - string: pick the named form
 *  - UIFormDef: use this object directly */
export type UIFormRef = string | UIFormDef
