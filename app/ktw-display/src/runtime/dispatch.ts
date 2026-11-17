// SPDX-License-Identifier: Apache-2.0
// Dispatch — pick the typed view kind for a given schema + optional URL
// widget kind hint.
//
// Refactored 2026-05-06 (display-package-restructure Phase 1.5): replaced
// per-kind harness component dispatch with a thin `pickViewKind` that
// returns just the kind string. The runtime mounts <UniversalHarness>
// with the appropriate `renderInner` from `render-inner.tsx`. Falls back
// to `'proto'` when no extension matches; the entry-point handles the
// proto-walker fallback by mounting the legacy <Harness> component
// directly (it has its own internal chrome).
//
// Resolution precedence:
//   1. Explicit URL `?widget=<kind>` query param OR `/widget/<kind>` path
//   2. Schema-extension lookup (auto-detect)
//   3. Fallback: 'proto' (proto-walker)

import type { Schema } from '@ktw/display-lib/schema'
import type { TypedKind } from './render-inner.tsx'

export type ViewKind = TypedKind | 'proto'

/** True iff the schema's root type carries an extension of the given type. */
function hasExtension(schema: unknown, extensionType: string): boolean {
  if (!schema || typeof schema !== 'object') return false
  const s = schema as Schema
  if (!s.root || !s.types) return false
  const rootType = s.types[s.root]
  if (!rootType || typeof rootType !== 'object') return false
  const exts = (rootType as { extensions?: Array<{ type?: string }> }).extensions
  if (!Array.isArray(exts)) return false
  return exts.some((e) => e?.type === extensionType)
}

/** Resolve the URL widget kind from `window.location.pathname` — e.g.,
 *  `/widget/form` → `'form'`. Returns undefined when not in a widget URL. */
function resolveUrlWidgetKind(): string | undefined {
  try {
    const m = window.location.pathname.match(/\/widget\/([^/?#]+)/)
    return m?.[1]
  } catch {
    return undefined
  }
}

/** Pick the view kind for the runtime's current schema.
 *
 * @param schema       The schema from `__KTW_RUNTIME_CONFIG__.schema`
 * @param urlKindHint  Optional override (test harness sets this when
 *   mounting a case via `/test/<id>`); production reads from window.location.
 */
export function pickViewKind(schema: unknown, urlKindHint?: string): ViewKind {
  const urlKind = urlKindHint ?? resolveUrlWidgetKind()

  // 1. URL hint pinned to a specific kind
  if (urlKind === 'page') return 'page'
  if (urlKind === 'wizard') return 'wizard'
  if (urlKind === 'table') return 'table'
  if (urlKind === 'tree-table') return 'tree-table'
  if (urlKind === 'view') return 'view'
  if (urlKind === 'form') return 'form'
  if (urlKind === 'drawio') return 'drawio'
  if (urlKind === 'deck') return 'deck'

  // 2. No URL hint: schema-extension auto-detect
  if (hasExtension(schema, 'ui-wizard')) return 'wizard'
  if (hasExtension(schema, 'ui-tree-table')) return 'tree-table'
  if (hasExtension(schema, 'ui-table')) return 'table'
  if (hasExtension(schema, 'ui-view')) return 'view'
  if (hasExtension(schema, 'ui-form')) return 'form'
  if (hasExtension(schema, 'ui-drawio')) return 'drawio'

  // 3. Fallback: proto walker
  return 'proto'
}
