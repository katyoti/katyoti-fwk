// SPDX-License-Identifier: Apache-2.0
// <UIDrawio> — typed host wrapper for the bun-side drawio viewer.
//
// View-only (gui-drawio M1, 2026-05-07). The iframe lazy-loads the
// vendored viewer-static.min.js (Apache 2.0) on first mount and renders
// the XML; the user clicks "Save .drawio" to download the file locally
// for editing in their drawio desktop / external viewer.
//
// Streaming: when the host's `pushValue` prop changes, the iframe's
// harness routes a new `value` postMessage and DrawioView re-renders.
// Token-level streaming + auto-close shim land in M2.

import { useCallback } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { DrawioEditor } from './DrawioEditor'

export interface UIDrawioValue {
  xml: string
  filename?: string
  format?: 'mxgraph' | 'svg+xml'
  page?: number
}

export interface UIDrawioProps {
  /** Initial drawio value. The iframe receives this as `valueInline`. */
  value: UIDrawioValue

  /** Optional cross-universe override. */
  universe?: string

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired on iframe-reported runtime errors (e.g. viewer-static.min.js
   *  failed to load). */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback for parent debug panels. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px. Default 600. */
  initialHeight?: number

  /** Maximum container height in px. Default 2000. */
  maxHeight?: number

  /** Live-update push: new XML pushed via postMessage; the iframe
   *  re-renders. Wire this to streamed `value_patch` events from the
   *  agent (M2). */
  pushValue?: UIDrawioValue

  /** When true, fill the parent container. */
  fillHeight?: boolean

  /** Display mode forwarded via `?display=`. */
  display?: 'ui' | 'schema' | 'full'

  /** When true, mount the full editable drawio editor instead of the
   *  read-only viewer. Requires the editor bundle to be vendored at
   *  app/ktw-display/vendor/drawio/editor/ (see vendor/drawio/NOTICE).
   *  See modules/drawio/docs/viewer-and-streaming.md. */
  editable?: boolean

  /** Fired when the user clicks Save in the drawio editor. Caller
   *  persists the new XML (e.g. updates the artifact in the store).
   *  Only meaningful when `editable` is true. */
  onChange?: (xml: string) => void

  /** Fired when the user closes/exits the editor. Caller typically
   *  flips back to view mode. Only meaningful when `editable` is true. */
  onExit?: () => void

  /** Defer anchoring the diagram into the URL until streaming settles —
   *  see UIPage for the full rationale. While unset/false AND no xml has
   *  arrived, the iframe mounts WITHOUT a `value` param (and WITH
   *  `?loading=1`) so the bun route shows the shared loading screen
   *  instead of the viewer's blank "No diagram" state. Default undefined
   *  (treated as true — anchor as soon as xml is present). */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served drawio viewer at `/widget/drawio` for a given
 * `.drawio` XML value. View-only in v1: a "Save .drawio" button does
 * a frontend Blob download for external editing.
 *
 * @example
 * ```tsx
 * <UIDrawio
 *   value={{ xml: drawioXml, filename: 'architecture.drawio' }}
 *   onReady={() => console.log('viewer ready')}
 * />
 * ```
 */
export function UIDrawio({
  value,
  universe,
  onReady,
  onError,
  onStatusChange,
  initialHeight = 600,
  maxHeight = 2000,
  pushValue,
  fillHeight,
  display,
  editable,
  onChange,
  onExit,
  streamingComplete,
}: UIDrawioProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'error':
          onError?.(msg.error)
          return
        // drawio v1 doesn't emit change/submit/action — view-only.
      }
    },
    [onReady, onError],
  )

  // The diagram value is delivered via postMessage (pushValue), NOT the
  // `?value=` URL param. A real .drawio is large (10KB+ XML → 30-50KB
  // base64), and a URL that long makes the bun harness inside the iframe
  // never emit `ready` ("ready-timeout" → blank-white viewer). postMessage
  // has no such size limit. IframeWidget posts `pushValue` to the iframe as
  // soon as it's ready (and on every change), and the in-iframe harness
  // updates DrawioView. (Previously the initial value went via the URL,
  // which only worked for small diagrams.)
  const xmlReady = !!(value?.xml && String(value.xml).trim())

  // Editable mode: bypass the IframeWidget pipeline entirely. The
  // DrawioEditor mounts the vendored drawio webapp directly and bridges
  // drawio's own embed protocol (init/load/save/exit) to React callbacks.
  // See modules/drawio/docs/viewer-and-streaming.md.
  if (editable) {
    const xml = (pushValue?.xml ?? value.xml) ?? ''
    return (
      <DrawioEditor
        xml={xml}
        universe={universe}
        onSave={onChange}
        onExit={onExit}
      />
    )
  }

  // The iframe always mounts WITHOUT a `?value=` param — it shows the shared
  // loading screen until the value is delivered via postMessage (below).
  const extraParams: Record<string, string> = { display: display ?? 'ui' }

  // Deliver the diagram via postMessage: an explicit live `pushValue` (cross-
  // turn patch) wins; otherwise push the artifact's own `value` once its xml
  // is ready. IframeWidget posts this after the iframe reports `ready`.
  const valuePush = pushValue ?? (xmlReady ? value : undefined)

  return (
    <IframeWidget
      kind="drawio"
      universe={universe}
      extraParams={extraParams}
      onIframeMessage={onIframeMessage}
      onStatusChange={onStatusChange}
      initialHeight={initialHeight}
      maxHeight={maxHeight}
      pushValue={valuePush}
      fillHeight={fillHeight}
    />
  )
}
