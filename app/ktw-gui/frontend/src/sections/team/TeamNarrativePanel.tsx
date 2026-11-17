// SPDX-License-Identifier: Apache-2.0
/**
 * TeamNarrativePanel — the TEAM lens: renders the displayed run's team from
 * its team.* narrative events (created / member-joined / message /
 * state-changed / item-shared / completed), NOT from the task tree.
 *
 * Top: team header (name + FSM state) and member chips with live states
 * (working / done / left). Body: the chronological narrative. Clicking a
 * member opens their mailbox via the existing selectTeammate flow.
 */
import { Badge, Group, Paper, Stack, Text, Timeline } from '@mantine/core'
import {
  IconUsersGroup, IconUserPlus, IconUserMinus, IconMessage, IconShare,
  IconFlag, IconCircleCheck, IconCircleX, IconActivity,
} from '@tabler/icons-react'
import { useTeamStore, selectViewScope, selectTeamView, type TeamEventEntry } from '../../store/teamSlice'
import { useT } from '../../i18n'

type T = ReturnType<typeof useT>

const TEAM_STATE_COLOR: Record<string, string> = {
  forming: 'yellow',
  active: 'blue',
  done: 'green',
  failed: 'red',
}

const MEMBER_STATE_COLOR: Record<string, string> = {
  working: 'blue',
  done: 'green',
  left: 'gray',
}

const ACTION_ICON: Record<string, typeof IconUsersGroup> = {
  'created': IconUsersGroup,
  'member-joined': IconUserPlus,
  'member-left': IconUserMinus,
  'member-state-changed': IconActivity,
  'message': IconMessage,
  'item-shared': IconShare,
  'state-changed': IconFlag,
  'completed': IconCircleCheck,
  'failed': IconCircleX,
}

export function TeamNarrativePanel() {
  const t = useT('team')
  const scope = useTeamStore(selectViewScope)
  const team = useTeamStore(selectTeamView)
  const selectTeammate = useTeamStore((s) => s.selectTeammate)
  const selectedTeammate = useTeamStore((s) => s.selectedTeammate)

  if (!scope || scope.teamEvents.length === 0) {
    return (
      <Stack p="md" gap="xs">
        <Text size="sm" c="dimmed">
          {t('narrative.empty')}
        </Text>
      </Stack>
    )
  }

  return (
    <Stack gap={0} style={{ height: '100%', overflow: 'hidden' }}>
      {/* Team header */}
      {team && (
        <Group
          gap="xs"
          px="sm"
          py={8}
          style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
        >
          <IconUsersGroup size={16} color="var(--mantine-color-blue-4)" />
          <Text size="sm" fw={600}>{team.teamName || t('narrative.team')}</Text>
          <Badge size="xs" color={TEAM_STATE_COLOR[team.state] ?? 'gray'} variant="light">
            {team.state ? t(`narrative.teamState.${team.state}`, { defaultValue: team.state }) : t('narrative.unknown')}
          </Badge>
          <Text size="xs" c="dimmed" ff="monospace" truncate style={{ maxWidth: 180 }}>{team.teamId}</Text>
          <div style={{ flex: 1 }} />
          {team.members.map((m) => (
            <Badge
              key={m.memberId}
              size="sm"
              variant={selectedTeammate === m.memberId ? 'filled' : 'light'}
              color={MEMBER_STATE_COLOR[m.state] ?? 'gray'}
              style={{ cursor: 'pointer', textTransform: 'none' }}
              onClick={() => selectTeammate(selectedTeammate === m.memberId ? null : m.memberId)}
              title={t('narrative.memberTip', { member: m.memberId, state: memberState(t, m.state) })}
            >
              {m.memberId} · {memberState(t, m.state)}
            </Badge>
          ))}
        </Group>
      )}

      {/* Narrative timeline */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 16 }}>
        <Timeline bulletSize={20} lineWidth={1} active={scope.teamEvents.length - 1} color="blue">
          {scope.teamEvents.map((e, i) => {
            const Icon = ACTION_ICON[e.action] ?? IconActivity
            return (
              <Timeline.Item
                key={i}
                bullet={<Icon size={11} />}
                title={<Text size="xs" fw={600}>{narrativeTitle(t, e)}</Text>}
              >
                <Group gap={6}>
                  {e.ts > 0 && (
                    <Text size="xs" c="dimmed">{new Date(e.ts).toLocaleTimeString()}</Text>
                  )}
                  {e.runId && (
                    <Text size="xs" c="dimmed" ff="monospace">{e.runId.slice(0, 16)}</Text>
                  )}
                </Group>
                {e.detail && Object.keys(e.detail).length > 0 && (
                  <Paper p={6} mt={4} radius="sm" bg="dark.6" withBorder>
                    <Text size="xs" c="gray.4" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                      {detailText(e.detail)}
                    </Text>
                  </Paper>
                )}
              </Timeline.Item>
            )
          })}
        </Timeline>
      </div>
    </Stack>
  )
}

function memberState(t: T, state: string): string {
  return t(`narrative.memberState.${state}`, { defaultValue: state })
}

function narrativeTitle(t: T, e: TeamEventEntry): string {
  const member = e.memberId || t('narrative.someone')
  switch (e.action) {
    case 'created':              return t('narrative.event.created', { team: e.teamName || e.teamId })
    case 'member-joined':        return t('narrative.event.memberJoined', { member: e.memberId })
    case 'member-left':          return t('narrative.event.memberLeft', { member: e.memberId })
    case 'member-state-changed': return `${e.memberId} → ${memberState(t, e.state ?? '')}`
    case 'message':              return t('narrative.event.message', { member })
    case 'item-shared':          return t('narrative.event.itemShared', { member })
    case 'state-changed':        return t('narrative.event.stateChanged', { state: t(`narrative.teamState.${e.state}`, { defaultValue: e.state }) })
    case 'completed':            return t('narrative.event.completed')
    case 'failed':               return t('narrative.event.failed')
    default:                     return e.action
  }
}

function detailText(detail: Record<string, unknown>): string {
  // Common shapes: {summary}, {message}, {to}/{team}. Fall back to JSON.
  if (typeof detail.summary === 'string' && detail.summary) return detail.summary
  if (typeof detail.message === 'string' && detail.message) return detail.message
  try {
    return JSON.stringify(detail)
  } catch {
    return String(detail)
  }
}
