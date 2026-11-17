// SPDX-License-Identifier: Apache-2.0
/**
 * Media slice — camera PiP + screen recording state.
 *
 * Pure browser APIs (getUserMedia + getDisplayMedia + MediaRecorder).
 * The camera stream lives in this store so the IconRibbon button and
 * the CameraOverlay component can both read/toggle it without prop
 * drilling. Recording uses getDisplayMedia which captures the visible
 * window content — so if the camera overlay is mounted on the page,
 * it appears in the recording naturally (no manual canvas compositing
 * needed in v1).
 *
 * Per gui-presenter-recorder feature scoped 2026-05-07.
 */

import { create } from 'zustand'

interface MediaState {
  /** Live camera MediaStream when the camera is on; null otherwise. */
  cameraStream: MediaStream | null
  /** Last error message from getUserMedia (permission denied / no device). */
  cameraError: string | null
  /** True between record-start and stop. */
  recording: boolean
  /** Active MediaRecorder instance — kept here so the toggle can stop it. */
  recorder: MediaRecorder | null
  /** Active screen MediaStream during recording. Released on stop. */
  screenStream: MediaStream | null
  /** Last error message from getDisplayMedia / MediaRecorder. */
  recordingError: string | null
  /** Blur the camera background (Teams/Meet-style). Persisted; default on. */
  cameraBlur: boolean

  toggleCamera: () => Promise<void>
  setCameraBlur: (v: boolean) => void
  startRecording: () => Promise<void>
  stopRecording: () => void
}

// Persisted camera-blur preference (default ON).
const BLUR_KEY = 'ktw.camera.blur'
function loadBlurPref(): boolean {
  try { return localStorage.getItem(BLUR_KEY) !== '0' } catch { return true }
}

export const useMediaStore = create<MediaState>((set, get) => ({
  cameraStream: null,
  cameraError: null,
  recording: false,
  recorder: null,
  screenStream: null,
  recordingError: null,
  cameraBlur: loadBlurPref(),

  setCameraBlur: (v: boolean) => {
    try { localStorage.setItem(BLUR_KEY, v ? '1' : '0') } catch { /* ignore */ }
    set({ cameraBlur: v })
  },

  toggleCamera: async () => {
    const current = get().cameraStream
    if (current) {
      // Camera on → turn off: stop all tracks + release.
      current.getTracks().forEach((t) => t.stop())
      set({ cameraStream: null, cameraError: null })
      return
    }
    // Camera off → request permission + start stream.
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })
      set({ cameraStream: stream, cameraError: null })
    } catch (err) {
      const message = (err as Error).message || 'failed to access camera'
      // eslint-disable-next-line no-console
      console.error('[media] camera start failed:', err)
      set({ cameraError: message })
    }
  },

  startRecording: async () => {
    if (get().recording) return
    try {
      // getDisplayMedia captures the screen / window / tab the user picks.
      // The browser shows its own picker UI; we don't pre-select.
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: true,
        // Audio capture from the screen share is opt-in by the user in
        // the picker; ask for it but accept whatever they grant.
        audio: true,
      })

      // Pick the best supported codec. webm (VP9 then VP8) is the broad
      // baseline; mp4 is opt-in and rarely supported by MediaRecorder
      // on Chromium without flags.
      const mimeType = pickMimeType()
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined)

      const chunks: Blob[] = []
      recorder.addEventListener('dataavailable', (e) => {
        if (e.data && e.data.size > 0) chunks.push(e.data)
      })
      recorder.addEventListener('stop', () => {
        // Release the underlying stream.
        stream.getTracks().forEach((t) => t.stop())
        set({ recording: false, recorder: null, screenStream: null })

        if (chunks.length === 0) return
        const blob = new Blob(chunks, { type: recorder.mimeType || 'video/webm' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `ktw-recording-${Date.now()}.${recorder.mimeType?.includes('mp4') ? 'mp4' : 'webm'}`
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        URL.revokeObjectURL(url)
      })

      // If the user cancels the share via the browser's own UI (e.g.
      // clicks the "Stop sharing" pill), the video track ends; we
      // mirror that into our state so the button flips back.
      stream.getVideoTracks()[0]?.addEventListener('ended', () => {
        const r = get().recorder
        if (r && r.state !== 'inactive') r.stop()
      })

      recorder.start(1000) // request a chunk every 1s for streaming-friendly Blobs
      set({ recording: true, recorder, screenStream: stream, recordingError: null })
    } catch (err) {
      const message = (err as Error).message || 'failed to start recording'
      // eslint-disable-next-line no-console
      console.error('[media] recording start failed:', err)
      set({ recordingError: message })
    }
  },

  stopRecording: () => {
    const r = get().recorder
    if (r && r.state !== 'inactive') {
      r.stop() // triggers the 'stop' listener → finalise blob + reset state
    } else {
      // Defensive — if the recorder isn't running but state says so, reset.
      set({ recording: false, recorder: null, screenStream: null })
    }
  },
}))

/** Pick the most-supported MediaRecorder mime type. webm/VP9 first
 *  (best compression on Chromium); fall back to webm/VP8, then plain
 *  webm, then undefined (lets MediaRecorder pick). */
function pickMimeType(): string | undefined {
  if (typeof MediaRecorder === 'undefined') return undefined
  const candidates = [
    'video/webm;codecs=vp9,opus',
    'video/webm;codecs=vp8,opus',
    'video/webm;codecs=vp9',
    'video/webm;codecs=vp8',
    'video/webm',
  ]
  for (const c of candidates) {
    if (MediaRecorder.isTypeSupported(c)) return c
  }
  return undefined
}
