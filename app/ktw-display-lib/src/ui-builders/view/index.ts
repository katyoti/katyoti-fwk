// SPDX-License-Identifier: Apache-2.0
// Public surface of the view package — DSL extension types, runtime
// types, the `buildUIView` / `buildUIViews` builders, the `<UIViewView>`
// React component, and the shared layout primitives that custom layouts
// compose.

// DSL extension types (authored on schema definitions)
export type {
  UIViewDef,
  UIViewGroupDef,
  UIViewRef,
} from './ui-view.schema.ts'

// Runtime types (consumed by layouts and the view component)
export type {
  UIView,
  UIViewButtonsRenderer,
  UIViewEmptyStrategy,
  UIViewFields,
  UIViewGroup,
  UIViewGroups,
  UIViewGroupRenderer,
  UIViewRenderer,
  UIViewRendererParams,
} from './ui-view.types.ts'

// Builders + overrides
export {
  buildUIView,
  buildUIViews,
  buildViewGroups, // @internal — exposed for visibility/detail-level filter unit tests
  toUIView,
  toUIViewDef,
  type BuildUIViewParams,
  type BuildUIViewsParams,
  type UIViewOverrides,
} from './ui-view.builder.ts'

// React component
export { UIViewView, type UIViewViewProps } from './ui-view.view.tsx'

// Shared layout primitives — exposed so authors writing a 6th custom
// layout under `content-types/view-layouts/<name>/` can compose them
// without re-implementing.
export { vol_buttons_layout, vol_group_layout } from './layouts/_shared/index.ts'
