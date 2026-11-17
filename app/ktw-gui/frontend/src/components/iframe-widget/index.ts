// SPDX-License-Identifier: Apache-2.0
// Public surface of the iframe-widget package.
//
// Consumers (per-composition wrappers like <UIForm>, <UIView>, the dev
// demo page, the future Claude-style artifact renderer) import from here.

export { IframeWidget } from './IframeWidget'
export type {
  IframeWidgetProps,
  IframeWidgetStatus,
} from './IframeWidget'

export { useDisplayStatus } from './use-display-status'
export type { DisplayStatus, UseDisplayStatusResult } from './use-display-status'

// Lifecycle helpers — useful for parent components that need to push
// host → iframe messages outside of <IframeWidget>'s built-in effects
// (e.g. injecting a value in response to request-value, or forcing reload).
export {
  postTheme,
  postValue,
  postSchemaChanged,
  postReload,
  resolveTheme,
  clampHeight,
  dispatchIframeMessage,
} from './lifecycle'
export type { LifecycleCallbacks } from './lifecycle'
