// SPDX-License-Identifier: Apache-2.0
// Field-content-type factory — co-located static map of all available
// field renderers in this folder. Reorganized 2026-06-02: this file
// replaces the field-renderer portion of the (now-removed)
// src/registry-wire.ts. The dispatch mechanism (createFieldRenderer,
// createViewRenderer, createTableRenderers) lives next to it in
// `./orchestrator.ts`.
//
// AUTHORING CONTRACT:
// When a field renderer is added:
//   1. Author `<name>/renderer.tsx` exporting a default
//      `FieldRendererFactory<SType, CType>`.
//   2. Add an import + an entry in the FIELD_FACTORIES map below.
//   3. Re-bundle (server restart or registry hot-reload).
//
// At v1 the catalog is short (8 wired renderers; ~20 more
// content-type folders ship manifest-only stubs awaiting renderers).
// When it grows past ~40 entries, switch to a build-time codegen
// step that generates this file from a folder scan.

import BooleanRenderer from './boolean/renderer.tsx'
import DateRenderer from './date/renderer.tsx'
import EmailRenderer from './email/renderer.tsx'
import EnumMultiRenderer from './enum-multi/renderer.tsx'
import FibonacciRenderer from './fibonacci/renderer.tsx'
import JsonRenderer from './json/renderer.tsx'
import MarkdownRenderer from './markdown/renderer.tsx'
import NumberRenderer from './number/renderer.tsx'
import ObjectRenderer from './object/renderer.tsx'
import ObjectRefRenderer from './object-ref/renderer.tsx'
import PanelRefRenderer from './panel-ref/renderer.tsx'
import RatingRenderer from './rating/renderer.tsx'
import SliderRenderer from './slider/renderer.tsx'
import SourceSetRenderer from './source-set/renderer.tsx'
import TypedTagsRenderer from './typed-tags/renderer.tsx'
import TextRenderer from './text/renderer.tsx'
import CodRenderer from './cod/renderer.tsx'
import GridRenderer from './grid/renderer.tsx'
import TableRenderer from './table/renderer.tsx'
import TreeRenderer from './tree/renderer.tsx'
// The planning marks, in the Workflows view's palette.
import StatusRenderer from './status/renderer.tsx'
import ObjectIdRenderer from './object-id/renderer.tsx'
import PriorityRenderer from './priority/renderer.tsx'
import StageRenderer from './stage/renderer.tsx'
import ProgressSegmentsRenderer from './progress-segments/renderer.tsx'
import GateDotsRenderer from './gate-dots/renderer.tsx'
import StepperRenderer from './stepper/renderer.tsx'
import AlertRenderer from './alert/renderer.tsx'
import IconRenderer from './icon/renderer.tsx'

import type { FieldRendererFactory } from './renderer.ts'
import type { CrudFieldInfo } from './info.ts'
import { setRendererLookup } from './orchestrator.ts'

/** Default per-base-kind content-type names used by `wireFieldRenderers`
 *  as the fallback when a field's declared content.type has no exact
 *  match in `FIELD_FACTORIES`. Authors get a usable input + a console
 *  warning instead of a render crash. */
export const DEFAULT_FIELD_BY_BASE_KIND = {
  string:  'text',
  number:  'number',
  boolean: 'boolean',
  // cwm-v3 US-4 Tier 1 (2026-06-07): arrays default to enum-multi
  // (MultiSelect / TagsInput depending on item enum constraint).
  // Covers `string[]` (Dossier parent_da, free-form tags) AND
  // arrays of enum values. Without this, array fields fell through
  // to the text renderer which stringified them — useless.
  array:   'enum-multi',
} as const

/** Map from field content-type name → its FieldRendererFactory bundle.
 *  Add a new entry when authoring `<name>/renderer.tsx`. */
const FIELD_FACTORIES: Record<string, FieldRendererFactory<any, any, any>> = {
  boolean:      BooleanRenderer,
  date:         DateRenderer,
  // `string@date-time` / `string@time` emit these variant content-types (the
  // canonical model is `date` + a pattern; the shorthand carries the variant
  // on `type`). The date renderer infers the pattern from it.
  'date-time':  DateRenderer,
  time:         DateRenderer,
  email:        EmailRenderer,
  'enum-multi': EnumMultiRenderer,
  // Modified-Fibonacci point picker — the CoD primitives + any story-
  // point field. Select of 1/2/3/5/8/13/20.
  fibonacci:    FibonacciRenderer,
  // `json` is the fallback content-type for nested user-defined
  // objects + `any`/`null` shapes — wired by `info.ts` when the
  // schema's content lookup misses. Textarea + JSON pretty-print in
  // view. Replaced per-call by US-S4's recursive nested-form renderer
  // (deferred).
  json:         JsonRenderer,
  markdown:     MarkdownRenderer,
  number:       NumberRenderer,
  // Recursive sub-form / list editor for nested user-defined object
  // types + arrays of them (US-S4). Routed here by info.ts when a
  // field's resolved type is an object the content catalog misses.
  object:       ObjectRenderer,
  // A reference to an object (its id) / to one of its panels (`<id>#<panel>`):
  // searched through the host (ui-core/host), a text input without one.
  'object-ref': ObjectRefRenderer,
  'source-set': SourceSetRenderer,
  'typed-tags': TypedTagsRenderer,
  'panel-ref':  PanelRefRenderer,
  // Array-of-objects display family (info.ts routes array-of-objects here,
  // default 'grid'). All three reuse the object renderer's FORM (basic card
  // editor); only the VIEW differs. `extension` (named on the item type OR
  // inline) picks the ui-table / ui-tree-table spec.
  grid:         GridRenderer,     // object cards, each via a `ui-view` (default)
  table:        TableRenderer,    // flat table (ui-table)
  tree:         TreeRenderer,     // tree-table (ui-tree-table)
  rating:       RatingRenderer,
  slider:       SliderRenderer,
  text:         TextRenderer,
  // Read-only computed Cost of Delay (method-core): value+criticality+risk,
  // read from sibling primitive fields. Live in the form.
  cod:          CodRenderer,
  // The planning marks: chips, the segmented bar, gate dots
  // and the stepper, in one palette (_planning/palette.ts).
  status:       StatusRenderer,
  'object-id':  ObjectIdRenderer,
  priority:     PriorityRenderer,
  stage:        StageRenderer,
  'progress-segments': ProgressSegmentsRenderer,
  'gate-dots':  GateDotsRenderer,
  stepper:      StepperRenderer,
  alert:        AlertRenderer,
  // A Tabler icon by name (a row's open mark before its title).
  icon:         IconRenderer,
}

/** Wire the static FIELD_FACTORIES map into the orchestrator's lookup.
 *
 *  Fallback semantics: when no factory is registered for the
 *  content-type declared on a field, we fall back to a factory that
 *  matches the field's base schema kind (`string`/`number`/`boolean`)
 *  — text, number, boolean respectively. Authors get a usable input +
 *  a console warning instead of a hard render-throw that crashes the
 *  entire form tree (and silently blanks the iframe because the
 *  `ready` postMessage never fires after a React unmount). The first
 *  missing-renderer surfaced was `content: 'date'` on the
 *  display-demo art-shape1 form — observed 2026-05-28; the date
 *  renderer is now wired, but the fallback prevents the same class of
 *  bug from re-occurring for any future content-type that ships
 *  unwired.
 */
export function wireFieldRenderers(): void {
  setRendererLookup((info: CrudFieldInfo) => {
    const ctName = info.contentType.type
    const exact = FIELD_FACTORIES[ctName]
    if (exact) return exact
    // Fallback by schema base kind via the DEFAULT_FIELD_BY_BASE_KIND
    // table. `info.type.type` is the schema kind discriminator
    // (`string` / `number` / `boolean` / …); we coerce it to its plain
    // renderer. The renderers themselves don't care about the unknown
    // ContentType — they only read fields they understand.
    const baseKind = info.type.type as keyof typeof DEFAULT_FIELD_BY_BASE_KIND
    const fallbackName = DEFAULT_FIELD_BY_BASE_KIND[baseKind]
    const fallback = fallbackName ? FIELD_FACTORIES[fallbackName] : undefined
    if (fallback) {
      // eslint-disable-next-line no-console
      console.warn(
        `[ktw-display] No renderer registered for content.type=${ctName} (field=${info.name}, kind=${baseKind}); falling back to the plain ${baseKind} renderer. Wire the dedicated renderer in ui-builders/fields/factory.ts to silence this warning.`,
      )
      return fallback
    }
    return undefined
  })
}

/** Names of all content types that currently have a wired field
 *  renderer. Used by the harness for diagnostics. */
export function wiredRendererNames(): string[] {
  return Object.keys(FIELD_FACTORIES)
}
