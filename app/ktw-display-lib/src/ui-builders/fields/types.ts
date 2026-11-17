// SPDX-License-Identifier: Apache-2.0
// UI field / column declarative types — JSON-serializable layout definitions.
//
// `UIFieldDef` (form/view) and `UIColumnDef` (table) are what callers WRITE
// to declare a field's appearance. They're plain data and travel as JSON
// across the host ↔ iframe boundary.
//
// At runtime they're combined with the schema (via SchemaQuery) into the
// renderer-ready aggregate `CrudFieldInfo` (see `./info.ts`). Each renderer
// then receives `{ def, info, texts }`.
//
// Ported from _ktw_workspace/old_ui_main/.../ui-fields/types.ts with the
// alignment decisions locked in MIGRATION.md / unify-content-types-as-folders:
//   - `notNew`/`notEdit` (negative-form booleans) → `visibility?: SchemaFieldVisibility[]`
//   - `detailLevel?: number[]` → `detail?: SchemaFieldDetailLevel` (singular 1|2|3)
//   - `UIFieldDef.name` stays singular (`SchemaFieldName`); composite columns use UIColumnDef.name array form

import type { ReactNode } from 'react'
import type {
  SchemaFieldDetailLevel,
  SchemaFieldName,
  SchemaFieldVisibility,
  SchemaTypeName,
} from '../../schema/schema-types.ts'
import type { FieldOptionList } from '../../schema/schema-contents.ts'
import type { JsonValue } from '../../types/json-types.ts'
import type { UIPageAction } from '../page/ui-page.interactive.ts'

// ----------------------------------------------------------------------------
// UI-layer display aliases
// ----------------------------------------------------------------------------
// Moved here from src/schema/schema-contents.ts during its strict cleanup
// (only the open ContentType envelope + ContentRef stay there). These are
// table / label / column display concerns — UI-layer types, not schema-layer.
//
// Layering: src/ui/ is the library; content-types/ is the catalog of
// consumers. content-types/<name>/types.ts may import from here when its
// content type wants to use these vocabulary types (e.g. LabelContent
// declaring `color?: ContentColor`). The reverse direction is forbidden.

/** Horizontal alignment for column display. */
export type ContentAlign = 'left' | 'center' | 'right'

/** Casing/transformation applied to displayed string values. */
export type ContentTransformation =
  | 'lowercase'
  | 'uppercase'
  | 'capitalize'   // capitalize first letter of every word
  | 'upper-first'  // capitalize first letter of the whole string

/** Predefined semantic colors recognized by COLOR_MAP in fields/utils.tsx. */
export type ContentColorPredefined =
  | 'primary'
  | 'secondary'
  | 'info'
  | 'success'
  | 'warning'
  | 'error'

/** Either a predefined semantic color or any free-form CSS color string. */
export type ContentColor = ContentColorPredefined | string

/** Mantine label/badge variant aliases. */
export type LabelVariant =
  | 'light'
  | 'filled'
  | 'outline'
  | 'dot'
  | 'transparent'
  | 'white'
  | 'default'
  | 'gradient'

// ============================================================================
// FORM / VIEW FIELD DEFINITION
// ============================================================================

/**
 * Form / view field definition (declarative, JSON-serializable).
 * Use to declare how a single schema field should appear in a form, view,
 * or any non-table layout.
 */
export type UIFieldDef = {
  /** Stable id for this field appearance. Defaults to the schema field name. */
  id: string

  /**
   * Schema field name to bind to. Defaults to `id`. Allows the same schema
   * field to appear multiple times under different ids/labels with different
   * renderers. Singular only — multi-source data (e.g., avatar = url + name)
   * is expressed via the content-type's own params (e.g., AvatarContent.nameKey).
   */
  name?: SchemaFieldName

  label?: string
  description?: string

  /**
   * Override the schema-resolved type. Normally we use the field's own type,
   * but in some cases you may want to render the same value as a different type
   * (e.g., a string rendered as JSON code).
   */
  type?: SchemaTypeName

  /**
   * Which CRUD modes show this field. Replaces the legacy `notNew`/`notEdit`
   * booleans. Aligns with the schema-side `SchemaFieldVisibility[]`.
   */
  visibility?: SchemaFieldVisibility[]

  /**
   * Progressive disclosure level — minimal (1), medium (2), detailed (3).
   * Singular value (matches the schema-side `SchemaFieldDetailLevel`),
   * replaces the legacy `detailLevel?: number[]`.
   */
  detail?: SchemaFieldDetailLevel

  /** Field is required. */
  required?: boolean
  /** Field is disabled (rendered, but not interactive). */
  disabled?: boolean
  /** Field is read-only (rendered, value visible, not editable). */
  readOnly?: boolean

  /** Placeholder text. */
  placeholder?: string

  /** Default value (used by forms when no initial value is provided). */
  defaultValue?: JsonValue

  /** Layout column span (form-grid or view-grid). */
  span?: number
  /** Layout order (used by form / view section ordering). */
  order?: number

  /** A form pre-fills the field from another while the person types, and
   *  keeps the person's value once they edit it. */
  derive?: { from: string; as?: 'slug' | 'copy'; fromLabel?: string }
  /** A form checks the field on its line: the host method `uri` called with
   *  `{value}` over `args`, whose `${field}` templates read the form's live
   *  values (`${code}`, `${owner.name}`; asked again when one changes);
   *  `{ok: false, message}` is the line's error, `{ok: true, text}` an
   *  accepted text under the field (green, a check icon; a derived field
   *  adds « · rempli d'après … » or « · modifié par vous »), `{text}` a
   *  muted note. An empty value is asked only with `whenEmpty: true`. */
  check?: { uri: string; args?: Record<string, unknown>; whenEmpty?: boolean }
  /** Where a form field's description sits: `below` the label (default),
   *  or `beside` it on the label's line (« Code — facultatif · … »). A
   *  field that edits in place (a table) keeps it below. */
  descriptionAt?: 'below' | 'beside'
  /** A read row shows a note under the field's open editor: the host
   *  method `uri` called with `{value}` over `args`, whose `${field}`
   *  templates read the form's live values, when the editor opens and as
   *  the person types in the field or a field the args name; the answer's
   *  `text` is shown, nothing when the call fails or answers none. */
  note?: { uri: string; args?: Record<string, unknown> }
  /** The labels of an enum field's values (`{value, label}`): read, and
   *  listed by its select, through them; the value stored stays the key.
   *  They win over the content type's options. */
  options?: FieldOptionList
  /** A read row's pencil runs this page action (open an object or one of
   *  its panels, show a tab, ...) in place of opening the field's inline
   *  editor; its `${field}` templates read the form's live values. Drawn
   *  disabled where the page cannot run it. */
  action?: UIPageAction
  /** A read row's trailing control is this button in place of the pencil:
   *  a press calls the host method `uri` with `{value}` over `args` (their
   *  `${field}` templates read the form's live values) and draws its answer
   *  in the row with the check grammar - `{ok: false, message}` refused,
   *  `{ok: true, text}` accepted, `{text}` a note. */
  button?: { label: string; uri: string; args?: Record<string, unknown> }
  /** The field's choices come from the host method `uri` called with
   *  `args` (their `${field}` templates read the form's live values; asked
   *  again when one changes): its answer `{options: [{value, label,
   *  description?}]}` lists the field's values over `options`, the value
   *  read by its label. A call that fails keeps `options`. */
  choices?: { uri: string; args?: Record<string, unknown> }
  /** How a closed read row (ui-form `edit: row`) draws the value; the
   *  editor behind the pencil stays the field's own. */
  read?: UIFieldRead
}

/** A read row's drawing of its value (UIFieldDef.read), all parts
 *  optional, an empty value drawn plain:
 *  - `template`: a worded value, `{value}` the value as text (an enum by
 *    its label) - « Au plus {value} applications »;
 *  - `pill`: a tone pill of the word at `path` in the value (the value
 *    itself without one), coloured by `tones` (a word to a colour name:
 *    teal, red, ...; grey when unlisted);
 *  - `after`: a muted note after it, the part at `path` in the value,
 *    said as a relative time (`as: 'relative-time'`, « il y a 5 min », the
 *    date its tooltip) or as it is, `prefix` before it (« testée »);
 *  - `mark`: a brand mark before the value, `letters` (else the monogram of
 *    `text`, else of the value) on `color`;
 *  - `lead`: a muted text before it all (« les mots-clés de l'étude »). */
export type UIFieldRead = {
  template?: string
  pill?: { path?: string; tones?: Record<string, string> }
  after?: { path: string; as?: 'relative-time'; prefix?: string }
  mark?: { text?: string; letters?: string; color?: string }
  lead?: string
}

export type UIFieldRef = string | UIFieldDef

/** Form-context render function — produces a Mantine field bound to react-hook-form. */
export type UIFormFieldRenderer<Item = any> = (
  item: Item,
  isNew: boolean,
  isSubmitting: boolean,
) => ReactNode

/** View-context render function — read-only display of a field value. */
export type UIViewFieldRenderer<Item = any> = (item: Item) => ReactNode

// ============================================================================
// TABLE COLUMN DEFINITION
// ============================================================================

/**
 * Per-column style. `ContentTransformation` covers `lowercase`/`uppercase`/
 * `capitalize`/`upper-first`; the rest are typographic flags.
 */
export type UIColumnStyle =
  | ContentTransformation
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'overline'

/**
 * Table column definition (declarative, JSON-serializable).
 * Like UIFieldDef but adds table-specific affordances (alignment, sorting,
 * width, multi-source name, truncation).
 */
export type UIColumnDef = {
  id: string

  /**
   * Schema field name(s) to bind to. Plural form supported — composite columns
   * can read multiple fields (e.g., a "name" column from `first_name + last_name`).
   * Multi-source semantics are renderer-specific.
   */
  name?: SchemaFieldName | SchemaFieldName[]

  /** Override the schema-resolved type (see UIFieldDef.type). */
  type?: SchemaTypeName

  /** Cell text alignment. */
  align?: ContentAlign

  /** The labels of an enum column's values (see UIFieldDef.options). */
  options?: FieldOptionList

  label?: string

  /** Pixel width or CSS dimension. */
  width?: number | string
  minWidth?: number | string

  /** Behavior toggles. */
  sortable?: boolean
  filterable?: boolean
  clickable?: boolean

  /** Free-form cell-level props (HTML-like attrs). */
  cellProps?: Record<string, JsonValue>

  /**
   * Style flags. Three accepted shapes for ergonomics:
   *   - 'bold,italic'                 (CSV string)
   *   - 'bold'                        (single value)
   *   - ['bold', 'italic']            (array)
   * `buildInfo()` normalizes to `UIColumnStyle[]`.
   */
  styles?: string | UIColumnStyle | UIColumnStyle[]

  /** Detail level (matches UIFieldDef.detail). */
  detail?: SchemaFieldDetailLevel

  /** Truncate long values (use with `maxLength`). */
  truncate?: boolean
  maxLength?: number

  /** Hide column by default; user can still toggle it. */
  hidden?: boolean
  /** Hide on mobile viewports. */
  hiddenOnMobile?: boolean

  /** Header tooltip / column help text. */
  tooltip?: string
  helpText?: string

  /** Fields drawn after the column's own value, in the same cell, each by
   *  its content renderer's column view (`{id: title, append: [id]}`: the
   *  title then the id chip). */
  append?: string[]
  /** Fields drawn before the column's own value, in the same cell. */
  prepend?: string[]
  /** Fields drawn on a second line under the column's own value, in the
   *  same cell, dimmed and smaller (`{id: title, below: [id, release]}`: the
   *  title, then the id and the release under it); an empty one takes no
   *  room. */
  below?: string[]
  /** A field of the row shown on hover over the cell's own value, wrapped in
   *  a box (a description behind a title). */
  hover?: string
}

export type UIColumnRef = string | UIColumnDef

/** Table header render function — receives no item (just the column meta). */
export type UIHeaderRenderer = () => ReactNode

/** Table cell render function — receives the row item + zero-based index. */
export type UIColumnRenderer<Item = any> = (item: Item, index: number) => ReactNode
