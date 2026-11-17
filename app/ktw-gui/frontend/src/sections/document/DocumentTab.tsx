// SPDX-License-Identifier: Apache-2.0
/**
 * Document tab — a chat document or a streamed LLM artifact, shown through
 * the one document entry point (DocumentView). What is specific to the tab
 * is where the content lives: inline for a document, in the artifacts store
 * for an artifact (patches land there), with the artifact's interaction
 * wiring (Mode-A round-trips, Mode-B shortcuts) and in-flight feedback.
 */
import { useEffect, useRef, useState } from 'react'
import { Loader, Stack, Text } from '@mantine/core'
import type { DocumentState } from './useDocuments'
import { SendUIEvent } from '../../../wailsjs/go/main/App'
import { useArtifactsStore } from '../../store/artifactsSlice'
import { useAppStore } from '../../store'
import { dispatchActionShortcut } from '../../lib/action-shortcut'
import { DocumentView } from '../../components/document/DocumentView'
import { artifactDocument, artifactValueFromContent } from '../../components/document/artifact'
import { useT } from '../../i18n'

interface Props {
  doc: DocumentState
}

export function DocumentTab({ doc }: Props) {
  // A hook before the early return: the artifact branch is its own component.
  const setDocumentContent = useAppStore((s) => s.setDocumentContent)
  if (doc.artifactId) {
    return <ArtifactTab doc={doc} artifactId={doc.artifactId} />
  }
  return (
    <DocumentView
      doc={{ mediaType: doc.mediaType, title: doc.title, content: doc.content }}
      streaming={doc.status === 'streaming'}
      header
      // An engine that edits (draw.io) commits here: the tab keeps the
      // user's version.
      onContentChange={(content) => setDocumentContent(doc.id, typeof content === 'string' ? content : JSON.stringify(content))}
    />
  )
}

function ArtifactTab({ doc, artifactId }: Props & { artifactId: string }) {
  const t = useT('screens')
  const artifact = useArtifactsStore((s) => s.artifacts.get(artifactId))
  const setArtifactValue = useArtifactsStore((s) => s.setArtifactValue)

  // Stream-stability heuristic: PERSISTENT artifacts never reach status
  // 'complete' (the agent closes them explicitly via complete-ui-component),
  // so gating the compositions' URL anchor on completion alone left their
  // iframes on the skeleton page forever. The store emits a fresh artifact
  // object per mount/patch — when no change lands for 800ms the stream has
  // settled and anchoring is safe.
  const [streamStable, setStreamStable] = useState(false)
  useEffect(() => {
    if (!artifact || artifact.status === 'complete') return
    setStreamStable(false)
    const t = window.setTimeout(() => setStreamStable(true), 800)
    return () => window.clearTimeout(t)
  }, [artifact])

  // Mode-A in-flight feedback (US-S3 UX): a round-trip through the agent
  // takes seconds with no visual reaction — without feedback the user clicks
  // again (flooding the prompt gate with stale events). While an action is
  // pending we show a working overlay and drop further actions until the
  // agent reacts (any artifact mounts/changes) or 15s elapses.
  const [pendingAction, setPendingAction] = useState<string | null>(null)
  const pendingTimerRef = useRef<number | undefined>(undefined)
  const artifactCount = useArtifactsStore((s) => s.artifacts.size)
  const baselineCountRef = useRef(artifactCount)
  if (pendingAction === null) {
    baselineCountRef.current = artifactCount
  } else if (artifactCount !== baselineCountRef.current) {
    window.clearTimeout(pendingTimerRef.current)
    setPendingAction(null)
  }
  const beginPendingAction = (action: string) => {
    setPendingAction(action)
    window.clearTimeout(pendingTimerRef.current)
    pendingTimerRef.current = window.setTimeout(() => setPendingAction(null), 15000)
  }

  if (!artifact) {
    return (
      <Stack p="md" align="center">
        <Loader size="sm" />
        <Text size="sm" c="dimmed">
          {t('document.waiting', { id: artifactId })}
        </Text>
      </Stack>
    )
  }

  const complete = artifact.status === 'complete'
  return (
    <DocumentView
      doc={artifactDocument(artifact, doc.title)}
      streaming={!complete}
      header
      overlay={pendingAction ? <PendingOverlay action={pendingAction} /> : undefined}
      onContentChange={(content) => setArtifactValue({ artifactId, value: artifactValueFromContent(artifact, content) })}
      host={{
        fillHeight: true,
        // Defer URL anchoring until streaming settles — the partial-JSON
        // parser emits intermediate schemas / trees that pass the readiness
        // heuristic but miss fields still streaming.
        streamingComplete: complete || streamStable,
        // Mode A: the interaction flows back to the agent as a ui_event,
        // delivered as the pending ask-user prompt's answer.
        onSubmit: (value) => {
          if (pendingAction) return // one in-flight interaction per artifact
          beginPendingAction('submit')
          void SendUIEvent(artifactId, 'submit', JSON.stringify(value))
        },
        // Mode B first: a declared shortcut is handled here, instantly;
        // otherwise Mode A.
        onAction: (action, args) => {
          const shortcut = artifact.actions?.[action]
          if (shortcut && dispatchActionShortcut(shortcut, args as Record<string, unknown>)) return
          if (pendingAction) return
          beginPendingAction(action)
          void SendUIEvent(artifactId, action, JSON.stringify(args))
        },
      }}
    />
  )
}

function PendingOverlay({ action }: { action: string }) {
  const t = useT('screens')
  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        zIndex: 20,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'color-mix(in srgb, var(--mantine-color-body) 60%, transparent)',
        backdropFilter: 'blur(1.5px)',
      }}
    >
      <Stack
        align="center"
        gap="sm"
        p="lg"
        style={{
          background: 'var(--mantine-color-body)',
          border: '1px solid var(--mantine-color-default-border)',
          borderRadius: 8,
          boxShadow: 'var(--mantine-shadow-md)',
          maxWidth: 360,
        }}
      >
        <Loader size="md" />
        <Text size="sm" fw={600} ta="center">
          {action === 'submit' ? t('document.formSubmitted') : t('document.actionSent', { action })}
        </Text>
        <Text size="xs" c="dimmed" ta="center">
          {t('document.wait')}
        </Text>
      </Stack>
    </div>
  )
}
