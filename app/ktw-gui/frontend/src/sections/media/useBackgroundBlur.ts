// SPDX-License-Identifier: Apache-2.0
/**
 * useBackgroundBlur — Teams/Meet-style background blur for a <video>.
 *
 * Runs MediaPipe Selfie Segmentation (ImageSegmenter, VIDEO mode) over the
 * source <video> each frame and composites a sharp foreground (you) over a
 * Gaussian-blurred background into the target <canvas>. WASM + the .tflite
 * model are bundled under public/mediapipe/ (no network needed).
 *
 * Returns `active` — true once segmentation is producing frames. If init
 * fails (e.g. WASM can't load), `active` stays false and the caller shows
 * the raw video instead, so the camera never breaks.
 */

import { useEffect, useState } from 'react'
import { FilesetResolver, ImageSegmenter } from '@mediapipe/tasks-vision'

// One segmenter for the whole app (a single camera). VIDEO mode is stateful
// on monotonically-increasing timestamps, which performance.now() satisfies.
let segmenterPromise: Promise<ImageSegmenter> | null = null
function getSegmenter(): Promise<ImageSegmenter> {
  if (!segmenterPromise) {
    segmenterPromise = (async () => {
      const fileset = await FilesetResolver.forVisionTasks('/mediapipe/wasm')
      return ImageSegmenter.createFromOptions(fileset, {
        baseOptions: { modelAssetPath: '/mediapipe/selfie_segmenter.tflite', delegate: 'GPU' },
        runningMode: 'VIDEO',
        outputConfidenceMasks: true,
        outputCategoryMask: false,
      })
    })().catch((e) => {
      segmenterPromise = null // allow a retry next toggle
      throw e
    })
  }
  return segmenterPromise
}

export function useBackgroundBlur(
  videoRef: React.RefObject<HTMLVideoElement | null>,
  canvasRef: React.RefObject<HTMLCanvasElement | null>,
  enabled: boolean,
  blurPx = 12,
): { active: boolean } {
  const [active, setActive] = useState(false)

  useEffect(() => {
    if (!enabled) { setActive(false); return }
    let stopped = false
    let raf = 0
    let segmenter: ImageSegmenter | null = null
    const person = document.createElement('canvas')

    const loop = () => {
      if (stopped) return
      const video = videoRef.current
      const canvas = canvasRef.current
      if (video && canvas && segmenter && video.readyState >= 2 && video.videoWidth > 0) {
        try {
          const res = segmenter.segmentForVideo(video, performance.now())
          const mask = res.confidenceMasks?.[0]
          if (mask) {
            const mw = mask.width, mh = mask.height
            const conf = mask.getAsFloat32Array()

            // Foreground layer: person alpha cut from the sharp video.
            person.width = mw; person.height = mh
            const pctx = person.getContext('2d')!
            const alpha = pctx.createImageData(mw, mh)
            for (let i = 0; i < conf.length; i++) alpha.data[i * 4 + 3] = (conf[i] * 255) | 0
            pctx.putImageData(alpha, 0, 0)
            pctx.globalCompositeOperation = 'source-in'
            pctx.drawImage(video, 0, 0, mw, mh)
            pctx.globalCompositeOperation = 'source-over'
            mask.close()

            // Composite: blurred background, sharp person on top.
            const cw = canvas.width, ch = canvas.height
            const cctx = canvas.getContext('2d')!
            cctx.filter = `blur(${blurPx}px)`
            cctx.drawImage(video, 0, 0, cw, ch)
            cctx.filter = 'none'
            cctx.drawImage(person, 0, 0, cw, ch)
            if (!stopped) setActive(true)
          }
        } catch { /* drop this frame */ }
      }
      raf = requestAnimationFrame(loop)
    }

    getSegmenter()
      .then((s) => { if (!stopped) { segmenter = s; loop() } })
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.error('[camera-blur] segmenter init failed — showing raw video', e)
        if (!stopped) setActive(false)
      })

    return () => {
      stopped = true
      if (raf) cancelAnimationFrame(raf)
      setActive(false)
      // Keep the cached segmenter alive for the next toggle.
    }
  }, [enabled, videoRef, canvasRef, blurPx])

  return { active }
}
