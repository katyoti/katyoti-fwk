// SPDX-License-Identifier: Apache-2.0
// <UIView> — typed host wrapper for the bun-side view widget.
//
// Specialization of <IframeWidget> that hardcodes `kind="view"` and
// reshapes the postMessage callback API to be value-centric: hosts pass
// `onAction={(action, args) => ...}` instead of fishing through the raw
// `IframeMessage` union for `msg.type === 'action'`.
//
// Sister to <UIForm> (gui-form Subtask 6). Same layering principle: thin
// (~50 LOC of wiring) on top of <IframeWidget> + the LifecycleCallbacks
// contract from the iframe-widget package — does NOT duplicate URL
// resolution, dev-fallback, origin checks, or sandboxing logic.

import { useCallback, useRef } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isSchemaReady, isValueReady } from './anchor-readiness'

/** View CRUD modes. The bun-side runtime treats `'view'` as the only
 *  legitimate token here; `'edit'`/`'create'`/`'update'` are silently
 *  rejected (UIView is read-only). Pass `'view'` (the default) unless
 *  you have a specific reason. */
export type UIViewMode = 'view'

export interface UIViewProps {
  /** Optional schema-name lookup (server-side registry). Without a
   *  schemaName or schemaInline, the bun route renders a skeleton
   *  placeholder (it never fabricates demo data). */
  schemaName?: string

  /** Optional value reference — typically a record id the bun side
   *  resolves to a full value object before mounting. */
  value?: string

  /** CRUD mode. Default + only valid value: `'view'`. */
  mode?: UIViewMode

  /** Optional cross-universe override. Defaults to the GUI's displayed
   *  universe. Forwarded to <IframeWidget>. */
  universe?: string

  /** Pick a named view variant (when the schema declares multiple
   *  `ui-view` extensions). Forwarded to the iframe URL as `?view=<name>`.
   *  The runtime defaults to `'default'` when omitted. */
  viewName?: string

  /** Empty-value rendering strategy: `omit` skips empty fields, `em-dash`
   *  renders a dimmed `—`, `explicit-empty` defers to per-renderer empty
   *  placeholders. Forwarded to the iframe URL as `?empty=<strategy>`. */
  emptyStrategy?: 'omit' | 'em-dash' | 'explicit-empty'

  /** Inline schema injection — used by the LLM-streaming-protocol
   *  artifact mount path. base64url-encoded into `?schema=<...>`. */
  schemaInline?: unknown

  /** Inline value (companion to schemaInline). */
  valueInline?: unknown

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired when the user clicks an item-scope action (edit / delete /
   *  duplicate / export / etc.). The host is responsible for
   *  navigating to the edit form, opening a confirmation dialog, etc. */
  onAction?: (action: string, args: Record<string, unknown>) => void

  /** Fired on iframe-reported runtime errors. */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback for parents that want to surface a loading/ok/
   *  fallback/down badge. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px (until the iframe sends a resize).
   *  Default 480 — taller than the iframe-widget default (320) because
   *  views typically have multiple sections. */
  initialHeight?: number

  /** Maximum container height in px — clamps runaway resize messages.
   *  Default 1600. */
  maxHeight?: number

  /** Live-update push (LLM-streaming-protocol). See UIForm for details. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent's height. */
  fillHeight?: boolean

  /** Display mode forwarded via `?display=`. `'ui'` strips chrome; `'schema'`
   *  shows only the source schema; `'full'` keeps legacy debug surface. */
  display?: 'ui' | 'schema' | 'full'

  /** Defer URL anchoring until streaming completes — see UIForm for the
   *  full rationale. Default undefined (treated as true). */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served view widget at `/widget/view` for a given
 * (schemaName, value) tuple. The iframe renders through the new
 * `buildUIView()` + `<UIViewView>` pipeline (auto-detected runtime-side
 * by the presence of a `ui-view` extension on the schema's root type).
 *
 * @example
 * ```tsx
 * <UIView
 *   schemaName="Book"
 *   value="book-id-abc123"
 *   onAction={(action, args) => {
 *     if (action === 'edit') openEditForm(args.item)
 *     else if (action === 'delete') confirmDelete(args.item)
 *   }}
 * />
 * ```
 */
export function UIView({
  schemaName,
  value,
  mode = 'view',
  universe,
  viewName,
  emptyStrategy,
  schemaInline,
  valueInline,
  onReady,
  onAction,
  onError,
  onStatusChange,
  initialHeight = 480,
  maxHeight = 1600,
  pushValue,
  fillHeight,
  display,
  streamingComplete,
}: UIViewProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'action':
          onAction?.(msg.action, msg.args)
          return
        case 'error':
          onError?.(msg.error)
          return
        // 'change' / 'submit' don't fire in view (read-only); 'resize'
        // is handled inside <IframeWidget>; 'request-value' isn't part
        // of the view surface contract. Silently ignored.
      }
    },
    [onReady, onAction, onError],
  )

  // Capture initial schemaInline + valueInline once — see iframe-stable-
  // extra-params pattern (`modules/display/host-iframe-wire.md#rules`). Live updates flow
  // via `pushValue` postMessage; URL stays stable so the iframe doesn't
  // remount on every render.
  //
  // For streaming-mounted artifacts (display-ui-component), schemaInline
  // arrives `undefined` on the first mount (partial-JSON parse hasn't seen
  // it yet) then fills in via a later patch. We deliberately DO NOT lock
  // an empty/undefined capture — wait for a meaningful value to arrive,
  // then freeze. The iframe remounts once when schema/value materialize,
  // then stays stable for the rest of the artifact's lifetime.
  const initialSchemaEnc = useRef<string | null>(null)
  const initialValueEnc = useRef<string | null>(null)
  if (
    initialSchemaEnc.current === null &&
    (streamingComplete ?? true) &&
    isSchemaReady(schemaInline)
  ) {
    const enc = encodeSchemaParam(schemaInline)
    if (enc) initialSchemaEnc.current = enc
  }
  if (
    initialValueEnc.current === null &&
    (streamingComplete ?? true) &&
    isValueReady(valueInline)
  ) {
    const enc = encodeSchemaParam(valueInline)
    if (enc) initialValueEnc.current = enc
  }

  const extraParams: Record<string, string> = {}
  extraParams['display'] = display ?? 'ui'
  if (viewName) extraParams['view'] = viewName
  if (emptyStrategy) extraParams['empty'] = emptyStrategy
  if (initialSchemaEnc.current) extraParams['schema'] = initialSchemaEnc.current
  if (initialValueEnc.current) extraParams['value'] = initialValueEnc.current

  return (
    <IframeWidget
      kind="view"
      schemaName={schemaName}
      value={value}
      mode={mode}
      universe={universe}
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
