// SPDX-License-Identifier: Apache-2.0
// Public surface of the drawio ui-builder. The view wraps a vendored
// drawio viewer (Apache 2.0) loaded on demand from
// /widget/vendor/drawio/viewer-static.min.js. See
// modules/drawio/docs/viewer-and-streaming.md.

export { DrawioView } from './DrawioView.tsx'
export type { DrawioValue, DrawioViewProps } from './DrawioView.tsx'
export { loadDrawioViewer, createDrawioViewer } from './viewer-shim.ts'
export type { DrawioViewerHandle } from './viewer-shim.ts'
export { autoCloseXml } from './auto-close.ts'
