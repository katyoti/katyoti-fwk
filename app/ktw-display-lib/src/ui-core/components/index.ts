// SPDX-License-Identifier: Apache-2.0
// Pure presentational components — no business logic, no schema-aware
// rendering. Used by content-type renderers (e.g., markdown/renderer.tsx
// imports MarkdownEditor) and by composition-level UI code.
//
// Layering: src/ui-core/components/ is the LIBRARY. content-types/<name>/ and
// upstream UI (forms, layouts, etc.) IMPORT from here. The reverse is
// forbidden.

export * from './icons/index.ts'
export * from './editors/index.ts'
export * from './other/index.ts'
