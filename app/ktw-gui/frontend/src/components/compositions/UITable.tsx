// SPDX-License-Identifier: Apache-2.0
// <UITable> — typed host wrapper for the bun-side table widget.
//
// Specialization of <IframeWidget> that hardcodes `kind="table"` and
// reshapes the postMessage callback API to be value-centric: hosts pass
// `onRowClick={(item) => ...}` instead of fishing through the raw
// `IframeMessage` union for `msg.type === 'action'`.
//
// Sister to <UIForm> + <UIView> (gui-form / gui-view Subtask 6 each). Same
// layering principle: thin wiring on top of <IframeWidget> + the
// LifecycleCallbacks contract — does NOT duplicate URL resolution,
// dev-fallback, origin checks, sandboxing, or sentinel UX.

import { useCallback, useRef } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isSchemaReady, isValueReady } from './anchor-readiness'

/** Table CRUD modes. The bun-side runtime treats `'view'` as the only
 *  legitimate token here; tables are read-only at v1 (writes go through
 *  row-click → host-mounted UIForm). */
export type UITableMode = 'view'

export interface UITableProps {
  /** Optional schema-name lookup (server-side registry). Without a
   *  schemaName or schemaInline, the bun route renders a skeleton
   *  placeholder (it never fabricates demo data). */
  schemaName?: string

  /** Optional value reference (e.g. a list-id the bun side resolves). */
  value?: string

  /** CRUD mode. Default + only valid value: `'view'`. */
  mode?: UITableMode

  /** Optional cross-universe override. Defaults to the GUI's displayed
   *  universe. Forwarded to <IframeWidget>. */
  universe?: string

  /** Pick a named table variant (when the schema declares multiple
   *  `ui-table` extensions). Forwarded as `?table=<name>`. */
  tableName?: string

  /** Pick a named filter set (when the schema declares multiple
   *  `ui-filter` extensions). Forwarded as `?filter=<name>`. */
  filterName?: string

  /** Inline schema injection (LLM-streaming-protocol artifact mount). */
  schemaInline?: unknown
  /** Inline value (companion to schemaInline). */
  valueInline?: unknown

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired when the user clicks a row. The `item` payload is the full
   *  row object (typed loosely as Record<string, unknown> until the
   *  schema-typed-row API lands). */
  onRowClick?: (item: Record<string, unknown>) => void

  /** Fired on iframe-reported runtime errors. */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback for parent debug panels. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px. Default 600 (taller than form/view —
   *  tables typically need more vertical room for pagination + chrome). */
  initialHeight?: number

  /** Maximum container height in px. Default 2000. */
  maxHeight?: number

  /** Live-update push (LLM-streaming-protocol). See UIForm for details. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent's height. */
  fillHeight?: boolean

  /** Display mode forwarded via `?display=`. */
  display?: 'ui' | 'schema' | 'full'

  /** Defer URL anchoring until streaming completes — see UIForm. */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served table widget at `/widget/table`. Renders an
 * Array-of-Object schema as a sortable, filterable, paginated table.
 *
 * @example
 * ```tsx
 * <UITable
 *   schemaName="Library"
 *   onRowClick={(book) => openDetailPane(book)}
 * />
 * ```
 */
export function UITable({
  schemaName,
  value,
  mode = 'view',
  universe,
  tableName,
  filterName,
  schemaInline,
  valueInline,
  onReady,
  onRowClick,
  onError,
  onStatusChange,
  initialHeight = 600,
  maxHeight = 2000,
  pushValue,
  fillHeight,
  display,
  streamingComplete,
}: UITableProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'action':
          // The table emits `{action: 'click', args: {item}}` on row clicks.
          if (msg.action === 'click' && onRowClick && msg.args && 'item' in msg.args) {
            onRowClick(msg.args.item as Record<string, unknown>)
          }
          return
        case 'error':
          onError?.(msg.error)
          return
        // 'change' / 'submit' don't fire in table (read-only); 'resize'
        // is handled inside <IframeWidget>; 'request-value' isn't part
        // of the table surface contract. Silently ignored.
      }
    },
    [onReady, onRowClick, onError],
  )

  // Capture initial schemaInline + valueInline once — see iframe-stable-
  // extra-params pattern (`modules/display/host-iframe-wire.md#rules`). Live updates flow
  // via `pushValue` postMessage; URL stays stable so the iframe doesn't
  // remount on every render.
  //
  // For streaming-mounted artifacts (display-ui-component), schemaInline
  // arrives `undefined` on the first mount (partial-JSON parse hasn't
  // seen it yet) then fills in via a later patch. We deliberately DO NOT
  // lock an empty capture — wait for a meaningful value to arrive, then
  // freeze. The iframe remounts once when schema/value materialize, then
  // stays stable for the rest of the artifact's lifetime.
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
  if (tableName) extraParams['table'] = tableName
  if (filterName) extraParams['filter'] = filterName
  if (initialSchemaEnc.current) extraParams['schema'] = initialSchemaEnc.current
  if (initialValueEnc.current) extraParams['value'] = initialValueEnc.current

  return (
    <IframeWidget
      kind="table"
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
