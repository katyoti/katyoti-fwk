// SPDX-License-Identifier: Apache-2.0
// DrawioView — view-only drawio renderer.
//
// V1 scope (gui-drawio M1, 2026-05-07):
//   - Lazy-loads viewer-static.min.js on first mount via viewer-shim
//   - Renders the XML via drawio's documented data-mxgraph pattern
//   - Empty / malformed XML → discreet placeholder; safety-nets the
//     auto-close shim that lands in M2
//
// Save handling: deliberately delegated to the host (DocumentTab's
// DocHeader carries the Save button). Avoids two visible Save buttons
// in the streaming-document case. Standalone callers (DrawioDemoPage)
// can save via a host-side button using the same DrawioValue.xml.
//
// Reactive: `xml` flows in as a prop; on change, the viewer is re-mounted
// (drawio has no in-place setXml). The host's `pushValue` postMessage
// pipeline already drives this.

import { Box, Notification, Stack } from '@mantine/core'
import { useThrottledValue } from '@mantine/hooks'
import { useEffect, useMemo, useRef, useState } from 'react'

import { autoCloseXml } from './auto-close.ts'
import { createDrawioViewer, loadDrawioViewer, type DrawioViewerHandle } from './viewer-shim.ts'
import { LoadingView } from '../../ui-core/loading/index.ts'

export interface DrawioValue {
  /** mxGraph XML — typically a `<mxfile>...<mxGraphModel>...</mxGraphModel>...</mxfile>`. */
  xml: string
  /** Suggested filename for the Save button. Default: `'diagram.drawio'`. */
  filename?: string
  /** Reserved for future use (svg+xml-with-embedded-XML; multi-page nav). */
  format?: 'mxgraph' | 'svg+xml'
  page?: number
}

export interface DrawioViewProps {
  value: DrawioValue
  /** Minimum container height in pixels. The view fills its parent by
   *  default (`height: '100%'`); the floor kicks in when the parent
   *  has no intrinsic height (display=ui wrapper, no flex chain). */
  minHeight?: number
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
}

export function DrawioView({ value, minHeight = 480, attrs }: DrawioViewProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const handleRef = useRef<DrawioViewerHandle | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)

  // Run the auto-close shim on every XML change. For valid full
  // documents this is a no-op (returns the input unchanged); for
  // partial buffers (mid-stream Mode B) it appends balancing closes
  // so each tick produces a renderable doc. See ./auto-close.ts and
  // modules/drawio/docs/viewer-and-streaming.md §3.
  const balancedXml = useMemo(() => autoCloseXml(value.xml ?? ''), [value.xml])

  // THROTTLE viewer remounts to at most once per 2s. Each remount rebuilds
  // the viewer + re-fits; doing that on every streamed chunk re-zooms
  // constantly (visible churn). useThrottledValue renders the first value
  // immediately (so a complete diagram shows at once) then at most every 2s
  // while the XML keeps changing, with a final trailing render when it
  // settles — i.e. "max one re-scale per 2 seconds" during streaming.
  const debouncedXml = useThrottledValue(balancedXml, 2000)

  // 1. Lazy-load the vendored viewer once. Component-lifetime: the script
  //    stays in the document until the page unloads (cheaply re-used
  //    across drawio mounts within one iframe session).
  useEffect(() => {
    let cancelled = false
    loadDrawioViewer()
      .then(() => {
        if (!cancelled) setLoaded(true)
      })
      .catch((err: Error) => {
        if (!cancelled) setLoadError(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [])

  // 2. Once loaded, mount/remount the viewer whenever the *debounced*
  //    balanced XML changes. Drawio has no in-place setXml — we rebuild
  //    the in-element DOM each time. The 200 ms debounce keeps the
  //    viewer from being saturated during fast streams (Mode B).
  useEffect(() => {
    if (!loaded) return
    const el = containerRef.current
    if (!el) return
    if (!debouncedXml) {
      // Empty input (or no complete tag yet) — clear the prior viewer
      // if any and skip mount.
      handleRef.current?.destroy()
      handleRef.current = null
      return
    }
    try {
      if (handleRef.current) {
        handleRef.current.setXml(debouncedXml)
      } else {
        handleRef.current = createDrawioViewer(el, debouncedXml)
      }
    } catch (err) {
      // Rendering errors land here when XML is malformed beyond what
      // the viewer can recover. Keep last-good behaviour: don't crash;
      // surface a small notification.
      // eslint-disable-next-line no-console
      console.warn('[drawio] render failed:', err)
    }
  }, [loaded, debouncedXml])

  // 3. Cleanup on unmount.
  useEffect(() => {
    return () => {
      handleRef.current?.destroy()
      handleRef.current = null
    }
  }, [])

  if (loadError) {
    return (
      <Notification {...attrs} color="red" withCloseButton={false} title="Drawio viewer failed to load">
        {loadError}
      </Notification>
    )
  }

  const hasXml = !!debouncedXml

  return (
    <Stack
      {...attrs}
      gap={0}
      style={{ height: '100%', minHeight, width: '100%' }}
      data-testid="drawio.root"
    >
      {/* Viewer mount point. drawio writes SVG + chrome here. The whole
          surface is FIXED WHITE — theme-independent — by architectural
          convention: diagrams are authored on a white canvas, and drawio's
          shape fills (light blue / green / orange / pink) + edge labels are
          designed against white, becoming invisible on a dark surface. So we
          do NOT follow the host's dark theme here; the letterbox around the
          diagram stays white to match the SVG page inside it. */}
      <Box
        style={{
          flex: 1,
          minHeight: 0,
          position: 'relative',
          overflow: 'auto',
          background: '#ffffff',
          color: '#000',
        }}
      >
        {/* While the viewer script is loading OR the diagram XML hasn't
            arrived yet (it streams in via postMessage), show the shared
            KaTyoti loading animation instead of a bare canvas. */}
        {(!loaded || !hasXml) && <LoadingView noun="diagram" />}
        <div
          ref={containerRef}
          style={{ width: '100%', height: '100%' }}
          data-testid="drawio.viewer"
        />
      </Box>
    </Stack>
  )
}
