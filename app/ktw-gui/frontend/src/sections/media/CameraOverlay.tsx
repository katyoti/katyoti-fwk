// SPDX-License-Identifier: Apache-2.0
/**
 * CameraOverlay — draggable PiP video overlay rendered at root level.
 *
 * Mounted by BaseLayout; renders only when useMediaStore.cameraStream
 * is non-null. Position is local state (top-right by default; user
 * drags to reposition). The overlay is a sibling of the IconRibbon +
 * main pane so it floats above all routes.
 *
 * Per gui-presenter-recorder feature (2026-05-07).
 */

import { useEffect, useRef, useState } from 'react'
import { ActionIcon, Box, Tooltip } from '@mantine/core'
import { IconX, IconGripVertical } from '@tabler/icons-react'
import { useMediaStore } from '../../store/mediaSlice'
import { useBackgroundBlur } from './useBackgroundBlur'
import { useT } from '../../i18n'

// Square dimensions so border-radius:50% gives a perfect circle. A
// 180x180 PiP feels right at typical screen sizes — large enough to
// see the speaker's face, small enough not to occlude content.
const DEFAULT_WIDTH = 180
const DEFAULT_HEIGHT = 180
const MARGIN = 16

export function CameraOverlay() {
  const t = useT('screens')
  const stream = useMediaStore((s) => s.cameraStream)
  const toggleCamera = useMediaStore((s) => s.toggleCamera)
  const cameraBlur = useMediaStore((s) => s.cameraBlur)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  // Background blur. `active` is true only once segmentation is actually
  // producing frames; until then (or on init failure) we show the raw video.
  const { active: blurActive } = useBackgroundBlur(videoRef, canvasRef, cameraBlur)

  // Position — initialised to top-right corner on first mount.
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const [dragging, setDragging] = useState(false)
  const dragOffsetRef = useRef<{ dx: number; dy: number } | null>(null)

  useEffect(() => {
    if (!stream) return
    const v = videoRef.current
    if (!v) return
    v.srcObject = stream
    v.play().catch(() => {
      // Autoplay can be blocked when there's no user gesture; the
      // toggle button click IS a gesture, so this is rare. Silent.
    })
    return () => {
      // Detach without stopping the stream — the store owns the stream
      // lifecycle, not the overlay.
      v.srcObject = null
    }
  }, [stream])

  // Initialise top-right corner once the overlay mounts.
  useEffect(() => {
    if (pos !== null) return
    if (typeof window === 'undefined') return
    setPos({ x: window.innerWidth - DEFAULT_WIDTH - MARGIN, y: MARGIN })
  }, [pos])

  // Drag handlers — pointer events on `window` (fire reliably during a
  // drag regardless of what's under the cursor; no pointer-capture needed).
  useEffect(() => {
    if (!dragging) return
    const onMove = (e: PointerEvent) => {
      const off = dragOffsetRef.current
      if (!off) return
      // Clamp to viewport so the overlay never goes fully off-screen.
      const x = Math.max(0, Math.min(window.innerWidth - DEFAULT_WIDTH, e.clientX - off.dx))
      const y = Math.max(0, Math.min(window.innerHeight - DEFAULT_HEIGHT, e.clientY - off.dy))
      setPos({ x, y })
    }
    const onUp = () => setDragging(false)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [dragging])

  // Keep the overlay inside the viewport when the window is resized —
  // otherwise a top-right PiP drifts off-screen when the window shrinks
  // (fixed left/top doesn't track the corner).
  useEffect(() => {
    const onResize = () => {
      setPos((p) => (p ? {
        x: Math.max(0, Math.min(window.innerWidth - DEFAULT_WIDTH, p.x)),
        y: Math.max(0, Math.min(window.innerHeight - DEFAULT_HEIGHT, p.y)),
      } : p))
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  // Start a drag from anywhere on the overlay body (the whole PiP is the
  // grab surface — the tiny grip alone was hard to hit). Interactive
  // controls (the close button) stop propagation so they don't start one.
  const startDrag = (e: React.PointerEvent) => {
    if (!pos) return
    dragOffsetRef.current = { dx: e.clientX - pos.x, dy: e.clientY - pos.y }
    setDragging(true)
  }

  if (!stream || !pos) return null

  return (
    <Box
      onPointerDown={startDrag}
      style={{
        position: 'fixed',
        left: pos.x,
        top: pos.y,
        width: DEFAULT_WIDTH,
        height: DEFAULT_HEIGHT,
        zIndex: 9999,
        background: '#000',
        border: '2px solid var(--mantine-color-default-border)',
        borderRadius: '50%',
        overflow: 'hidden',
        boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4)',
        userSelect: 'none',
        cursor: dragging ? 'grabbing' : 'grab',
        touchAction: 'none',
      }}
    >
      <video
        ref={videoRef}
        autoPlay
        muted
        playsInline
        style={{
          width: '100%', height: '100%', objectFit: 'cover',
          // When blur is active the canvas shows the composited frame and the
          // raw video is the (hidden) source feeding the segmenter.
          display: blurActive ? 'none' : 'block',
        }}
      />
      <canvas
        ref={canvasRef}
        width={256}
        height={256}
        style={{
          width: '100%', height: '100%', objectFit: 'cover',
          display: blurActive ? 'block' : 'none',
        }}
      />

      {/* Top control strip — drag handle + close button. Overlays the
          video; visible on hover or always-on for discoverability. */}
      <Box
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          padding: '4px 6px',
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          background: 'linear-gradient(to bottom, rgba(0,0,0,0.6), transparent)',
        }}
      >
        <Tooltip label={t('camera.drag')} position="bottom">
          <ActionIcon
            size="xs"
            variant="subtle"
            color="gray"
            style={{ cursor: dragging ? 'grabbing' : 'grab', color: 'white' }}
          >
            <IconGripVertical size={12} />
          </ActionIcon>
        </Tooltip>
        <Box style={{ flex: 1 }} />
        <Tooltip label={t('camera.off')} position="bottom">
          <ActionIcon
            size="xs"
            variant="subtle"
            color="gray"
            style={{ color: 'white' }}
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => void toggleCamera()}
            aria-label={t('camera.off')}
          >
            <IconX size={12} />
          </ActionIcon>
        </Tooltip>
      </Box>
    </Box>
  )
}
