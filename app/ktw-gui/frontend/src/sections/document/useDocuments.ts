// SPDX-License-Identifier: Apache-2.0
/**
 * Document animation hook — plays the mock delta timeline.
 * Writes documents to the central Zustand store (shared with real terminal events).
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useAppStore } from '../../store'
import { MOCK_DOCS, buildTimeline } from './mockDocData'

export type { DocumentState } from '../../store'

export function useDocuments(sessionId?: string) {
  // Per-session scoping: a ChatPanel passes its own sessionId and sees ONLY
  // documents that session produced. Parallel chats never share doc tabs.
  // A session-scoped panel requires an EXACT sessionId match — a doc with no
  // sessionId is NOT shown here (it used to bleed into every panel, which is
  // the wrong-session bug). Only a panel with no sessionId (none in the live
  // tab system) falls back to showing session-less docs.
  const docs = useAppStore((s) =>
    Array.from(s.documents.values()).filter((d) =>
      sessionId ? d.sessionId === sessionId : !d.sessionId,
    ),
  )
  const openDocument = useAppStore((s) => s.openDocument)
  const appendDocumentDelta = useAppStore((s) => s.appendDocumentDelta)
  const finalizeDocument = useAppStore((s) => s.finalizeDocument)
  const clearDocuments = useAppStore((s) => s.clearDocuments)
  const closeDoc = useAppStore((s) => s.closeDocument)

  const [playing, setPlaying] = useState(false)
  const timelineRef = useRef(buildTimeline())
  const indexRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const applyDelta = useCallback((docId: string, delta: string) => {
    const store = useAppStore.getState()
    if (store.documents.has(docId)) {
      appendDocumentDelta(docId, delta)
    } else {
      // First delta — create the document from mock metadata.
      const info = MOCK_DOCS.find(d => d.id === docId)
      if (info) {
        openDocument({
          id: docId,
          title: info.title,
          mediaType: info.mediaType,
          content: delta,
          status: 'streaming',
        })
      }
    }
  }, [appendDocumentDelta, openDocument])

  const playNext = useCallback(() => {
    const timeline = timelineRef.current
    const idx = indexRef.current
    if (idx >= timeline.length) {
      // Mark all docs done.
      const store = useAppStore.getState()
      store.documents.forEach((_, id) => finalizeDocument(id))
      setPlaying(false)
      return
    }

    const step = timeline[idx]
    applyDelta(step.docId, step.delta)
    indexRef.current = idx + 1

    if (indexRef.current < timeline.length) {
      timerRef.current = setTimeout(playNext, timeline[indexRef.current].delayMs)
    } else {
      timerRef.current = setTimeout(() => {
        const store = useAppStore.getState()
        store.documents.forEach((_, id) => finalizeDocument(id))
        setPlaying(false)
      }, 500)
    }
  }, [applyDelta, finalizeDocument])

  const play = useCallback(() => {
    if (playing) return
    if (indexRef.current >= timelineRef.current.length) {
      reset()
      setTimeout(() => {
        setPlaying(true)
        timerRef.current = setTimeout(playNext, timelineRef.current[0].delayMs)
      }, 100)
      return
    }
    setPlaying(true)
    const delay = indexRef.current === 0
      ? timelineRef.current[0].delayMs
      : 100
    timerRef.current = setTimeout(playNext, delay)
  }, [playing, playNext])

  const pause = useCallback(() => {
    setPlaying(false)
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const reset = useCallback(() => {
    pause()
    clearDocuments()
    indexRef.current = 0
    timelineRef.current = buildTimeline()
  }, [pause, clearDocuments])

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  return {
    docs,
    playing,
    play,
    pause,
    reset,
    closeDoc,
  }
}
