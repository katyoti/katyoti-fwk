// SPDX-License-Identifier: Apache-2.0
/**
 * LiveSessionList — sessions tab body backed by real on-disk journal data.
 *
 * Calls `ListSessions(universeName)` on mount and whenever the displayed
 * universe changes. Each session is the journal of one ktw-agents run
 * (directory under `<DataDir>/sessions/`). Click → switches MainArea to the
 * read-only chat viewer (Resume button lives there; deferred wiring).
 *
 * The empty state (no universe displayed, no sessions, or load error) is
 * always shown — no spinner — because the call is local-filesystem and
 * completes in milliseconds.
 */

import { useEffect, useState } from 'react'
import { ActionIcon, Button, Group, NavLink, ScrollArea, Stack, Text, Tooltip } from '@mantine/core'
import {
  IconCircleCheck,
  IconCircleX,
  IconLoader2,
  IconMessageCircle,
  IconCircleDashed,
  IconTrash,
  IconCheck,
  IconX,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

// The wailsjs/go/main/App.d.ts is regenerated on each `wails build` — the
// just-added ListSessions binding lands there on the next build. Use the
// runtime-access pattern (same as IconRibbon does for PickWorkspaceFolder)
// so this file compiles before that regen happens.
interface SessionInfo {
  id:         string
  task:       string
  date:       string
  time:       string
  status?:    string
  turns?:     number
  tokens?:    number
  model?:     string
  durationMs?: number
}

interface ListSessionsResult {
  sessions?: SessionInfo[]
  error?:    string
}

interface DeleteSessionResult {
  error?: string
}

type GoBindings = {
  go?: {
    main?: {
      App?: {
        ListSessions?: (universeName: string) => Promise<ListSessionsResult>
        DeleteSession?: (universeName: string, sessionId: string) => Promise<DeleteSessionResult>
      }
    }
  }
}

function isFinished(status?: string): boolean {
  return status === 'completed' || status === 'failed' || status === 'error'
}

function statusIcon(status: string | undefined) {
  switch (status) {
    case 'completed': return <IconCircleCheck size={14} color="var(--mantine-color-green-6)" />
    case 'failed':
    case 'error':     return <IconCircleX     size={14} color="var(--mantine-color-red-6)" />
    case 'running':   return <IconLoader2     size={14} color="var(--mantine-color-yellow-6)" />
    default:          return <IconCircleDashed size={14} color="var(--mantine-color-dimmed)" />
  }
}

interface LiveSessionListProps {
  /** The currently displayed universe's slug, or null when none selected. */
  universeName: string | null
}

export function LiveSessionList({ universeName }: LiveSessionListProps) {
  const tw = useT('workspace')
  const t = useT('shell')
  const setMainPaneRoute    = useAppStore((s) => s.setMainPaneRoute)
  // Re-fetch the list whenever a new run starts (runningSessionId
  // flips from null to a fresh UUID) or terminates (sessionStatus
  // moves into a 'done'/'error'/'stopped' state). Without this the
  // sidebar shows a stale snapshot from the last universe-select and
  // never picks up sessions the user just created.
  const runningSessionId    = useAppStore((s) => s.runningSessionId)
  const sessionStatus       = useAppStore((s) => s.sessionStatus)
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [error, setError]   = useState<string | null>(null)
  // Two-step delete: first trash click arms confirm for that row; the check
  // commits, the x cancels. Avoids an accidental destructive single click.
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const deleteSession = async (id: string) => {
    const w = window as unknown as GoBindings
    const fn = w.go?.main?.App?.DeleteSession
    if (!fn || !universeName) return
    setBusy(true)
    try {
      const res = await fn(universeName, id)
      if (res?.error) {
        setError(res.error)
      } else {
        setSessions((prev) => prev.filter((s) => s.id !== id))
      }
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
      setConfirmId(null)
    }
  }

  const clearFinished = async () => {
    const w = window as unknown as GoBindings
    const fn = w.go?.main?.App?.DeleteSession
    if (!fn || !universeName) return
    setBusy(true)
    const removed: string[] = []
    try {
      for (const s of sessions.filter((x) => isFinished(x.status))) {
        const res = await fn(universeName, s.id)
        if (!res?.error) removed.push(s.id)
      }
      setSessions((prev) => prev.filter((s) => !removed.includes(s.id)))
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    if (!universeName) {
      setSessions([])
      setError(null)
      return
    }
    let cancelled = false
    const w = window as unknown as GoBindings
    const fn = w.go?.main?.App?.ListSessions
    if (!fn) {
      // Wails not wired (dev/mock) — empty list, no error noise.
      setSessions([])
      setError(null)
      return
    }
    fn(universeName)
      .then((res: ListSessionsResult) => {
        if (cancelled) return
        if (res.error) {
          setError(res.error)
          setSessions([])
          return
        }
        setSessions(res.sessions ?? [])
        setError(null)
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setError(String(e))
        setSessions([])
      })
    return () => { cancelled = true }
  }, [universeName, runningSessionId, sessionStatus])

  if (!universeName) {
    return (
      <Stack align="center" gap="xs" py="xl">
        <IconMessageCircle size={32} stroke={1.2} color="var(--mantine-color-dimmed)" />
        <Text size="xs" c="dimmed" ta="center">
          {t('liveSessions.pickUniverse')}
        </Text>
      </Stack>
    )
  }

  if (error) {
    return (
      <Stack align="center" gap="xs" py="xl" px="sm">
        <IconCircleX size={28} stroke={1.2} color="var(--mantine-color-red-6)" />
        <Text size="xs" c="red" ta="center">{error}</Text>
      </Stack>
    )
  }

  if (sessions.length === 0) {
    return (
      <Stack align="center" gap="xs" py="xl">
        <IconMessageCircle size={32} stroke={1.2} color="var(--mantine-color-dimmed)" />
        <Text size="xs" c="dimmed" ta="center">
          {t('liveSessions.empty')}
          <br />
          {t('liveSessions.emptyHint')}
        </Text>
      </Stack>
    )
  }

  const finishedCount = sessions.filter((s) => isFinished(s.status)).length

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      {finishedCount > 0 && (
        <Group justify="space-between" px="sm" py={4} style={{ flexShrink: 0 }}>
          <Text size="xs" c="dimmed">{t('liveSessions.count', { count: sessions.length })}</Text>
          <Tooltip label={t('liveSessions.clearHint')} withArrow>
            <Button
              size="compact-xs"
              variant="subtle"
              color="gray"
              leftSection={<IconTrash size={12} />}
              onClick={clearFinished}
              loading={busy}
            >
              {t('liveSessions.clear')}
            </Button>
          </Tooltip>
        </Group>
      )}
      <ScrollArea style={{ flex: 1 }} scrollbarSize={4}>
        <Stack gap={0} py={4}>
          {sessions.map((s) => (
            <NavLink
              key={s.id}
              label={s.task || s.id}
              description={[
                s.date && `${s.date} ${s.time ?? ''}`.trim(),
                s.model,
                s.turns ? t('sessionViewer.turns', { count: s.turns }) : null,
                s.durationMs ? `${Math.round(s.durationMs / 1000)}s` : null,
              ].filter(Boolean).join(' · ')}
              leftSection={statusIcon(s.status)}
              rightSection={
                confirmId === s.id ? (
                  <Group gap={2} wrap="nowrap">
                    <Tooltip label={t('liveSessions.confirmDelete')} withArrow>
                      <ActionIcon
                        size="sm"
                        variant="subtle"
                        color="red"
                        loading={busy}
                        onClick={(e) => { e.stopPropagation(); void deleteSession(s.id) }}
                        aria-label={t('liveSessions.confirmDeleteAria')}
                      >
                        <IconCheck size={13} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label={tw('common.cancel')} withArrow>
                      <ActionIcon
                        size="sm"
                        variant="subtle"
                        color="gray"
                        onClick={(e) => { e.stopPropagation(); setConfirmId(null) }}
                        aria-label={t('liveSessions.cancelDeleteAria')}
                      >
                        <IconX size={13} />
                      </ActionIcon>
                    </Tooltip>
                  </Group>
                ) : (
                  <Tooltip label={t('liveSessions.delete')} withArrow>
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="gray"
                      className="session-delete-btn"
                      onClick={(e) => { e.stopPropagation(); setConfirmId(s.id) }}
                      aria-label={t('liveSessions.delete')}
                    >
                      <IconTrash size={13} />
                    </ActionIcon>
                  </Tooltip>
                )
              }
              onClick={() => setMainPaneRoute({ kind: 'session', universeName, sessionId: s.id })}
              styles={{
                root: { padding: '6px 12px', borderRadius: 4, margin: '0 4px', cursor: 'pointer' },
                label: { fontSize: 13 },
                description: { fontSize: 10 },
              }}
            />
          ))}
        </Stack>
      </ScrollArea>
    </Stack>
  )
}
