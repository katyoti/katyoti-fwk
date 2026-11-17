// SPDX-License-Identifier: Apache-2.0
// Editor components — pure presentational React widgets used inside
// content-type renderers (markdown editor, future code editor, etc.).
//
// Layering: src/ui-core/components/ is the LIBRARY of pure components;
// content-types/ is the CATALOG of consumers. content-types/<name>/
// renderer.tsx imports from here. The reverse direction is forbidden.

export { MarkdownEditor, type MarkdownEditorProps } from './markdown-editor.tsx'
