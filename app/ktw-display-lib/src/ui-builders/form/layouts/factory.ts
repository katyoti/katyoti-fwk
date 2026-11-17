// SPDX-License-Identifier: Apache-2.0
// Form-layout factory — co-located static map of all available form
// layouts in this folder. Reorganized 2026-06-02: this file replaces
// the layouts portion of the (now-removed) src/registry-wire.ts.
//
// AUTHORING CONTRACT:
// When a form-layout is added:
//   1. Author `<name>/renderer.tsx` exporting a default factory
//      function `(groupsDef) => UIFormRenderer`.
//   2. Add an import + an entry in the FORM_LAYOUT_FACTORIES map below.
//   3. Re-bundle (server restart or registry hot-reload).
//
// At v1 the catalog is short (4 default layouts: single, groups, tabs,
// wizard). When it grows past ~40 entries, switch to a build-time
// codegen step that generates this file from a folder scan.

import GroupsLayout from './groups/renderer.tsx'
import SingleLayout from './single/renderer.tsx'
import TabsLayout from './tabs/renderer.tsx'
import WizardLayout from './wizard/renderer.tsx'

import type { UIFormGroups, UIFormRenderer } from '../ui-form.types.ts'

/** Per-layout renderer factory shape: each form-layout's renderer.tsx
 *  default-exports a function taking the form's groups definition and
 *  returning a UIFormRenderer. */
export type FormLayoutFactory = (groupsDef: UIFormGroups) => UIFormRenderer

/** Default form layout name when the schema's ui-form extension does
 *  not specify one (and as the last-resort fallback in
 *  `resolveLayoutRenderer`). */
export const DEFAULT_FORM_LAYOUT = 'groups'

/** Map from form-layout name → its factory. */
const FORM_LAYOUT_FACTORIES: Record<string, FormLayoutFactory> = {
  single: SingleLayout,
  groups: GroupsLayout,
  tabs:   TabsLayout,
  wizard: WizardLayout,
}

/** Resolve a form-layout name to its factory. */
export function getFormLayout(name: string): FormLayoutFactory | undefined {
  return FORM_LAYOUT_FACTORIES[name]
}

/** Names of all form-layouts that currently have a wired factory.
 *  Used by the harness for diagnostics. */
export function wiredFormLayoutNames(): string[] {
  return Object.keys(FORM_LAYOUT_FACTORIES)
}

/** Wire the form-layouts. No-op stub — getFormLayout() reads the map
 *  directly. Reserved for future side-effect registrations (theme
 *  hooks, i18n namespace bindings, etc.) per layout. Called by
 *  `wireRegistries()` at runtime startup. */
export function wireFormLayouts(): void {
  // intentionally empty
}
