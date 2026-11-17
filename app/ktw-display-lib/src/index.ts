// SPDX-License-Identifier: Apache-2.0
// @ktw/display-lib — public surface.
//
// Re-exports the entire schema-driven UI rendering library so consumers
// can `import { ... } from '@ktw/display-lib'` for everything, or
// import from specific sub-paths (e.g. '@ktw/display-lib/ui-builders/form')
// for tree-shakable granularity. Both shapes resolve through the
// `exports` map in package.json.
//
// Consumed by:
//   - @ktw/display (the bun iframe runtime in app/ktw-display/)
//   - ktw-gui's frontend (direct React embed via the Wails webview)
//
// What's NOT here: HTTP server, iframe wrapper, widget URL composition,
// KTSP membership. Those live in @ktw/display and @ktw/ktsp-sdk.

// --- Schema AST (the canonical shape every consumer reads) ---
export * from './schema/index.ts'

// --- Cross-cutting infrastructure ---
export * from './types/index.ts'

// --- Boot umbrella: wires all the per-category static factory maps
// (form-layouts, view-layouts, field-renderers) into their respective
// dispatchers. One call at runtime startup. ---
export * from './registry-wire.ts'

// --- ui-core: action surface, filter, i18n, presentational components ---
// (The legacy `ui-core/fields/` was merged into `ui-builders/fields/`
// 2026-06-02 — see that folder's index.ts for the field renderer
// surface.)
export * from './ui-core/action/index.ts'
export * from './ui-core/components/index.ts'
export * from './ui-core/i18n/index.ts'

// --- ui-builders: schema → React component pipelines ---
export * from './ui-builders/fields/index.ts'
export * from './ui-builders/filter/index.ts'
export * from './ui-builders/form/index.ts'
export * from './ui-builders/view/index.ts'
export * from './ui-builders/table/index.ts'
export * from './ui-builders/wizard/index.ts'
export * from './ui-builders/timeline-lanes/index.ts'
export * from './ui-builders/page/index.ts'
export * from './ui-builders/drawio/index.ts'
