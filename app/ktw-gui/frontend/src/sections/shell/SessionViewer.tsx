// SPDX-License-Identifier: Apache-2.0
/**
 * SessionViewer — read-only outer shell for a single session.
 *
 * Owns:
 *  - Header: back button, task name, status badge, Resume button, and a
 *    metadata strip (date/time, model, turns, duration, tokens).
 *  - Tabs: "Chat" (user-perspective transcript) | "Journal" (LLM-level
 *    events — turns, tool calls, system prompt). Same data root, two
 *    projections.
 *  - Resume modal: opens follow-up prompt → ResumeSession Wails call,
 *    routes back to the universe view on success.
 *
 * Pure rendering of either tab lives in:
 *  - [ConversationView](../conversation/ConversationView.tsx) — the shared speaker→message transcript (also used by the live chat's debug Conversation tab).
 *  - [SessionJournalPanel](./SessionJournalPanel.tsx) — the new
 *    `ktw-utils journal`-equivalent browser.
 */

import { useEffect, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Box,
  Button,
  CopyButton,
  Group,
  Modal,
  SegmentedControl,
  Stack,
  Tabs,
  Text,
  Textarea,
  Tooltip,
} from '@mantine/core'
import {
  IconArrowLeft,
  IconCheck,
  IconCircleX,
  IconCoin,
  IconCopy,
  IconFileText,
  IconList,
  IconUsers,
  IconLoader,
  IconPlayerPlay,
  IconWorld,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { useT } from '../../i18n'

import { ConversationView, findInitialPrompt } from '../conversation/ConversationView'
import type { TranscriptEntry } from '../conversation/ConversationView'
import { SessionJournalPanel } from './SessionJournalPanel'
import { SessionCostPanel } from './SessionCostPanel'
import { SessionTeamPanel } from './SessionTeamPanel'
import { SessionOutputPanel } from './SessionOutputPanel'
import { HttpPanel } from '../http/HttpPanel'

interface SessionDetail {
  id:          string
  task:        string
  date:        string
  time:        string
  status?:     string
  turns?:      number
  tokens?:     number
  model?:      string
  provider?:   string
  durationMs?: number
  transcript?: TranscriptEntry[]
}

interface GetSessionResult {
  detail?: SessionDetail
  error?:  string
}

interface ResumeSessionResult {
  runID?: string
  error?: string
}

type GoBindings = {
  go?: {
    main?: {
      App?: {
        GetSession?: (universeName: string, sessionId: string) => Promise<GetSessionResult>
        ResumeSession?: (
          universeName: string,
          sessionId: string,
          newInput: string,
        ) => Promise<ResumeSessionResult>
      }
    }
  }
}

interface Props {
  universeName: string
  sessionId:    string
}

type SessionTab = 'transcript' | 'cost' | 'team' | 'http' | 'output'
type TranscriptDensity = 'reader' | 'detailed'

export function SessionViewer({ universeName, sessionId }: Props) {
  const t = useT('shell')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const setActiveTab     = useTabsStore((s) => s.setActive)
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const startSession     = useAppStore((s) => s.startSession)

  const [detail, setDetail] = useState<SessionDetail | null>(null)
  const [error, setError]   = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<SessionTab>('transcript')
  // Cold sessions default to the Detailed (Journal) view — the inspection
  // surface — with Reader (clean transcript) as a toggle. Same data root.
  const [density, setDensity] = useState<TranscriptDensity>('detailed')

  // Resume modal state — opens when the user clicks the Resume button.
  const [resumeOpen, setResumeOpen] = useState(false)
  const [resumeInput, setResumeInput] = useState('')
  const [resuming, setResuming] = useState(false)
  const [resumeError, setResumeError] = useState<string | null>(null)

  // A session still running is followed live: read again every 2 s until it
  // ends — a run started elsewhere (the planner, the scheduler, an agent) too.
  const [refreshKey, setRefreshKey] = useState(0)
  const live = sessionIsLive(detail?.status)
  useEffect(() => {
    if (!live) return
    const timer = window.setInterval(() => setRefreshKey((k) => k + 1), LIVE_REFRESH_MS)
    return () => window.clearInterval(timer)
  }, [live])
  useEffect(() => { setRefreshKey(0) }, [universeName, sessionId])

  useEffect(() => {
    let cancelled = false
    if (refreshKey === 0) setLoading(true)
    const w = window as unknown as GoBindings
    const fn = w.go?.main?.App?.GetSession
    if (!fn) {
      setError('Wails binding GetSession not available')
      setLoading(false)
      return
    }
    fn(universeName, sessionId)
      .then((res) => {
        if (cancelled) return
        if (res.error) {
          setError(res.error)
          setDetail(null)
        } else {
          setDetail(res.detail ?? null)
          setError(null)
        }
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setError(String(e))
        setDetail(null)
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [universeName, sessionId, refreshKey])

  // Back to where the runs and the sessions are listed: Global → Scheduler.
  const goBack = () => {
    const home = useTabsStore.getState().getForUniverse(universeName).openTabs.find((t) => t.id === 'home')
    updateTabPayload(universeName, 'home', { ...((home?.payload ?? {}) as object), activeKind: 'runs' })
    setActiveTab(universeName, 'home')
    setMainPaneRoute({ kind: 'universe' })
  }

  const handleResume = async () => {
    if (resuming) return
    setResuming(true)
    setResumeError(null)
    try {
      const w = window as unknown as GoBindings
      const fn = w.go?.main?.App?.ResumeSession
      if (!fn) {
        setResumeError('Wails binding ResumeSession not available (rebuild ktw-gui)')
        setResuming(false)
        return
      }
      const res = await fn(universeName, sessionId, resumeInput)
      if (res?.error) {
        setResumeError(res.error)
        setResuming(false)
        return
      }
      startSession(detail?.task ?? sessionId)
      setResumeOpen(false)
      setResuming(false)
      setResumeInput('')
      setMainPaneRoute({ kind: 'universe' })
    } catch (e) {
      setResumeError(String(e))
      setResuming(false)
    }
  }

  if (loading) {
    return (
      <Stack align="center" gap="xs" py="xl">
        <IconLoader size={28} stroke={1.2} color="var(--mantine-color-dimmed)" />
        <Text size="sm" c="dimmed">{t('sessionViewer.loading')}</Text>
      </Stack>
    )
  }
  if (error) {
    return (
      <Stack gap="md" p="md">
        <Group gap="xs">
          <ActionIcon variant="subtle" size="sm" onClick={goBack} aria-label={t('sessionViewer.backToSessions')}>
            <IconArrowLeft size={16} />
          </ActionIcon>
          <Text size="sm" c="dimmed">{t('sessionViewer.sessions')}</Text>
        </Group>
        <Stack align="center" gap="xs" py="xl">
          <IconCircleX size={28} stroke={1.2} color="var(--mantine-color-red-6)" />
          <Text size="sm" c="red" ta="center">{error}</Text>
        </Stack>
      </Stack>
    )
  }
  if (!detail) return null

  const initialPrompt = findInitialPrompt(detail.transcript ?? [])
  const finishedLabel = computeFinished(detail.date, detail.time, detail.durationMs)

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      {/* Header */}
      <Stack gap={4} px="md" py="sm" style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}>
        <Group gap="xs" wrap="nowrap">
          <Tooltip label={t('sessionViewer.backToSessions')} withArrow>
            <ActionIcon variant="subtle" size="sm" onClick={goBack} aria-label={t('sessionViewer.back')}>
              <IconArrowLeft size={16} />
            </ActionIcon>
          </Tooltip>
          <Text fw={600} truncate style={{ flex: 1 }}>{detail.task || detail.id}</Text>
          {detail.status && (
            <Badge size="sm" variant="light" color={statusColor(detail.status)}>{detail.status}</Badge>
          )}
          <Tooltip label={t('sessionViewer.resumeHint')} withArrow>
            <Button
              size="xs"
              variant="light"
              leftSection={<IconPlayerPlay size={12} />}
              onClick={() => {
                setResumeError(null)
                setResumeInput('')
                setResumeOpen(true)
              }}
            >
              {t('sessionViewer.resume')}
            </Button>
          </Tooltip>
        </Group>
        <Group gap="md">
          <Group gap={4} wrap="nowrap">
            <Text size="xs" c="dimmed" ff="monospace" truncate style={{ maxWidth: 300 }} title={detail.id}>
              {detail.id}
            </Text>
            <CopyButton value={detail.id} timeout={1500}>
              {({ copied, copy }) => (
                <Tooltip label={copied ? t('sessionViewer.copied') : t('sessionViewer.copyId')} withArrow>
                  <ActionIcon size="xs" variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label={t('sessionViewer.copyId')}>
                    {copied ? <IconCheck size={12} /> : <IconCopy size={12} />}
                  </ActionIcon>
                </Tooltip>
              )}
            </CopyButton>
          </Group>
          <Text size="xs" c="dimmed">{t('sessionViewer.started', { date: detail.date, time: detail.time })}</Text>
          {finishedLabel && <Text size="xs" c="dimmed">{t('sessionViewer.finished', { when: finishedLabel })}</Text>}
          {detail.model && <Text size="xs" c="dimmed">{detail.model}</Text>}
          {detail.turns ? <Text size="xs" c="dimmed">{t('sessionViewer.turns', { count: detail.turns })}</Text> : null}
          {detail.durationMs ? <Text size="xs" c="dimmed">{Math.round(detail.durationMs / 1000)}s</Text> : null}
          {detail.tokens ? <Text size="xs" c="dimmed">{t('sessionViewer.tokens', { tokens: detail.tokens })}</Text> : null}
        </Group>
      </Stack>

      {/* Tabs: Chat / Journal */}
      <Tabs
        value={tab}
        onChange={(v) => v && setTab(v as SessionTab)}
        variant="outline"
        styles={{ tab: { fontSize: 13 } }}
        style={{ flexShrink: 0 }}
      >
        <Tabs.List>
          <Tabs.Tab value="transcript" leftSection={<IconList size={14} />}>
            {t('sessionViewer.tabs.transcript')}
          </Tabs.Tab>
          <Tabs.Tab value="cost" leftSection={<IconCoin size={14} />}>
            {t('sessionViewer.tabs.cost')}
          </Tabs.Tab>
          <Tabs.Tab value="team" leftSection={<IconUsers size={14} />}>
            {t('sessionViewer.tabs.teams')}
          </Tabs.Tab>
          <Tabs.Tab value="http" leftSection={<IconWorld size={14} />}>
            {t('sessionViewer.tabs.http')}
          </Tabs.Tab>
          <Tabs.Tab value="output" leftSection={<IconFileText size={14} />}>
            {t('sessionViewer.tabs.output')}
          </Tabs.Tab>
        </Tabs.List>
      </Tabs>

      <Box style={{ flex: 1, minHeight: 0 }}>
        {tab === 'transcript' && (
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
                onChange={(v) => setDensity(v as TranscriptDensity)}
                data={[
                  { label: t('sessionViewer.density.detailed'), value: 'detailed' },
                  { label: t('sessionViewer.density.reader'), value: 'reader' },
                ]}
              />
              <Text size="xs" c="dimmed">
                {density === 'detailed'
                  ? t('sessionViewer.density.detailedHint')
                  : t('sessionViewer.density.readerHint')}
              </Text>
            </Group>
            <Box style={{ flex: 1, minHeight: 0 }}>
              {density === 'reader' ? (
                <ConversationView
                  transcript={detail.transcript ?? []}
                  initialPrompt={initialPrompt}
                />
              ) : (
                <SessionJournalPanel universeName={universeName} sessionId={sessionId} refreshKey={refreshKey} />
              )}
            </Box>
          </Stack>
        )}
        {tab === 'cost' && (
          <SessionCostPanel universeName={universeName} sessionId={sessionId} />
        )}
        {tab === 'team' && (
          <SessionTeamPanel universeName={universeName} sessionId={sessionId} />
        )}
        {tab === 'http' && (
          <HttpPanel universeName={universeName} sessionId={sessionId} />
        )}
        {tab === 'output' && (
          <SessionOutputPanel
            universeName={universeName}
            sessionId={sessionId}
            task={detail.task}
          />
        )}
      </Box>

      {/* Resume modal */}
      <Modal
        opened={resumeOpen}
        onClose={() => { if (!resuming) setResumeOpen(false) }}
        title={<Text fw={600}>{t('sessionViewer.resumeTitle')}</Text>}
        size="md"
        centered
      >
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            {t('sessionViewer.resumeBody')}
          </Text>
          <Textarea
            value={resumeInput}
            onChange={(e) => setResumeInput(e.currentTarget.value)}
            placeholder={t('sessionViewer.resumePlaceholder')}
            autosize
            minRows={3}
            maxRows={10}
            disabled={resuming}
            styles={{ input: { fontFamily: 'monospace', fontSize: 13 } }}
          />
          {resumeError && (
            <Text size="xs" c="red">{resumeError}</Text>
          )}
          <Group justify="flex-end" gap="sm">
            <Button variant="subtle" color="gray" onClick={() => setResumeOpen(false)} disabled={resuming}>
              {t('sessionViewer.cancel')}
            </Button>
            <Button
              leftSection={<IconPlayerPlay size={14} />}
              onClick={() => void handleResume()}
              loading={resuming}
            >
              {t('sessionViewer.resume')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}

/** Finished wall-clock = started (parsed from the session's local date+time,
 *  derived from the session-id UUIDv7) + durationMs. Returns just the time
 *  when it lands on the start date, else date+time. Empty when not computable. */
function computeFinished(date?: string, time?: string, durationMs?: number): string {
  if (!date || !time || !durationMs) return ''
  const start = new Date(`${date}T${time}`)
  if (isNaN(start.getTime())) return ''
  const end = new Date(start.getTime() + durationMs)
  const pad = (n: number) => String(n).padStart(2, '0')
  const t = `${pad(end.getHours())}:${pad(end.getMinutes())}:${pad(end.getSeconds())}`
  const sameDay =
    end.getFullYear() === start.getFullYear() &&
    end.getMonth() === start.getMonth() &&
    end.getDate() === start.getDate()
  if (sameDay) return t
  return `${end.getFullYear()}-${pad(end.getMonth() + 1)}-${pad(end.getDate())} ${t}`
}

/** How often a running session is read again. */
const LIVE_REFRESH_MS = 2000

/** A session still running is followed live; an ended one is read once. */
export function sessionIsLive(status: string | undefined): boolean {
  return status === 'running'
}

function statusColor(status: string): string {
  switch (status) {
    case 'completed': return 'green'
    case 'failed':
    case 'error':     return 'red'
    case 'running':   return 'yellow'
    default:          return 'gray'
  }
}
