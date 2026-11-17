// SPDX-License-Identifier: Apache-2.0
// <UIPage> — typed host wrapper for the bun-side page widget.
//
// Specialization of <IframeWidget> that hardcodes `kind="page"` and
// reshapes the postMessage callback API for tree-shaped artifacts.
//
// Per the 2026-05-05 ADR `ui-micro-frontend-architecture`, this is the
// canonical mount point for any non-trivial UI artifact — a vbox of
// dashboards, a slide deck, a multi-section report, etc. The DSL tree
// is base64url-encoded into the `?page=<...>` URL param; cross-turn
// updates are pushed via the IframeWidget's `pushValue` mechanism (the
// host posts the WHOLE updated tree as a `value` postMessage, the
// page-iframe's harness re-renders).
//
// Sister to <UIForm>/<UIView>/<UITable>/<UIWizard>. Same layering
// principle: thin wiring on top of <IframeWidget> + the LifecycleCallbacks
// contract from the iframe-widget package — does NOT duplicate URL
// resolution, dev-fallback, origin checks, or sandboxing logic.
//
// Wired by display-dsl-framework S7 (ms06, 2026-05-05).

import { useCallback, useRef } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isDslTreeReady, isValueReady } from './anchor-readiness'

export interface UIPageProps {
  /** Initial DSL tree (a UIPageNode). Required — the page widget has
   *  nothing to render without it. */
  dslTree: unknown

  /** Optional outer data tree — fields referenced by `dataField: "$.path"`
   *  inside the dslTree resolve against this object via JSONPath. Set
   *  when the LLM emits one outer data record + multiple widgets binding
   *  to its fields (dashboard / customer-detail patterns). Forwarded to
   *  the iframe via `?data=<base64>` URL param at mount time. Wired by
   *  display-package-restructure Phase 3 (2026-05-06). */
  data?: unknown

  /** Optional cross-universe override. */
  universe?: string

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired when an inline schema-bound node submits (form save) or a
   *  wizard completes. The host typically forwards this back to the
   *  agent as a tool result. */
  onSubmit?: (value: Record<string, unknown>) => void

  /** Fired on item-scope action clicks from inline schema-bound nodes
   *  (view edit/delete, table row clicks). */
  onAction?: (action: string, args: Record<string, unknown>) => void

  /** Fired on iframe-reported runtime errors. */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback for parent debug panels. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px. Default 600. */
  initialHeight?: number

  /** Maximum container height in px. Default 2000. */
  maxHeight?: number

  /** Live-update push: when this changes, the host posts a `value`
   *  message to the iframe. Wire this to the artifact-store's tree
   *  state — every cross-turn `patch-ui-component` op updates the tree,
   *  and this prop forwards the new tree to the page-iframe. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent container's height
   *  instead of sizing to content. Use when mounted inside a sized tab
   *  (artifact tabs in ChatPanel). Default: false. */
  fillHeight?: boolean

  /** Display mode forwarded via `?display=`. `'ui'` strips harness chrome;
   *  `'schema'` shows only the source schema; `'full'` keeps legacy debug. */
  display?: 'ui' | 'schema' | 'full'

  /** Defer URL anchoring until streaming completes — see UIForm for the
   *  full rationale. Default undefined (treated as true). */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served page widget at `/widget/page` for a given
 * DSL tree. The iframe renders through `<UIPageView>` — recursive
 * dispatch over the 21 DSL node kinds, with schema-bound nodes
 * (form/view/table/wizard) inlined inside the same iframe (no nested
 * iframes per the 2026-05-05 ADR).
 *
 * @example
 * ```tsx
 * <UIPage
 *   dslTree={{
 *     kind: 'vbox',
 *     children: [
 *       { kind: 'heading', text: 'Project X' },
 *       { kind: 'grid', cols: 3, children: [
 *         { kind: 'kpi', label: 'Open', value: 42 },
 *         { kind: 'kpi', label: 'Closed', value: 12 },
 *         { kind: 'kpi', label: 'MTTR', value: '4.2h' },
 *       ]},
 *     ],
 *   }}
 *   pushValue={artifact.dslTree}
 * />
 * ```
 */
export function UIPage({
  dslTree,
  data,
  universe,
  onReady,
  onSubmit,
  onAction,
  onError,
  onStatusChange,
  initialHeight = 600,
  maxHeight = 2000,
  pushValue,
  fillHeight,
  display,
  streamingComplete,
}: UIPageProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'submit':
          onSubmit?.(msg.value)
          return
        case 'action':
          onAction?.(msg.action, msg.args)
          return
        case 'error':
          onError?.(msg.error)
          return
        // 'change' / 'request-value' aren't part of the page surface
        // contract today — silently ignored. 'resize' is handled inside
        // <IframeWidget>.
      }
    },
    [onReady, onSubmit, onAction, onError],
  )

  // Capture initial dslTree + data once, on first non-empty arrival — same
  // pattern as UIForm/View/Table/Wizard. Per-render re-encoding (the prior
  // approach) thrashed the iframe `src` on every streamed patch
  // during streaming: every patch generated a new URL, the browser
  // cancelled the in-flight 6.5MB runtime.js load, and the runtime never
  // finished bootstrapping → blank iframe forever (verified 2026-05-17
  // via [frontend][iframe-widget] log: 289 resolve OK + 0 ready events).
  //
  // Subsequent dslTree updates flow via `pushValue` postMessage (the bun
  // harness's universal-harness.tsx `setTree(msg.value)` handler). The
  // iframe-stable-extra-params pattern's capture gate — wait for non-
  // undefined before locking — gives the streaming path one clean window
  // to capture a fully-parsed tree before the URL freezes.
  const initialTreeEnc = useRef<string | null>(null)
  const initialDataEnc = useRef<string | null>(null)
  // Anchor only when the dslTree is "ready" (non-empty kind + non-empty
  // children for containers). Partial-JSON parsing during streaming yields
  // intermediate states like `{kind: "vbox"}` or `{kind: "vbox", children: []}`
  // that would render as an empty page; anchoring on them freezes the
  // iframe URL on a useless skeleton. See ./anchor-readiness.ts.
  if (
    initialTreeEnc.current === null &&
    (streamingComplete ?? true) &&
    isDslTreeReady(dslTree)
  ) {
    const enc = encodeSchemaParam(dslTree)
    if (enc) initialTreeEnc.current = enc
  }
  if (
    initialDataEnc.current === null &&
    (streamingComplete ?? true) &&
    isValueReady(data)
  ) {
    const enc = encodeSchemaParam(data)
    if (enc) initialDataEnc.current = enc
  }

  const extraParams: Record<string, string> = {}
  extraParams['display'] = display ?? 'ui'
  if (initialTreeEnc.current) extraParams['page'] = initialTreeEnc.current
  if (initialDataEnc.current) extraParams['data'] = initialDataEnc.current
  // While no tree has anchored, the iframe mounts with no `?page=` param;
  // the bun route renders the shared loading screen until the host swaps
  // in the real tree (URL change → reload).

  return (
    <IframeWidget
      kind="page"
      extraParams={extraParams}
      onIframeMessage={onIframeMessage}
      onStatusChange={onStatusChange}
      initialHeight={initialHeight}
      maxHeight={maxHeight}
      pushValue={pushValue}
      fillHeight={fillHeight}
    />
  )
}
