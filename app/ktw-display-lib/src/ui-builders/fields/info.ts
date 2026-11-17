// SPDX-License-Identifier: Apache-2.0
// CrudFieldInfo — the renderer-ready aggregate of (schema + UIFieldDef|UIColumnDef).
//
// Built once per field at render time via `buildInfo()`. Carries everything a
// renderer factory needs to render a field: the schema reference (for cross-field
// queries), the resolved type + content type, the label, options, styles, etc.
//
// The name `CrudFieldInfo` is retained from the _ktw_workspace port; a future
// rename to a name like `UIFieldInfo` is tracked separately.
//
// The factory params shape `{ def, info, texts }` (3 args) replaces the
// legacy 5-arg `{ schema, fieldName, fieldDef|columnDef, fieldInfo|columnInfo, texts }`
// since `info.schema` and `info.name` cover the schema/fieldName roles.

import type { ContentType } from '../../schema/schema-contents.ts'
import { SchemaQuery } from '../../schema/schema-query.ts'
import type {
  ArrayType,
  Schema,
  SchemaType,
  SchemaTypeRef,
} from '../../schema/schema-types.ts'
import { arrayOrString } from '../../utils/array-or-string.ts'
import type { UIColumnDef, UIColumnStyle, UIFieldDef } from './types.ts'

/**
 * Renderer-ready aggregate. Embeds the schema reference so renderers can perform
 * any schema query (cross-field lookup, type resolution, content options, …)
 * without needing schema as a separate parameter.
 */
export type CrudFieldInfo<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = {
  /** The schema this field belongs to. Embedded so renderers don't need it as
   *  a separate factory parameter. */
  schema: Schema

  /** The UIFieldDef.id / UIColumnDef.id. Stable identifier for the rendered slot. */
  id: string

  /** Schema field name (singular). For UIColumnDef-with-array name, use the
   *  first name and treat extras as supplementary (renderer-specific). */
  name: string

  /** Resolved schema type (after array-unwrap if applicable). */
  type: SType

  /** Resolved content type (the `content` of the field's primitive type, with
   *  default applied if absent). */
  contentType: CType

  /** True iff the field is declared as `array` in the schema. */
  isArray: boolean

  label: string

  /** Flat key→label map of the content-type's options (when applicable).
   *  For ordering or the all-option, query SchemaQuery.getContentTypeOptions
   *  directly via `info.schema`. */
  options?: Record<string, string>

  /** Normalized style list (parsed from UIColumnDef.styles + augmented with
   *  the content-type's `transformation`). Empty for form/view contexts. */
  styles?: UIColumnStyle[]

  description?: string
}

/**
 * Build the renderer-ready aggregate from (schema + UIFieldDef|UIColumnDef).
 * Resolves the array wrapper, looks up the content type, normalizes styles,
 * cascades the label / description through (def → type → parent).
 */
export function buildInfo(
  schema: Schema,
  id: string,
  name: string,
  type: SchemaType,
  def: UIFieldDef | UIColumnDef,
): CrudFieldInfo {
  const isArray = type.type === 'array'
  const parent = type
  let resolvedType = type
  if (isArray) {
    resolvedType = SchemaQuery.getType(schema, (type as ArrayType).items)
  }
  // Dereference a named user type (a field `type: Foo`, or an array `items: Foo`).
  // getType() returns a `{ type: 'Foo' }` field/item shape UNTOUCHED (it has a
  // `type` key), so a nested object field would otherwise carry `type: 'Foo'`
  // (≠ 'object') and fall through to the raw-JSON fallback instead of expanding
  // into a sub-form. Resolve it explicitly to its actual definition. Primitives
  // (`string`, `number`, …) live in ReferencableTypes, not `schema.types`, so
  // this skips them — only declared types get dereferenced.
  if (resolvedType.type !== 'object' && schema.types?.[resolvedType.type] !== undefined) {
    resolvedType = SchemaQuery.getType(schema, resolvedType.type)
  }
  // Preserve the FIELD's own `optional` flag on the resolved type. For an array
  // the flag rides the array wrapper (`type`), which is discarded by the unwrap
  // above; for a named ref the bare definition has no per-field flag. Renderers
  // read `info.type.optional` to compute `required`, so without this every
  // array / named-object field renders as required even when declared optional.
  // Shallow-copy (never mutate the shared stored type) and only when the
  // resolved type doesn't already carry its own flag.
  const fieldOptional = (type as { optional?: boolean }).optional
  if (fieldOptional !== undefined && (resolvedType as { optional?: boolean }).optional === undefined) {
    resolvedType = { ...resolvedType, optional: fieldOptional }
  }
  const isLink = resolvedType.type === 'link' || resolvedType.type === 'tree'
  const lookupContent = SchemaQuery.getTypeContent(schema, isLink ? 'string' : resolvedType)
  // Content resolution, in priority order:
  //   1. OBJECT item type (single sub-record OR `Foo[]` array of them) →
  //      `object` content → the recursive sub-form / list editor (US-S4).
  //      The object renderer reads `info.isArray` to pick single-form vs
  //      list-editor; either way it reuses every leaf renderer.
  //   2. ARRAY of primitives (`string[]`, `number[]`, array-of-enum) →
  //      `enum-multi` (MultiSelect / TagsInput). CRITICAL: without this,
  //      an array unwraps to its scalar item type, resolves to the scalar
  //      renderer (text/number), and renders a single text input that
  //      SAVES A SCALAR STRING — which then collides with `[]string` meta
  //      fields (tags / derived_from) and breaks YAML decode on reload.
  //   3. Single primitive → its kind-default content (text/number/…).
  //   4. Anything still unresolved (`any`/`null`) → `json` textarea.
  let contentType: ContentType
  if (isArray && resolvedType.type === 'object') {
    // Array-of-objects: honor the field's declared content (grid | table |
    // tree), defaulting to 'grid' (object card list). Preserves an opaque
    // `extension` param (named display-extension ref, or inline spec) so the
    // table/tree renderers can resolve it. Mirrors the array-of-primitives
    // branch below — arrays are a first-class content family.
    contentType = SchemaQuery.toContentEnvelope(
      parent as { content?: ContentType | string; contentParams?: Record<string, unknown> },
      'grid',
    )
  } else if (resolvedType.type === 'object') {
    contentType = { type: 'object' }
  } else if (isArray) {
    // Preserve the array type's OWN declared content envelope (e.g. an
    // `enum-multi` carrying rich `options`) — merging its opaque
    // `contentParams` — instead of discarding it. Defaults to `enum-multi`
    // when the array declares no content, so plain `string[]` is unchanged.
    contentType = SchemaQuery.toContentEnvelope(
      parent as { content?: ContentType | string; contentParams?: Record<string, unknown> },
      'enum-multi',
    )
  } else {
    contentType = lookupContent ?? { type: 'json' }
  }

  // Options declared on the field / column (value + label) win over the
  // content type's: an enum read and picked through those labels.
  const defOptions = (def as { options?: unknown }).options
  if (Array.isArray(defOptions) && defOptions.length > 0) {
    contentType = { ...contentType, options: defOptions } as ContentType
  }

  const label =
    def.label ??
    (resolvedType as { label?: string }).label ??
    (parent as { label?: string }).label ??
    name ??
    id

  const styles =
    'styles' in def && def.styles
      ? (arrayOrString(def.styles, ',', true, true) as UIColumnStyle[])
      : []

  const description =
    'description' in def && def.description !== undefined
      ? def.description
      : (resolvedType as { description?: string }).description ??
        (parent as { description?: string }).description

  if (
    contentType !== undefined &&
    'transformation' in contentType &&
    (contentType as { transformation?: string }).transformation !== undefined
  ) {
    styles.push((contentType as unknown as { transformation: UIColumnStyle }).transformation)
  }

  return {
    schema,
    id,
    name,
    type: resolvedType,
    contentType,
    isArray,
    label,
    options: SchemaQuery.getContentTypeOptions(contentType).optionMap,
    styles,
    description,
  }
}

/**
 * Resolve `(schema, fieldName, def)` to a CrudFieldInfo.
 *
 * Resolution priority:
 *   1. If `def.type` is set, use that as the explicit type override.
 *   2. Otherwise look up the field on `parentType` (defaults to `schema.root`).
 *
 * `parentType` lets table builders pass the resolved row type when the
 * schema root is an Array (the field-info lookup needs the row type, not
 * the array root). Form/view builders omit it and get the Object-root
 * default behavior.
 */
export function getInfo(
  schema: Schema,
  fieldName: string,
  def: UIFieldDef | UIColumnDef,
  parentType?: SchemaTypeRef,
): CrudFieldInfo {
  const { id, type: typeOverride } = def
  if (typeOverride && typeOverride !== '') {
    const type = SchemaQuery.getType(schema, typeOverride)
    return buildInfo(schema, id, fieldName, type, def)
  }
  const lookupType = parentType ?? schema.root
  let fieldInfo: ReturnType<typeof SchemaQuery.getFieldInfo>
  try {
    // A dotted name (`need.summary`) binds a nested field: its leaf's type.
    fieldInfo = fieldName.includes('.')
      ? SchemaQuery.getFieldInfoAt(schema, lookupType, fieldName)
      : SchemaQuery.getFieldInfo(schema, lookupType, fieldName)
  } catch {
    fieldInfo = undefined
  }
  if (fieldInfo === undefined) {
    // The column/field references something the schema doesn't declare — common
    // with thin or placeholder schemas (and with data-derived columns). Render
    // it generically as a string cell (the text renderer stringifies any value)
    // instead of throwing and white-screening the whole table.
    return buildInfo(schema, id, fieldName, { type: 'string' }, def)
  }
  return buildInfo(schema, id, fieldName, fieldInfo.fieldType, def)
}
