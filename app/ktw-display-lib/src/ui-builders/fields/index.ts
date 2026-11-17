// SPDX-License-Identifier: Apache-2.0
// Barrel — re-exports the public surface of src/ui-builders/fields/.
// (Moved from src/ui-core/fields/ 2026-06-02 — the field renderer
// infrastructure now sits next to its per-content-type implementations
// under one umbrella.) Two co-located concerns:
//   - `orchestrator.ts` — the dispatch mechanism (setRendererLookup,
//     getFactory, createFieldRenderer / createViewRenderer /
//     createTableRenderers)
//   - `factory.ts` — the static FIELD_FACTORIES map and the
//     `wireFieldRenderers()` boot function that wires it into the
//     orchestrator's lookup
export type {
  UIFieldDef,
  UIFieldRef,
  UIFieldRead,
  UIColumnDef,
  UIColumnRef,
  UIColumnStyle,
  UIFormFieldRenderer,
  UIViewFieldRenderer,
  UIColumnRenderer,
  UIHeaderRenderer,
} from './types.ts'

export type { CrudFieldInfo } from './info.ts'
export { buildInfo, getInfo } from './info.ts'

export type {
  FieldRendererFactory,
  ViewFieldRendererFactory,
  FormFieldRendererFactory,
  HeaderRendererFactory,
  ColumnRendererFactory,
  ViewFactoryParams,
  FormFactoryParams,
  ColumnFactoryParams,
  HeaderFactoryParams,
} from './renderer.ts'

export {
  setRendererLookup,
  getFactory,
  createFieldRenderer,
  createViewRenderer,
  createTableRenderers,
  createRowRenderers,
} from './orchestrator.ts'
export type { RendererLookup, UIFormRowRenderers } from './orchestrator.ts'
export { FieldLineContext } from './field-line.tsx'
export type { FieldLineAnswer } from './field-line.tsx'

// `./factory.ts` is INTENTIONALLY NOT re-exported here. It static-imports
// every per-content-type `<X>/renderer.tsx`, which in turn imports from
// THIS barrel (for `createHeaderRenderer` + the renderer types). Re-
// exporting factory.ts from the barrel creates a load-time cycle that
// leaves the renderers' `header:` slot undefined and crashes the table
// widget with `factory.header is not a function`. The wire functions
// (`wireFieldRenderers`, `wiredRendererNames`) are consumed by
// `src/registry-wire.ts` via a direct deep import — no barrel round-
// trip, no cycle.

export {
  COLOR_MAP,
  translateColor,
  createHeaderRenderer,
  columnText,
  columnTextWithColor,
  columnStyles,
} from './utils.tsx'
