// SPDX-License-Identifier: Apache-2.0
// Factory orchestrator — resolves (schema + UIFieldDef|UIColumnDef) into a
// concrete render function by looking up the right renderer.tsx via the
// content-types registry and invoking the appropriate sub-factory.
//
// Top-level helpers:
//   createFieldRenderer(schema, fieldName, def, texts)  → UIFormFieldRenderer
//   createViewRenderer(schema, fieldName, def, texts)   → UIViewFieldRenderer
//   createTableRenderers(schema, fieldName, def, texts) → { renderHeader, renderCell }
//
// Each calls `getInfo()` to build the CrudFieldInfo, then `lookupRenderer()`
// (defined below) to find the FieldRendererFactory bundle from the registry.

import type { JsonObject } from '../../types/json-types.ts'
import type { Schema, SchemaTypeRef } from '../../schema/schema-types.ts'
import type { FieldOptionList } from '../../schema/schema-contents.ts'
import type { UITexts } from '../../ui-core/i18n/types.ts'
import { withPathValue } from './editing.ts'
import { readView } from './read-value.tsx'
import { getInfo, type CrudFieldInfo } from './info.ts'
import type { FieldRendererFactory } from './renderer.ts'
import type {
  UIColumnDef,
  UIColumnRenderer,
  UIFieldDef,
  UIFormFieldRenderer,
  UIHeaderRenderer,
  UIViewFieldRenderer,
} from './types.ts'
import { wrapFieldLine } from './field-line.tsx'
import { wrapWithViewFieldRow } from './view-field-row.tsx'

/** Renderer lookup function — delegates to whatever provider the runtime sets up.
 *  In the iframe runtime (S5r2) this is wired to import all `content-types/<name>/
 *  renderer.tsx` files at bundle time. In tests it's a manual map. */
export type RendererLookup = (info: CrudFieldInfo) => FieldRendererFactory | undefined

let activeLookup: RendererLookup | undefined

/** Wire the active renderer lookup at runtime startup. */
export function setRendererLookup(lookup: RendererLookup): void {
  activeLookup = lookup
}

/** Resolve a (schema, fieldName, def) to its CrudFieldInfo + factory.
 *  `parentType` (optional) — pass the resolved row type when the schema
 *  root is an Array. Otherwise field-info lookups default to root. */
export function getFactory(
  schema: Schema,
  fieldName: string,
  def: UIFieldDef | UIColumnDef,
  parentType?: SchemaTypeRef,
): { factory: FieldRendererFactory; info: CrudFieldInfo } {
  if (!activeLookup) {
    throw new Error('No renderer lookup wired. Call setRendererLookup() at runtime startup.')
  }
  const info = getInfo(schema, fieldName, def, parentType)
  const factory = activeLookup(info)
  if (!factory) {
    throw new Error(
      `No renderer registered for field ${fieldName} (type=${info.type.type}, content=${info.contentType.type})`,
    )
  }
  return { factory, info }
}

export function createFieldRenderer<TData extends JsonObject = JsonObject>(
  schema: Schema,
  fieldName: string,
  def: UIFieldDef,
  texts: UITexts,
  parentType?: SchemaTypeRef,
): UIFormFieldRenderer<TData> {
  const { factory, info } = getFactory(schema, fieldName, def, parentType)
  const name = def.name || def.id || fieldName
  // `descriptionAt: beside`: the label line is drawn here (field-line.tsx)
  // and the editor bare below it; a field editing in place keeps its own.
  const inPlace = info.contentType.type === 'table'
  const beside = def.descriptionAt === 'beside' && !inPlace && !!info.label
  const edit = beside ? getFactory(schema, fieldName, { ...def, label: '', description: '' }, parentType) : { factory, info }
  const inner = (edit.factory.form as FieldRendererFactory<any, any, TData>['form'])({ def: beside ? { ...def, label: '', description: '' } : def, info: edit.info, texts })
  const checked = !!def.check && typeof def.check.uri === 'string' && def.check.uri !== ''
  return wrapFieldLine<TData>(inner, {
    name,
    texts,
    ...(beside ? { beside: { label: info.label, description: info.description, required: def.required ?? !info.type.optional } } : {}),
    checked,
    ...(checked && def.derive?.from ? { source: def.derive.fromLabel ?? sourceLabel(schema, def.derive.from, parentType) } : {}),
  })
}

/** How a derived field's line names its source: the source field's label,
 *  lower-cased (« titre »); its name when it has none. */
function sourceLabel(schema: Schema, from: string, parentType?: SchemaTypeRef): string {
  try {
    return getInfo(schema, from, { id: from }, parentType).label.toLowerCase() || from
  } catch {
    return from
  }
}

export function createViewRenderer<TData extends JsonObject = JsonObject>(
  schema: Schema,
  fieldName: string,
  def: UIFieldDef,
  texts: UITexts,
  parentType?: SchemaTypeRef,
): UIViewFieldRenderer<TData> {
  const { factory, info } = getFactory(schema, fieldName, def, parentType)
  const inner = readAtPath((factory.view as FieldRendererFactory<any, any, TData>['view'])({ def, info, texts }), info.name)
  // Wrap with the central label + description framing — the form side
  // gets this for free from Mantine's `<TextInput label= description=>`
  // shape; mirroring it here gives view fields the same visual hierarchy
  // (label sm/500 on top, description xs/dimmed, value below). The JSX
  // wrap lives in view-field-row.tsx because this file is `.ts`.
  return wrapWithViewFieldRow<TData>(inner, info.label, info.description)
}

export function createTableRenderers<TData extends JsonObject = JsonObject>(
  schema: Schema,
  fieldName: string,
  def: UIColumnDef,
  texts: UITexts,
  parentType?: SchemaTypeRef,
): { renderHeader: UIHeaderRenderer; renderCell: UIColumnRenderer<TData> } {
  const { factory, info } = getFactory(schema, fieldName, def, parentType)
  return {
    renderHeader: factory.header({ def, info, texts }),
    renderCell: (factory.column as FieldRendererFactory<any, any, TData>['column'])({
      def,
      info,
      texts,
    }),
  }
}

/** A view renderer of a nested field (a dotted name) reads its value at
 *  the path; a plain name is read as it is. */
function readAtPath<TData>(view: UIViewFieldRenderer<TData>, name: string): UIViewFieldRenderer<TData> {
  return name.includes('.') ? (item) => view(withPathValue(item, name)) : view
}

/** A field drawn as a read row (ui-form `edit: row`): its
 *  label and hint, its value read by its view renderer (no label of its
 *  own; drawn as its `read` declares - read-value.tsx), and its form
 *  renderer drawn bare in the row behind the pencil.
 *  `inPlace`: the field edits in place itself (an array as a table) - the
 *  row draws its form, never a pencil. */
export type UIFormRowRenderers<TData extends JsonObject = JsonObject> = {
  name: string
  label: string
  description?: string
  readOnly: boolean
  inPlace: boolean
  /** The host note shown under the open editor (UIFieldDef.note). */
  note?: { uri: string; args?: Record<string, unknown> }
  /** The page action the pencil runs in place of the editor (UIFieldDef.action). */
  action?: UIFieldDef['action']
  /** The button drawn in place of the pencil (UIFieldDef.button). */
  button?: UIFieldDef['button']
  /** The host method the field's choices come from (UIFieldDef.choices). */
  choices?: UIFieldDef['choices']
  /** The same row with these options listed (the choices' answer). */
  withOptions?: (options: FieldOptionList) => UIFormRowRenderers<TData>
  view: UIViewFieldRenderer<TData>
  form: UIFormFieldRenderer<TData>
}

export function createRowRenderers<TData extends JsonObject = JsonObject>(
  schema: Schema,
  fieldName: string,
  def: UIFieldDef,
  texts: UITexts,
  parentType?: SchemaTypeRef,
): UIFormRowRenderers<TData> {
  const { factory, info } = getFactory(schema, fieldName, def, parentType)
  const plain = readAtPath((factory.view as FieldRendererFactory<any, any, TData>['view'])({ def, info, texts }), info.name)
  const view = def.read ? readView(def.read, plain, info.name, `form.row.${info.name}`, info.options, texts.language) : plain
  const inPlace = info.contentType.type === 'table'
  // The editor in the row carries no label of its own: the row's is beside it.
  const bare: UIFieldDef = inPlace ? def : { ...def, label: '', description: '' }
  const edit = inPlace ? { factory, info } : getFactory(schema, fieldName, bare, parentType)
  const form = (edit.factory.form as FieldRendererFactory<any, any, TData>['form'])({ def: bare, info: edit.info, texts })
  return {
    name: info.name, label: info.label, description: info.description, readOnly: def.readOnly === true, inPlace,
    ...(def.note?.uri ? { note: def.note } : {}),
    ...(def.action ? { action: def.action } : {}),
    ...(def.button?.uri ? { button: def.button } : {}),
    ...(def.choices?.uri ? {
      choices: def.choices,
      withOptions: (options: FieldOptionList) => createRowRenderers<TData>(schema, fieldName, { ...def, options, choices: undefined }, texts, parentType),
    } : {}),
    view, form,
  }
}
