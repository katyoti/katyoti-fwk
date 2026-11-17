// SPDX-License-Identifier: Apache-2.0
// View-layout factory — co-located static map of all available view
// layouts in this folder. Reorganized 2026-06-02: this file replaces
// the layouts portion of the (now-removed) src/registry-wire.ts.
//
// AUTHORING CONTRACT:
// When a view-layout is added:
//   1. Author `<name>/renderer.tsx` exporting a default factory
//      function `(groupsDef) => UIViewRenderer`.
//   2. Add an import + an entry in the VIEW_LAYOUT_FACTORIES map below.
//   3. Re-bundle (server restart or registry hot-reload).

import CardLayout from './card/renderer.tsx'
import DetailPaneLayout from './detail-pane/renderer.tsx'
import InlineLayout from './inline/renderer.tsx'
import TabsLayout from './tabs/renderer.tsx'
import TwoColumnLayout from './two-column/renderer.tsx'
import SlotsLayout from './slots/renderer.tsx'

import type { Schema } from '../../../schema/index.ts'
import type { UITexts } from '../../../ui-core/i18n/index.ts'
import type { UIViewDef } from '../ui-view.schema.ts'
import type { UIViewGroups, UIViewRenderer } from '../ui-view.types.ts'

/** What a layout may read beyond its groups: the schema, the view's own
 *  definition (the `slots` layout reads its `slots`) and the texts. */
export type ViewLayoutContext = { schema: Schema; definition: UIViewDef; texts: UITexts }

/** Per-layout renderer factory shape: each view-layout's renderer.tsx
 *  default-exports a function taking the view's groups definition (and the
 *  layout context) and returning a UIViewRenderer. */
export type ViewLayoutFactory = (groupsDef: UIViewGroups, ctx: ViewLayoutContext) => UIViewRenderer

/** Default view layout name when the schema's ui-view extension does
 *  not specify one (and as the last-resort fallback in
 *  `resolveLayoutRenderer`). */
export const DEFAULT_VIEW_LAYOUT = 'detail-pane'

/** Map from view-layout name → its factory. */
const VIEW_LAYOUT_FACTORIES: Record<string, ViewLayoutFactory> = {
  card:          CardLayout,
  'detail-pane': DetailPaneLayout,
  inline:        InlineLayout,
  tabs:          TabsLayout,
  'two-column':  TwoColumnLayout,
  // A compact card or header: fields placed in named slots.
  slots:         SlotsLayout,
}

/** Resolve a view-layout name to its factory. */
export function getViewLayout(name: string): ViewLayoutFactory | undefined {
  return VIEW_LAYOUT_FACTORIES[name]
}

/** Names of all view-layouts that currently have a wired factory.
 *  Used by the harness for diagnostics. */
export function wiredViewLayoutNames(): string[] {
  return Object.keys(VIEW_LAYOUT_FACTORIES)
}

/** Wire the view-layouts. No-op stub for parallelism with
 *  `wireFormLayouts`. Called by `wireRegistries()` at runtime startup. */
export function wireViewLayouts(): void {
  // intentionally empty
}
