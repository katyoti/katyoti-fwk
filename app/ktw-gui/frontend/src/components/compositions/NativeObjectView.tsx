// SPDX-License-Identifier: Apache-2.0
// <NativeObjectView> — render a schema-driven `ui-view` of one object NATIVELY
// (no iframe) inside ktw-gui, via @ktw/display-lib.
//
// This is the reusable host for (a) long-running-method live-state and (b)
// custom node/item pages. Payload = `{ schema, value }` (schema may declare
// ui-view / ui-table / ui-tree-table extensions; array-of-object fields render
// as grid/table/tree per their `content`). The same display-lib components stay
// iframe-usable via the UIView/UITable wrappers — this is an ADDITIVE native path.
//
// Mirrors the iframe runtime bootstrap (app/ktw-display/src/runtime/index.tsx):
// wire the field/layout registries + i18n once, build the view, mount UIViewView.

import '@mantine/charts/styles.css'

import { useMemo } from 'react'

import { wireRegistries } from '@ktw/display-lib/registry-wire'
import { initI18n } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { buildUIViews, UIViewView } from '@ktw/display-lib/ui-builders/view/index.ts'
import type { Schema } from '@ktw/display-lib/schema'

// One-time registry + i18n wiring for the native render path (idempotent).
let wired = false
function ensureWired(): void {
  if (wired) return
  void initI18n()
  wireRegistries()
  wired = true
}

export interface NativeObjectViewProps {
  /** Canonical schema AST (with ui-view / ui-table / ui-tree-table extensions). */
  schema: unknown
  /** The object value to display. */
  value?: unknown
  /** Named ui-view variant (defaults to 'default'). */
  viewName?: string
  /** Initial detail level (1/2/3). */
  detailLevel?: 1 | 2 | 3
  /** Suppress the default object action bar (Close/Edit/Delete). Use for
   *  read-only displays like live-state, where those CRUD actions make no
   *  sense. Built via `actionMode: 'replace'` + empty refs so no defaults are
   *  injected. Default: false (pages that want CRUD keep the defaults). */
  hideActions?: boolean
  /** Suppress the "View : <row>" title + detail-level toggle (host provides its
   *  own chrome). Default: false. */
  hideTitle?: boolean
}

export function NativeObjectView({ schema, value, viewName = 'default', detailLevel, hideActions, hideTitle }: NativeObjectViewProps) {
  ensureWired()
  const view = useMemo(() => {
    // hideActions → replace the ui-view's default actions with none (the
    // defaults are baked into the renderer at build time, so this must happen
    // at build, not after).
    const views = hideActions
      ? buildUIViews({ schema: schema as Schema, actionMode: 'replace', actionRefs: [] })
      : buildUIViews({ schema: schema as Schema })
    return views[viewName] ?? views['default']
  }, [schema, viewName, hideActions])
  if (!view) return null
  return (
    <UIViewView
      view={view}
      item={(value ?? {}) as Record<string, unknown>}
      detailLevel={detailLevel}
      hideTitle={hideTitle}
    />
  )
}
