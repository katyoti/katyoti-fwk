// SPDX-License-Identifier: Apache-2.0
/**
 * RunningSessionList — the LIVE running-sessions view, backed by the
 * session manager service (the core's Room) rather than on-disk journals.
 *
 * Calls `ListRunningSessions(universeName)` (Wails → Terminal/list-sessions
 * → Room.ListSessions) and renders every session the core currently holds
 * with status `running`. Refreshes:
 *   - on mount + universe change,
 *   - immediately on each `session.created` Wails event (sessions THIS GUI
 *     starts appear instantly),
 *   - on a short poll, so sessions started ELSEWHERE (an external
 *     `ktw-agents run`, e.g. a `test-scenario` run) surface too — those do
 *     not emit this GUI's own `session.created` but DO land in the Room.
 *
 * Distinct from LiveSessionList (the historical, disk-based list). This one
 * is the "what is executing right now" surface the user asked for.
 */

import { useCallback, useEffect, useState } from 'react'
import { NavLink, ScrollArea, Stack, Text, Group, Badge } from '@mantine/core'
import { IconLoader2, IconActivity } from '@tabler/icons-react'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { useT } from '../../i18n'

// Runtime-access types — the wailsjs/go/main/App.d.ts binding for
// ListRunningSessions is regenerated on the next `wails build`; using the
// window.go runtime pattern (as LiveSessionList does) lets this compile now.
interface RunningSession {
  id:              string
  parentSessionId?: string
  name:            string
  status:          string
  taskId:          string
  parentTaskId?:   string
  messageCount:    number
  startedAt:       string
}

interface ListRunningSessionsResult {
  sessions?: RunningSession[]
  error?:    string
}

type GoBindings = {
  go?: { main?: { App?: {
    ListRunningSessions?: (universeName: string) => Promise<ListRunningSessionsResult>
  } } }
}

const POLL_MS = 10000

interface RunningSessionListProps {
  universeName: string | null
}

export function RunningSessionList({ universeName }: RunningSessionListProps) {
  const t = useT('shell')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const [sessions, setSessions] = useState<RunningSession[]>([])
  const [error, setError] = useState<string | null>(null)

  // A running session's LIVE surface is the chat tab — NOT the disk
  // session viewer (the Room session id, e.g. `sess-ff8e3295`, is not the
  // on-disk journal directory, so {kind:'session'} 404s). Focus the chat
  // tab instead. Per-session chat binding (one tab per session) is P3/P4;
  // v1 focuses the single live chat surface.
  const focusChat = useCallback((s: RunningSession) => {
    if (!universeName) return
    const tabId = `chat:${s.id}`
    setMainPaneRoute({ kind: 'universe' })
    // Focus this session's bucket (it has been accumulating live even while
    // unfocused) + open/focus its own chat tab.
    useAppStore.getState().ensureSession(s.id, s.name)
    useAppStore.getState().focusSession(s.id)
    useTabsStore.getState().openTab(universeName, {
      id:        tabId,
      kind:      'chat',
      label:     s.name === 'chat' ? 'Chat' : s.name,
      closeable: true,
      payload:   { taskName: s.name, sessionId: s.id },
    })
    useTabsStore.getState().setActive(universeName, tabId)
  }, [universeName, setMainPaneRoute])

  const fetchSessions = useCallback(async () => {
    if (!universeName) {
      setSessions([])
      setError(null)
      return
    }
    const fn = (window as unknown as GoBindings).go?.main?.App?.ListRunningSessions
    if (!fn) {
      setSessions([])
      setError(null)
      return
    }
    try {
      const res = await fn(universeName)
      if (res.error) {
        setError(res.error)
        setSessions([])
        return
      }
      // Only the sessions actually executing belong in this view; the
      // historical list (LiveSessionList) covers finished ones.
      setSessions((res.sessions ?? []).filter((s) => s.status === 'running'))
      setError(null)
    } catch (e: unknown) {
      setError(String(e))
      setSessions([])
    }
  }, [universeName])

  useEffect(() => {
    void fetchSessions()
    // session.created/ended events handle self-started runs instantly; the
    // poll is only a backstop for externally-started sessions + status
    // transitions. Keep it slow, and pause it while the window is hidden so
    // it isn't burning an IPC round-trip every few seconds in the background.
    const offCreated = EventsOn('session.created', () => { void fetchSessions() })
    const offEnded   = EventsOn('session.ended',  () => { void fetchSessions() })
    const timer = window.setInterval(() => {
      if (!document.hidden) void fetchSessions()
    }, POLL_MS)
    // Refresh once immediately when the window regains focus (covers anything
    // that changed while the poll was paused).
    const onVisible = () => { if (!document.hidden) void fetchSessions() }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      offCreated()
      offEnded()
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [fetchSessions])

  if (!universeName || (sessions.length === 0 && !error)) {
    // Stay quiet when nothing is running — the historical list below carries
    // the empty-state messaging.
    return null
  }

  return (
    <Stack gap={2} py={4}>
      <Group gap={6} px={12} py={2} align="center">
        <IconActivity size={13} color="var(--mantine-color-yellow-6)" />
        <Text size="xs" fw={600} c="dimmed" tt="uppercase">
          {t('runningSessions.running')}
        </Text>
        <Badge size="xs" variant="light" color="yellow">{sessions.length}</Badge>
      </Group>
      {error ? (
        <Text size="xs" c="red" px={12} py={4}>{error}</Text>
      ) : (
        <ScrollArea.Autosize mah={220} scrollbarSize={4}>
          <Stack gap={0}>
            {sessions.map((s) => (
              <NavLink
                key={s.id}
                label={s.name || s.id}
                description={[
                  s.parentTaskId ? 'child' : 'root',
                  s.messageCount ? `${s.messageCount} msg${s.messageCount === 1 ? '' : 's'}` : null,
                ].filter(Boolean).join(' · ')}
                leftSection={<IconLoader2 size={14} color="var(--mantine-color-yellow-6)" />}
                onClick={() => focusChat(s)}
                styles={{
                  root: { padding: '6px 12px', borderRadius: 4, margin: '0 4px', cursor: 'pointer' },
                  label: { fontSize: 13 },
                  description: { fontSize: 10 },
                }}
              />
            ))}
          </Stack>
        </ScrollArea.Autosize>
      )}
    </Stack>
  )
}
