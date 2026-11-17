// SPDX-License-Identifier: Apache-2.0
/**
 * EdgeDetailPanel — side panel for one LINK (delegation edge) in the task tree.
 *
 * A delegation edge carries data in both directions: the parent binds an
 * INPUT down into the child when it spawns it, and the child returns an
 * OUTPUT up when it finishes. Clicking the link shows exactly that payload,
 * plus the delegation mode the edge is labeled with. Live cards often lack
 * input/output (they only carry the outcome), so we enrich from the session
 * journal via GetSessionTaskTree — same source of truth as TaskDetailPanel.
 */
import { useEffect, useState } from 'react'
import { ActionIcon, Badge, Code, Group, Stack, Text, Tooltip } from '@mantine/core'
import { IconX, IconArrowDown, IconArrowUp, IconMessage } from '@tabler/icons-react'
import { type RunScope, type ColdTaskTree, useTeamStore } from '../../store/teamSlice'
import { useAppStore } from '../../store'
import { commsLabel } from './useTeamLiveSource'
import { GetSessionTaskTree } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

interface Props {
  scope: RunScope
  edgeKey: string  // `${parentRunId}->${childRunId}` (delegation) or `comm:${fromRun}->${toRun}`
  onClose: () => void
}

export function EdgeDetailPanel({ scope, edgeKey, onClose }: Props) {
  // A comm link (lead-to-lead channel) carries MESSAGES, not a spawn payload —
  // render those instead of input/output.
  if (edgeKey.startsWith('comm:')) {
    return <CommEdgeDetail scope={scope} edgeKey={edgeKey.slice('comm:'.length)} onClose={onClose} />
  }
  return <DelegationEdgeDetail scope={scope} edgeKey={edgeKey} onClose={onClose} />
}

function DelegationEdgeDetail({ scope, edgeKey, onClose }: Props) {
  const t = useT('team')
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const [arrow, child] = edgeKey.split('->')
  const parentRunId = arrow
  const childRunId = child
  const [cold, setCold] = useState<ColdTaskTree | null>(null)

  const sessionId = scope.sessionId || useAppStore.getState().runningSessionId || ''

  useEffect(() => {
    let cancelled = false
    setCold(null)
    if (!displayedUniverseName || !sessionId) return
    GetSessionTaskTree(displayedUniverseName, sessionId)
      .then((res) => { if (!cancelled && res?.tree) setCold(res.tree as unknown as ColdTaskTree) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [displayedUniverseName, sessionId, edgeKey])

  const parent = scope.children.get(parentRunId)
  const childInfo = scope.children.get(childRunId)
  // The root (coordinator) is not in the children map — fall back to the
  // workflow name when the parent end of the link is the root run.
  const parentName = parent?.name || (parentRunId === scope.workflow?.runId ? (scope.workflow?.name || t('edgeDetail.coordinator')) : parentRunId)
  const childName = childInfo?.name || childRunId

  const coldTask = cold?.tasks?.find((task) => task.runId === childRunId || task.name === childInfo?.name)
  const input = childInfo?.input ?? coldTask?.input
  const output = childInfo?.output ?? coldTask?.output
  const mode = childInfo?.mode || coldTask?.mode || 'worker'
  const comms = childInfo?.commsMode || coldTask?.commsMode || ''

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group
        justify="space-between"
        px="sm"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Group gap={6} wrap="nowrap">
          <Text size="sm" fw={600} style={{ whiteSpace: 'nowrap' }}>{parentName}</Text>
          <Text size="sm" c="dimmed">→</Text>
          <Text size="sm" fw={600} style={{ whiteSpace: 'nowrap' }}>{childName}</Text>
          {comms && (
            <Tooltip label={t('edgeDetail.commsTip', { comms })} position="bottom" withArrow>
              <Badge size="xs" color="teal" variant="light">{commsLabel(comms)}</Badge>
            </Tooltip>
          )}
          <Tooltip label={t('edgeDetail.modeTip', { mode })} position="bottom" withArrow>
            <Badge size="xs" color="grape" variant="light">{mode}</Badge>
          </Tooltip>
        </Group>
        <ActionIcon size="sm" variant="subtle" color="gray" onClick={onClose}>
          <IconX size={14} />
        </ActionIcon>
      </Group>

      <Stack gap="sm" p="sm" style={{ flex: 1, overflowY: 'auto' }}>
        <Text size="xs" c="dimmed">
          {t('edgeDetail.linkIntro')}
        </Text>

        <Field label={t('edgeDetail.sentDown')} icon={<IconArrowDown size={12} />}>
          {input && Object.keys(input).length > 0 ? (
            <JsonBlock value={input} />
          ) : (
            <Text size="xs" c="dimmed">{cold ? t('edgeDetail.noInput') : t('common.loadingJournal')}</Text>
          )}
        </Field>

        <Field label={t('edgeDetail.returnedUp')} icon={<IconArrowUp size={12} />}>
          {childInfo?.error ? (
            <Text size="xs" c="red.4" style={{ whiteSpace: 'pre-wrap' }}>{childInfo.error}</Text>
          ) : output != null ? (
            <JsonBlock value={output} />
          ) : (
            <Text size="xs" c="dimmed">{childInfo?.state === 'running' ? t('common.stillRunning') : t('common.noOutput')}</Text>
          )}
        </Field>
      </Stack>
    </Stack>
  )
}

function CommEdgeDetail({ scope, edgeKey, onClose }: Props) {
  const t = useT('team')
  const [fromRun, toRun] = edgeKey.split('->')
  const fromName = scope.children.get(fromRun)?.name || fromRun
  const toName = scope.children.get(toRun)?.name || toRun
  // Messages land in the recipient's mailbox; keep only those from this sender.
  const messages = (scope.mailboxes.get(toName) ?? []).filter((m) => m.from === fromName)

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group
        justify="space-between"
        px="sm"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Group gap={6} wrap="nowrap">
          <IconMessage size={14} color="var(--mantine-color-grape-4)" />
          <Text size="sm" fw={600} style={{ whiteSpace: 'nowrap' }}>{fromName}</Text>
          <Text size="sm" c="dimmed">→</Text>
          <Text size="sm" fw={600} style={{ whiteSpace: 'nowrap' }}>{toName}</Text>
          <Badge size="xs" color="grape" variant="light">{t('edgeDetail.channel')}</Badge>
        </Group>
        <ActionIcon size="sm" variant="subtle" color="gray" onClick={onClose}>
          <IconX size={14} />
        </ActionIcon>
      </Group>

      <Stack gap="sm" p="sm" style={{ flex: 1, overflowY: 'auto' }}>
        <Text size="xs" c="dimmed">
          {t('edgeDetail.channelIntro', { count: messages.length, from: fromName, to: toName })}
        </Text>
        {messages.length === 0 ? (
          <Text size="xs" c="dimmed">{t('edgeDetail.noMessages')}</Text>
        ) : (
          messages.map((m, i) => (
            <Stack key={i} gap={2} style={{ borderLeft: '2px solid var(--mantine-color-grape-6)', paddingLeft: 8 }}>
              {m.summary && <Text size="xs" c="grape.3" fw={600}>{m.summary}</Text>}
              <Text size="xs" style={{ whiteSpace: 'pre-wrap' }}>{m.message}</Text>
            </Stack>
          ))
        )}
      </Stack>
    </Stack>
  )
}

function Field({ label, icon, children }: { label: string; icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <Stack gap={2}>
      <Group gap={4}>
        {icon}
        <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ fontSize: 10, letterSpacing: 0.5 }}>{label}</Text>
      </Group>
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
    <Code block style={{ fontSize: 11, maxHeight: 260, overflowY: 'auto', whiteSpace: 'pre-wrap' }}>
      {text}
    </Code>
  )
}
