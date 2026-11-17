// SPDX-License-Identifier: Apache-2.0
/**
 * TaskDetailPanel — side panel for one task run in the session task tree.
 *
 * The six card fields in full: who started it (parent + delegation mode),
 * input, run ID (the journal drill key), output / error, status, and the
 * session resource summary (model / tokens / duration).
 *
 * Input/output not present in live events (live cards only carry outcome
 * payloads) are lazily enriched from the session journal via
 * GetSessionTaskTree — the journal is the source of truth for "what ran
 * with which input". The Journal button opens the full session journal.
 */
import { useEffect, useState } from 'react'
import { ActionIcon, Badge, Button, Code, CopyButton, Group, Modal, Stack, Text, Tooltip } from '@mantine/core'
import { IconX, IconCopy, IconCheck, IconList } from '@tabler/icons-react'
import { type RunScope, type ChildInfo, type ColdTaskTree, useTeamStore } from '../../store/teamSlice'
import { useAppStore } from '../../store'
import { GetSessionTaskTree } from '../../../wailsjs/go/main/App'
import { SessionJournalPanel } from '../shell/SessionJournalPanel'
import { useT } from '../../i18n'

interface Props {
  scope: RunScope
  runId: string
  onClose: () => void
}

const STATE_COLOR: Record<string, string> = {
  running: 'blue',
  done: 'green',
  failed: 'red',
}

export function TaskDetailPanel({ scope, runId, onClose }: Props) {
  const t = useT('team')
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const child = scope.children.get(runId)
  const [cold, setCold] = useState<ColdTaskTree | null>(null)
  const [journalOpen, setJournalOpen] = useState(false)

  const sessionId = scope.sessionId || useAppStore.getState().runningSessionId || ''

  // Lazy enrichment: fetch the cold projection once per panel open when the
  // live card is missing input/output (or for the resource summary).
  useEffect(() => {
    let cancelled = false
    setCold(null)
    if (!displayedUniverseName || !sessionId) return
    GetSessionTaskTree(displayedUniverseName, sessionId)
      .then((res) => {
        if (!cancelled && res?.tree) setCold(res.tree as unknown as ColdTaskTree)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [displayedUniverseName, sessionId, runId])

  if (!child) return null

  // Merge: live values win; the journal projection fills the gaps.
  const coldTask = cold?.tasks?.find((task) => task.runId === runId || task.name === child.name)
  const input = child.input ?? coldTask?.input
  const output = child.output ?? coldTask?.output
  const mode = child.mode || coldTask?.mode || ''
  const parent = child.parentRunId ? scope.children.get(child.parentRunId) : undefined
  const initiator = parent ? parent.name || parent.runId : child.parentRunId || t('taskDetail.userSession')
  const color = STATE_COLOR[child.state] ?? 'gray'

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group
        justify="space-between"
        px="sm"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Group gap={6}>
          <Text size="sm" fw={600}>{child.name || t('taskDetail.task')}</Text>
          <Badge size="xs" color={color} variant="light">{t(`common.state.${child.state}`, { defaultValue: child.state })}</Badge>
        </Group>
        <ActionIcon size="sm" variant="subtle" color="gray" onClick={onClose}>
          <IconX size={14} />
        </ActionIcon>
      </Group>

      <Stack gap="sm" p="sm" style={{ flex: 1, overflowY: 'auto' }}>
        {/* Who started it */}
        <Field label={t('taskDetail.startedBy')}>
          <Group gap={6}>
            <Text size="xs">{initiator}</Text>
            {mode && <Badge size="xs" color="grape" variant="light">{mode}</Badge>}
            {child.kind && <Badge size="xs" color="gray" variant="outline">{child.kind}</Badge>}
          </Group>
        </Field>

        {/* Run ID — the journal drill key */}
        <Field label={t('taskDetail.runId')}>
          <Group gap={4} wrap="nowrap">
            <Code style={{ fontSize: 11, wordBreak: 'break-all' }}>{child.runId}</Code>
            <CopyButton value={child.runId}>
              {({ copied, copy }) => (
                <Tooltip label={copied ? t('taskDetail.copied') : t('taskDetail.copyRunId')} withArrow>
                  <ActionIcon size="xs" variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy}>
                    {copied ? <IconCheck size={12} /> : <IconCopy size={12} />}
                  </ActionIcon>
                </Tooltip>
              )}
            </CopyButton>
          </Group>
        </Field>

        {/* Input */}
        <Field label={t('taskDetail.input')}>
          {input && Object.keys(input).length > 0 ? (
            <JsonBlock value={input} />
          ) : (
            <Text size="xs" c="dimmed">{cold ? t('taskDetail.noInput') : t('common.loadingJournal')}</Text>
          )}
        </Field>

        {/* Output / error */}
        <Field label={child.error ? t('taskDetail.error') : t('taskDetail.output')}>
          {child.error ? (
            <Text size="xs" c="red.4" style={{ whiteSpace: 'pre-wrap' }}>{child.error}</Text>
          ) : output != null ? (
            <JsonBlock value={output} />
          ) : (
            <Text size="xs" c="dimmed">{child.state === 'running' ? t('common.stillRunning') : t('common.noOutput')}</Text>
          )}
        </Field>

        {/* Resources — session-level summary (per-child rollup is a later
            iteration; child sessions are joinable via the run ID). */}
        <Field label={t('taskDetail.resources')}>
          {cold ? (
            <Group gap={6}>
              {cold.model && <Badge size="xs" color="blue" variant="light">{cold.model}</Badge>}
              {!!cold.totalTokens && <Text size="xs" c="dimmed">{t('taskDetail.tokens', { count: cold.totalTokens, tokens: cold.totalTokens.toLocaleString() })}</Text>}
              {!!cold.durationMs && <Text size="xs" c="dimmed">{formatMs(cold.durationMs)}</Text>}
              {!cold.model && !cold.totalTokens && <Text size="xs" c="dimmed">{t('taskDetail.sessionRunning')}</Text>}
            </Group>
          ) : (
            <Text size="xs" c="dimmed">…</Text>
          )}
        </Field>

        {/* Journal drill */}
        {displayedUniverseName && sessionId && (
          <Button
            size="xs"
            variant="light"
            leftSection={<IconList size={13} />}
            onClick={() => setJournalOpen(true)}
          >
            {t('taskDetail.openJournal')}
          </Button>
        )}
      </Stack>

      <Modal
        opened={journalOpen}
        onClose={() => setJournalOpen(false)}
        title={t('taskDetail.journalTitle', { sessionId })}
        size="xl"
      >
        {journalOpen && displayedUniverseName && sessionId && (
          <div style={{ height: '70vh', overflow: 'hidden' }}>
            <SessionJournalPanel universeName={displayedUniverseName} sessionId={sessionId} />
          </div>
        )}
      </Modal>
    </Stack>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Stack gap={2}>
      <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ fontSize: 10, letterSpacing: 0.5 }}>{label}</Text>
      {children}
    </Stack>
  )
}

function JsonBlock({ value }: { value: unknown }) {
  let text: string
  try {
    text = typeof value === 'string' ? value : JSON.stringify(value, null, 2)
  } catch {
    text = String(value)
  }
  return (
    <Code block style={{ fontSize: 11, maxHeight: 220, overflowY: 'auto', whiteSpace: 'pre-wrap' }}>
      {text}
    </Code>
  )
}

function formatMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

/** Hook used by TeamPanel: the currently selected task (if any). */
export function useSelectedTask(): { scope: RunScope | undefined; child: ChildInfo | undefined } {
  const scope = useTeamStore((s) => {
    const id = s.viewRootRunId ?? s.activeRootRunId
    return id ? s.runs.get(id) : undefined
  })
  const selectedTaskRunId = useTeamStore((s) => s.selectedTaskRunId)
  return {
    scope,
    child: scope && selectedTaskRunId ? scope.children.get(selectedTaskRunId) : undefined,
  }
}
