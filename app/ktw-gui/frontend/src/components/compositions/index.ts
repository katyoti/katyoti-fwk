// SPDX-License-Identifier: Apache-2.0
// Public surface of the compositions package — typed host wrappers
// over <IframeWidget> for each ktw-display top-level composition.
//
// One wrapper per kind: form / view / table / wizard / deck / dashboard
// / artifact (per modules/display/host-iframe-wire.md:
// one iframe per top-level composition). Each wrapper hardcodes its `kind` and reshapes the postMessage
// callback API to be value-centric — hosts get typed `onSubmit(value)` /
// `onChange(value, meta)` / etc. instead of fishing through the raw
// IframeMessage union.
//
// Layering:
//   <IframeWidget>     ← primitive (URL resolution, sandbox, postMessage plumbing)
//   <UIForm> / etc.    ← typed surface (hardcoded kind, value-centric callbacks)
//   sections/dev/*     ← demo pages exercising both layers
//   sections/<feature>/← real product surfaces consuming the typed wrappers
//
// Subtask 6 of gui-form (2026-05-03). Only `UIForm` ships in v1; future
// gui-view / gui-table / gui-wizard todos add the other wrappers.

export { UIForm, type UIFormProps, type UIFormMode } from './UIForm'
export { UIView, type UIViewProps, type UIViewMode } from './UIView'
export { UITable, type UITableProps, type UITableMode } from './UITable'
export { UITreeTable, type UITreeTableProps, type UITreeTableMode } from './UITreeTable'
export { NativeObjectView, type NativeObjectViewProps } from './NativeObjectView'
export { UIWizard, type UIWizardProps, type UIWizardMode } from './UIWizard'
export { UIPage, type UIPageProps } from './UIPage'
export { UIDrawio, type UIDrawioProps, type UIDrawioValue } from './UIDrawio'
export { UIDeck, type UIDeckProps } from './UIDeck'
export {
  UIComponent,
  type UIComponentProps,
  type UIComponentDef,
  type UIFormDef,
  type UIViewDef,
  type UITableDef,
  type UIWizardDef,
  type UIPageDef,
} from './UIComponent'
