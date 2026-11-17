// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState } from 'react'
import { ActionIcon, Box, Button, Group, SegmentedControl, Stack, Tabs, Text, Tooltip } from '@mantine/core'
import {
  IconMessage, IconUsers, IconFile, IconGlobe, IconX,
  IconPlayerStop,
  IconCircleCheck, IconCircleX, IconCircleMinus,
  IconFileText, IconBug, IconList, IconCoin,
} from '@tabler/icons-react'
import { StopTask } from '../../../wailsjs/go/main/App'
import { ChatHeader }       from './ChatHeader'
import { MessagePanel }      from './MessagePanel'
import { ChatInputPanel }    from './ChatInputPanel'
import { StatusBar }         from './StatusBar'
import { TeamPanel }         from '../team/TeamPanel'
import { DocumentTab }       from '../document/DocumentTab'
import { useDocuments }      from '../document/useDocuments'
import { useAppStore, useSessionMessages, useSessionStatus, useSessionPrompt, useSessionName, useSessionSummary } from '../../store'
import { SessionOutputPanel } from '../shell/SessionOutputPanel'
import { SessionJournalPanel } from '../shell/SessionJournalPanel'
import { SessionCostPanel } from '../shell/SessionCostPanel'
import { HttpPanel } from '../http/HttpPanel'
import { ConversationView, liveMessagesToTranscript } from '../conversation/ConversationView'
import { useT } from '../../i18n'
import { landsOnOutput, runFailureReason } from './runFailure'

/** Full chat panel with Chat/Team/Document tabs.
 *  Mounted by MainArea exclusively when a task is running (TaskMode). The
 *  task-gallery and run-task views live OUTSIDE this component. */
/** A ChatPanel is a PER-RUN display: it is bound to ONE session (`sessionId`,
 *  the run's journal id) and reads THAT session's bucket — messages, prompt,
 *  stats, status — directly. Two parallel chats are two ChatPanels bound to
 *  two sessions; they never share content (no global mirror). */
export function ChatPanel({ sessionId, scopedNodeName }: { sessionId?: string; scopedNodeName?: string } = {}) {
  const focusSession = useAppStore((s) => s.focusSession)
  const t = useT('chat')
  // Mark this run focused on the Go side (so AnswerPrompt & friends target
  // it) + for the Running-panel display. Content does NOT depend on focus —
  // it reads this session's bucket explicitly below.
  useEffect(() => {
    if (sessionId) focusSession(sessionId)
  }, [sessionId, focusSession])

  const [activeTab, setActiveTab] = useState<string>('chat')
  // Debug switch: a toggle that adds/removes the inspector tabs
  // (Transcript / Teams / Cost / HTTP) — the SAME set the finished
  // SessionViewer shows, so a running and a done session read identically.
  // Off by default so the normal chat stays uncluttered.
  const [debugMode, setDebugMode] = useState(false)
  // Transcript density — mirrors the SessionViewer: Detailed = LLM journal,
  // Reader = clean speaker→message view. One tool, two projections (replaces
  // the old separate Journal + Conversation debug tabs).
  const [density, setDensity] = useState<'reader' | 'detailed'>('detailed')
  const { docs, closeDoc } = useDocuments(sessionId)
  // THIS session's state — explicit per-run targeting (not the global mirror).
  const sessionStatus      = useSessionStatus(sessionId)
  const messages           = useSessionMessages(sessionId)
  const activePrompt       = useSessionPrompt(sessionId)
  const sessionName        = useSessionName(sessionId)
  const sessionSummary     = useSessionSummary(sessionId)
  const dismissRunningTask = useAppStore((s) => s.dismissRunningTask)
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const isTerminal = sessionStatus === 'done' || sessionStatus === 'error' || sessionStatus === 'stopped'
  // Why a failed run failed: its last error message, said in the banner too.
  const failureReason = sessionStatus === 'error' ? runFailureReason(messages) : undefined

  // Live conversation transcript — the same speaker→message view the
  // session viewer uses, fed from the live message store via the
  // shared adapter. The pending ask-user prompt lives in activePrompt
  // (a separate slice from the message thread), so append it as the
  // trailing entry — otherwise a turn whose only output was an
  // ask-user call (no agent text) would render an empty conversation.
  const liveTranscript = useMemo(() => {
    const tr = liveMessagesToTranscript(messages)
    if (activePrompt?.prompt) {
      tr.push({ type: 'prompt_shown', timestamp: '', data: { prompt: activePrompt.prompt } })
    }
    return tr
  }, [messages, activePrompt])

  // When the debug switch turns off while an inspector tab is active,
  // fall back to the live chat so the body never goes blank.
  useEffect(() => {
    if (!debugMode && (activeTab === 'team' || activeTab === 'transcript' || activeTab === 'cost' || activeTab === 'http')) {
      setActiveTab('chat')
    }
  }, [debugMode, activeTab])

  // Show the Output tab once the task terminates. The session id is
  // populated by the `session.created` Wails event (emitted by
  // runViaUniverse after the server returned the pre-computed UUIDv7
  // from StartTask); SessionOutputPanel falls back to a
  // ListSessions-by-task lookup if the event hasn't fired for
  // any reason.
  // Summary-only results are shown as the final chat message (see
  // useAgentEvents `complete` handling), so the raw-JSON Output tab is
  // suppressed — no Output document for a plain summary.
  const showOutputTab = isTerminal && !!sessionId && !!displayedUniverseName && !sessionSummary

  // Auto-switch to Output once per session: when the task terminates and
  // the Output tab becomes visible, jump there so the user lands on the
  // structured result without having to hunt for it. Switch fires once
  // per session — keyed by THIS panel's sessionId.
  const [autoSwitchedFor, setAutoSwitchedFor] = useState<string | null>(null)
  useEffect(() => {
    if (
      showOutputTab &&
      sessionId &&
      sessionId !== autoSwitchedFor
    ) {
      // A run that failed with a reason stays on its chat, where it is.
      if (landsOnOutput(sessionStatus, failureReason)) setActiveTab('output')
      setAutoSwitchedFor(sessionId)
    }
  }, [showOutputTab, sessionId, autoSwitchedFor, sessionStatus, failureReason])

  // Auto-open first document tab when a new doc appears.
  const lastDocCount = docs.length
  const [prevDocCount, setPrevDocCount] = useState(0)
  if (lastDocCount > prevDocCount && lastDocCount > 0) {
    setPrevDocCount(lastDocCount)
    setActiveTab(`doc:${docs[lastDocCount - 1].id}`)
  }

  return (
    <div style={{ position: 'relative', height: '100%', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      <ChatHeader sessionId={sessionId} scopedNodeName={scopedNodeName} />

      {/* Tab strip */}
      <Tabs
        value={activeTab}
        onChange={(v) => setActiveTab(v || 'chat')}
        variant="outline"
        styles={{
          root: { flexShrink: 0, borderBottom: '1px solid var(--mantine-color-dark-4)' },
          list: { paddingLeft: 8, gap: 0, borderBottom: 'none' },
          tab: { fontSize: 12, padding: '4px 10px', fontWeight: 500 },
        }}
      >
        <Tabs.List>
          <Tabs.Tab value="chat" leftSection={<IconMessage size={13} />}>{t('tabs.chat')}</Tabs.Tab>
          {showOutputTab && (
            <Tabs.Tab value="output" leftSection={<IconFileText size={13} />}>{t('tabs.output')}</Tabs.Tab>
          )}
          {/* Inspector tabs — present only while the debug switch is on.
              SAME set + names as the finished SessionViewer. */}
          {debugMode && (
            <>
              <Tabs.Tab value="transcript" leftSection={<IconList size={13} />}>{t('tabs.transcript')}</Tabs.Tab>
              <Tabs.Tab value="team" leftSection={<IconUsers size={13} />}>{t('tabs.teams')}</Tabs.Tab>
              <Tabs.Tab value="cost" leftSection={<IconCoin size={13} />}>{t('tabs.cost')}</Tabs.Tab>
              <Tabs.Tab value="http" leftSection={<IconGlobe size={13} />}>{t('tabs.http')}</Tabs.Tab>
            </>
          )}

          {/* Dynamic document tabs */}
          {docs.map((doc) => (
            <Tabs.Tab
              key={doc.id}
              value={`doc:${doc.id}`}
              leftSection={doc.mediaType === 'text/html' ? <IconGlobe size={13} /> : <IconFile size={13} />}
              rightSection={
                <ActionIcon
                  size={14}
                  variant="subtle"
                  color="gray"
                  onClick={(e) => {
                    e.stopPropagation()
                    closeDoc(doc.id)
                    if (activeTab === `doc:${doc.id}`) setActiveTab('chat')
                  }}
                  style={{ marginLeft: 4 }}
                >
                  <IconX size={10} />
                </ActionIcon>
              }
            >
              {doc.title}
            </Tabs.Tab>
          ))}

          {/* Right-aligned controls */}
          <div style={{ flex: 1 }} />
          <Group gap={4} px={8} align="center" style={{ alignSelf: 'center' }}>
            {/* Debug switch — adds/removes the inspector tabs (Teams /
                Journal / Conversation). Same affordance as the left
                ribbon's debug icon, scoped to the chat body. */}
            <Tooltip label={debugMode ? t('panel.hideDebugTabs') : t('panel.showDebugTabs')} withArrow>
              <ActionIcon
                size="xs"
                variant={debugMode ? 'light' : 'subtle'}
                color={debugMode ? 'orange' : 'gray'}
                onClick={() => setDebugMode((v) => !v)}
                aria-label={t('panel.toggleDebugTabs')}
              >
                <IconBug size={12} />
              </ActionIcon>
            </Tooltip>
            {/* Stop — to the RIGHT of the debug button, with a gap so it is
                not hit by accident. Stops THIS chat's run (per-session). */}
            {sessionStatus === 'running' && (
              <>
                <div style={{ width: 16 }} />
                <Tooltip label={t('panel.stopChat')} withArrow>
                  <ActionIcon
                    size="xs"
                    variant="subtle"
                    color="red"
                    onClick={() => StopTask(sessionId ?? useAppStore.getState().runningSessionId ?? '')}
                    aria-label={t('panel.stop')}
                  >
                    <IconPlayerStop size={12} />
                  </ActionIcon>
                </Tooltip>
                <div style={{ width: 16 }} />
              </>
            )}
            {docs.length > 0 && (
              <Text size="xs" c="dimmed">{t('panel.docs', { count: docs.length })}</Text>
            )}
          </Group>
        </Tabs.List>
      </Tabs>

      {/* Tab content */}
      {activeTab === 'chat' && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <MessagePanel sessionId={sessionId} />
        </div>
      )}

      {activeTab === 'team' && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <TeamPanel />
        </div>
      )}

      {/* Transcript (debug) — the SAME tab the finished SessionViewer shows:
          one tool, two projections via the density toggle. Detailed = the
          LLM-level journal; Reader = the clean speaker→message view fed from
          the live message store. Replaces the old separate Journal +
          Conversation debug tabs. */}
      {activeTab === 'transcript' && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <Stack gap={0} style={{ height: '100%' }}>
            <Group
              gap="xs"
              px="md"
              py={6}
              style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}
            >
              <SegmentedControl
                size="xs"
                value={density}
                onChange={(v) => setDensity(v as 'reader' | 'detailed')}
                data={[
                  { label: t('transcript.detailed'), value: 'detailed' },
                  { label: t('transcript.reader'), value: 'reader' },
                ]}
              />
              <Text size="xs" c="dimmed">
                {density === 'detailed'
                  ? t('transcript.detailedHint')
                  : t('transcript.readerHint')}
              </Text>
            </Group>
            <Box style={{ flex: 1, minHeight: 0 }}>
              {density === 'reader' ? (
                <ConversationView transcript={liveTranscript} />
              ) : displayedUniverseName && sessionId ? (
                <SessionJournalPanel universeName={displayedUniverseName} sessionId={sessionId} />
              ) : (
                <div style={{ padding: 16 }}>
                  <Text size="sm" c="dimmed">{t('transcript.noSession')}</Text>
                </div>
              )}
            </Box>
          </Stack>
        </div>
      )}

      {/* HTTP (debug) — DevTools-Network-style inspector over the
          session's llm-model-calls.jsonl (status, durations, headers,
          bodies, SSE timeline per model call). */}
      {activeTab === 'http' && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          {displayedUniverseName && sessionId ? (
            <HttpPanel
              universeName={displayedUniverseName}
              sessionId={sessionId}
              live={!isTerminal}
            />
          ) : (
            <div style={{ padding: 16 }}>
              <Text size="sm" c="dimmed">
                {t('panel.httpNoSession')}
              </Text>
            </div>
          )}
        </div>
      )}

      {/* Cost (debug) — per-model token + USD breakdown for the running
          session. Same panel the cold session viewer uses. */}
      {activeTab === 'cost' && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          {displayedUniverseName && sessionId ? (
            <SessionCostPanel universeName={displayedUniverseName} sessionId={sessionId} />
          ) : (
            <div style={{ padding: 16 }}>
              <Text size="sm" c="dimmed">
                {t('panel.costNoSession')}
              </Text>
            </div>
          )}
        </div>
      )}

      {/* Output tab — schema-driven structured view of the task's
          `complete` payload. Mounts only after the task has terminated
          (showOutputTab); SessionOutputPanel resolves the latest
          session matching the task name (the session id isn't
          surfaced to the UI for top-level runs). */}
      {activeTab === 'output' && showOutputTab && (
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <SessionOutputPanel
            universeName={displayedUniverseName!}
            sessionId={sessionId ?? undefined}
            task={sessionName}
          />
        </div>
      )}

      {/* Floating bottom panel — Done (terminal state) or ChatInputPanel
          (live chat). Hoisted out of the Chat-only conditional so the
          Done button is reachable from any tab (Chat, Team, Output,
          docs); the chat input itself is only useful when the active
          tab is Chat, so we gate that branch on tab. The card sits on
          a backdrop-blurred row at the bottom of the panel via
          absolute positioning. */}
      {(isTerminal || activeTab === 'chat') && (
        <div
          style={{
            position: 'absolute',
            bottom: 24,
            left: 0,
            right: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            pointerEvents: 'none',
            // Above the scrolling message content so the input / Done card
            // is always clickable and the text never paints over it.
            zIndex: 5,
          }}
        >
          <div
            style={{
              width: '100%',
              // Frosted-fade strip: content scrolls UNDER it with a soft blur
              // (intentional aesthetic). Clickability of the Done/input card
              // is handled by the zIndex on the wrapper above, not by making
              // this opaque.
              background: 'color-mix(in srgb, var(--mantine-color-body) 10%, transparent)',
              backdropFilter: 'blur(2px)',
              padding: '3px 0 10px',
              display: 'flex',
              justifyContent: 'center',
              pointerEvents: 'auto',
            }}
          >
            <div
              style={{
                width: '100%',
                maxWidth: 800,
                background: 'var(--mantine-color-body)',
                border: '1px solid var(--mantine-color-default-border)',
                borderRadius: 12,
                boxShadow: '0 -4px 16px rgba(0, 0, 0, 0.15)',
              }}
            >
              {isTerminal
                ? <SessionDoneCard status={sessionStatus} reason={failureReason} onDismiss={dismissRunningTask} />
                : <ChatInputPanel sessionId={sessionId} />}
            </div>
          </div>
        </div>
      )}

      {/* Document tabs */}
      {activeTab.startsWith('doc:') && (() => {
        const docId = activeTab.slice(4)
        const doc = docs.find(d => d.id === docId)
        if (!doc) return null
        return (
          <div style={{ flex: 1, overflow: 'hidden' }}>
            <DocumentTab doc={doc} />
          </div>
        )
      })()}

      {/* Status bar pinned at very bottom */}
      <StatusBar sessionId={sessionId} />
    </div>
  )
}

/** Replaces the ChatInputPanel when the session has finished.
 *  User must click Done to dismiss the task and return to the node view. */
function SessionDoneCard({ status, reason, onDismiss }: { status: string; reason?: string; onDismiss: () => void }) {
  const t = useT('chat')
  const { icon, color, label } = (() => {
    if (status === 'done') return { icon: <IconCircleCheck size={20} />, color: 'teal', label: t('done.completed') }
    if (status === 'error') return { icon: <IconCircleX size={20} />, color: 'red', label: t('done.failed') }
    if (status === 'stopped') return { icon: <IconCircleMinus size={20} />, color: 'gray', label: t('done.stopped') }
    return { icon: <IconCircleCheck size={20} />, color: 'teal', label: t('done.finished') }
  })()

  return (
    <Group justify="space-between" align="center" gap="md" px="md" py="sm">
      <Group gap="xs" align="center" wrap="nowrap" style={{ minWidth: 0, flex: 1 }}>
        <Text c={color}>{icon}</Text>
        <Text fw={500} style={{ flexShrink: 0 }}>{label}</Text>
        {reason
          ? <Text size="sm" c="red" data-testid="run-failure-reason" style={{ minWidth: 0 }}>{reason}</Text>
          : <Text size="sm" c="dimmed">{t('done.scrollHint')}</Text>}
      </Group>
      <Button variant="filled" color={color} onClick={onDismiss}>
        {t('done.dismiss')}
      </Button>
    </Group>
  )
}
