// SPDX-License-Identifier: Apache-2.0
// <UITreeTable> — typed host wrapper for the bun-side tree-table widget.
//
// Sister to <UITable> (same iframe layering); mounts `/widget/tree-table`.
// Renders an array-of-nodes schema (linked by a parent-id field, declaring a
// `ui-tree-table` extension) as nested, indented rows with expand/collapse and
// status badges. Read-only; row clicks emit an `action` the host can wire.

import { useCallback, useRef } from 'react'

import { IframeWidget, type IframeWidgetStatus } from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isSchemaReady, isValueReady } from './anchor-readiness'

export type UITreeTableMode = 'view'

export interface UITreeTableProps {
  schemaName?: string
  value?: string
  mode?: UITreeTableMode
  universe?: string
  /** Pick a named tree-table variant. Forwarded as `?tree-table=<name>`. */
  treeName?: string
  schemaInline?: unknown
  valueInline?: unknown
  onReady?: () => void
  onRowClick?: (item: Record<string, unknown>) => void
  onError?: (error: { name: string; message: string; stack?: string }) => void
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void
  initialHeight?: number
  maxHeight?: number
  pushValue?: unknown
  fillHeight?: boolean
  display?: 'ui' | 'schema' | 'full'
  streamingComplete?: boolean
}

export function UITreeTable({
  schemaName,
  value,
  mode = 'view',
  universe,
  treeName,
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
}: UITreeTableProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'action':
          if (msg.action === 'click' && onRowClick && msg.args && 'item' in msg.args) {
            onRowClick(msg.args.item as Record<string, unknown>)
          }
          return
        case 'error':
          onError?.(msg.error)
          return
      }
    },
    [onReady, onRowClick, onError],
  )

  // Freeze the first meaningful schema/value into the URL; live updates ride
  // pushValue postMessage (iframe-stable-extra-params pattern) — same as UITable.
  const initialSchemaEnc = useRef<string | null>(null)
  const initialValueEnc = useRef<string | null>(null)
  if (initialSchemaEnc.current === null && (streamingComplete ?? true) && isSchemaReady(schemaInline)) {
    const enc = encodeSchemaParam(schemaInline)
    if (enc) initialSchemaEnc.current = enc
  }
  if (initialValueEnc.current === null && (streamingComplete ?? true) && isValueReady(valueInline)) {
    const enc = encodeSchemaParam(valueInline)
    if (enc) initialValueEnc.current = enc
  }

  const extraParams: Record<string, string> = {}
  extraParams['display'] = display ?? 'ui'
  if (treeName) extraParams['tree-table'] = treeName
  if (initialSchemaEnc.current) extraParams['schema'] = initialSchemaEnc.current
  if (initialValueEnc.current) extraParams['value'] = initialValueEnc.current

  return (
    <IframeWidget
      kind="tree-table"
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
