// SPDX-License-Identifier: Apache-2.0
// Renderer factory contracts.
//
// A FieldRendererFactory<SType, CType> is what each `content-types/<name>/
// renderer.tsx` default-exports. It bundles four sub-factories — `view`,
// `form`, `header`, `column` — and each sub-factory is called with the
// unified params shape `{ def, info, texts }`:
//
//   - `def` is the declarative UIFieldDef or UIColumnDef (whichever applies)
//   - `info` is the renderer-ready CrudFieldInfo (carries `info.schema` and `info.name`)
//   - `texts` is the i18n facade (see ../i18n/types.ts)
//
// The sub-factories return render functions:
//
//   - view   → (item) => ReactNode               (read-only display)
//   - form   → (item, isNew, isSubmitting) => ReactNode   (editable, react-hook-form bound)
//   - header → () => ReactNode                   (table header cell)
//   - column → (item, index) => ReactNode        (table body cell)
//
// The `def` type is a union (UIFieldDef | UIColumnDef); each sub-factory
// narrows it via its own typed alias for clarity (FormFactoryParams uses
// UIFieldDef, ColumnFactoryParams uses UIColumnDef).

import type { ContentType } from '../../schema/schema-contents.ts'
import type { JsonObject } from '../../types/json-types.ts'
import type { SchemaType } from '../../schema/schema-types.ts'
import type { UITexts } from '../../ui-core/i18n/types.ts'
import type { CrudFieldInfo } from './info.ts'
import type {
  UIColumnDef,
  UIColumnRenderer,
  UIFieldDef,
  UIFormFieldRenderer,
  UIHeaderRenderer,
  UIViewFieldRenderer,
} from './types.ts'

// ---- factory params (per surface) ----

export type ViewFactoryParams<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = {
  def: UIFieldDef
  info: CrudFieldInfo<SType, CType>
  texts: UITexts
}

export type FormFactoryParams<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = {
  def: UIFieldDef
  info: CrudFieldInfo<SType, CType>
  texts: UITexts
}

export type ColumnFactoryParams<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = {
  def: UIColumnDef
  info: CrudFieldInfo<SType, CType>
  texts: UITexts
}

// Header takes the same params shape as Column (it dispatches off columnDef.tooltip,
// columnInfo.label, etc.). Aliased for readability at call sites.
export type HeaderFactoryParams<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = ColumnFactoryParams<SType, CType>

// ---- sub-factory signatures (each returns a render function) ----

export type ViewFieldRendererFactory<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
  IType extends JsonObject = JsonObject,
> = (params: ViewFactoryParams<SType, CType>) => UIViewFieldRenderer<IType>

export type FormFieldRendererFactory<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
  IType extends JsonObject = JsonObject,
> = (params: FormFactoryParams<SType, CType>) => UIFormFieldRenderer<IType>

export type HeaderRendererFactory<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
> = (params: HeaderFactoryParams<SType, CType>) => UIHeaderRenderer

export type ColumnRendererFactory<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
  IType extends JsonObject = JsonObject,
> = (params: ColumnFactoryParams<SType, CType>) => UIColumnRenderer<IType>

// ---- the top-level factory bundle ----

/**
 * The default export shape of every `content-types/<name>/renderer.tsx`.
 * Bundles all 4 surface factories — view / form / header / column. Each
 * surface is independently invoked with `{ def, info, texts }`.
 */
export type FieldRendererFactory<
  SType extends SchemaType = SchemaType,
  CType extends ContentType = ContentType,
  IType extends JsonObject = JsonObject,
> = {
  view: ViewFieldRendererFactory<SType, CType, IType>
  form: FormFieldRendererFactory<SType, CType, IType>
  header: HeaderRendererFactory<SType, CType>
  column: ColumnRendererFactory<SType, CType, IType>
}
