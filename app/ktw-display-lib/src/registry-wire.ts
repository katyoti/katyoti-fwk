// SPDX-License-Identifier: Apache-2.0
// Boot umbrella — wires every per-category factory map into its
// dispatcher. Co-located static maps live next to their content-type
// folders:
//
//   - `ui-builders/fields/factory.ts`        → FIELD_FACTORIES
//   - `ui-builders/form/layouts/factory.ts`  → FORM_LAYOUT_FACTORIES
//   - `ui-builders/view/layouts/factory.ts`  → VIEW_LAYOUT_FACTORIES
//
// This file kept its `registry-wire` name across the 2026-06-02
// folder reorg so existing consumers (`@ktw/display`'s
// runtime/index.tsx) keep working without an import rename. The
// previous body that held all three static maps + wire functions
// distributed into the three factory files; this is now a thin
// umbrella.

import { wireFieldRenderers, wiredRendererNames } from './ui-builders/fields/factory.ts'
import { wireFormLayouts, wiredFormLayoutNames } from './ui-builders/form/layouts/factory.ts'
import { wireViewLayouts, wiredViewLayoutNames } from './ui-builders/view/layouts/factory.ts'

/** Umbrella — call once at runtime bootstrap to wire ALL registries.
 *  Calls `wireFieldRenderers` + `wireFormLayouts` + `wireViewLayouts`. */
export function wireRegistries(): void {
  wireFieldRenderers()
  wireFormLayouts()
  wireViewLayouts()
}

// Re-export the individual wire functions + diagnostics for finer-
// grained consumption (e.g. test fixtures that need just one path).
export {
  wireFieldRenderers,
  wireFormLayouts,
  wireViewLayouts,
  wiredRendererNames,
  wiredFormLayoutNames,
  wiredViewLayoutNames,
}
