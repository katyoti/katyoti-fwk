// SPDX-License-Identifier: Apache-2.0
/**
 * Custom React Flow node — an agent "deck card".
 * Renders as a Mantine Paper with status glow, icon, stats bar.
 */
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { ActionIcon, Badge, Group, Paper, Stack, Text, Tooltip } from '@mantine/core'
import {
  IconBrain, IconUsers, IconUser, IconTool, IconMessage, IconClock, IconBan, IconDatabase,
} from '@tabler/icons-react'
import type { AgentNodeData, AgentStatus } from './teamNodeTypes'
import { CancelChild } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

const STATUS_COLOR: Record<AgentStatus, string> = {
  idle:    'gray',
  running: 'blue',
  done:    'green',
  error:   'red',
  waiting: 'yellow',
}

const STATUS_GLOW: Record<AgentStatus, string> = {
  idle:    'none',
  running: '0 0 12px rgba(59,130,246,0.5)',
  done:    '0 0 8px rgba(76,175,80,0.4)',
  error:   '0 0 8px rgba(244,67,54,0.4)',
  waiting: '0 0 8px rgba(255,193,7,0.3)',
}

const ROLE_ICON: Record<string, typeof IconBrain> = {
  coordinator: IconBrain,
  'team-lead': IconUsers,
  worker: IconUser,
}

const CAPABILITY_SHORT: Record<string, string> = {
  'planning-best': '🧠',
  'smart-best': '⚡',
  'fast-best': '🏎',
  'creative-best': '🎨',
  'workflow-best': '⚙️',
}

export function AgentNode(props: NodeProps) {
  const t = useT('runs')
  const tt = useT('team')
  const d = props.data as unknown as AgentNodeData
  const runId = props.id // useTeamLiveSource sets node.id = child.runId
  const color = STATUS_COLOR[d.status] || 'gray'
  const glow = STATUS_GLOW[d.status] || 'none'
  const RoleIcon = ROLE_ICON[d.role] || IconUser
  const capEmoji = CAPABILITY_SHORT[d.capability] || '🤖'
  const isLead = d.role === 'coordinator' || d.role === 'team-lead'
  const canCancel = d.status === 'running' || d.status === 'waiting'
  const isLedger = d.kind === 'ledger'

  const handleCancel = (e: React.MouseEvent) => {
    e.stopPropagation()
    // Minimal native confirm — adding @mantine/modals would be more polished,
    // but this is a destructive action where the OS dialog's friction is OK.
    const ok = window.confirm(t('team.cancelChild', { label: d.label }))
    if (ok) {
      void CancelChild(runId, d.label)
    }
  }

  // Ledger nodes are NOT agents that run — they're shared-state (the Magentic
  // pattern). Render them as a distinct, static "shared-state" element: grape
  // database card, no running glow, no agent stats / cancel. The full timeline
  // lives in the Ledger tab; this node just marks its place in the topology.
  if (isLedger) {
    return (
      <>
        <Handle type="target" position={Position.Top} style={{ background: '#555', width: 8, height: 8 }} />
        <Paper
          shadow="xs"
          radius="md"
          p="xs"
          style={{
            background: 'var(--mantine-color-dark-7)',
            border: '2px dashed var(--mantine-color-grape-6)',
            minWidth: 170,
            cursor: 'pointer',
          }}
        >
          <Stack gap={4}>
            <Group gap={6} wrap="nowrap">
              <IconDatabase size={16} color="var(--mantine-color-grape-4)" />
              <Text size="sm" fw={600} c="gray.1" style={{ flex: 1 }} truncate>
                {d.label}
              </Text>
            </Group>
            <Badge size="xs" color="grape" variant="light" styles={{ root: { textTransform: 'none' } }}>
              {tt('agentNode.ledger')}
            </Badge>
            {d.summary && (
              <Text size="xs" c="gray.4" lineClamp={1}>
                {d.summary}
              </Text>
            )}
          </Stack>
        </Paper>
        <Handle type="source" position={Position.Bottom} style={{ background: '#555', width: 8, height: 8 }} />
      </>
    )
  }

  return (
    <>
      <Handle type="target" position={Position.Top} style={{ background: '#555', width: 8, height: 8 }} />

      <Paper
        shadow="md"
        radius="md"
        p="xs"
        style={{
          background: 'var(--mantine-color-dark-7)',
          border: `2px solid var(--mantine-color-${color}-6)`,
          boxShadow: glow,
          minWidth: isLead ? 200 : 170,
          cursor: 'pointer',
          transition: 'box-shadow 0.3s, border-color 0.3s',
        }}
      >
        <Stack gap={4}>
          {/* Header: icon + name + capability + cancel button */}
          <Group gap={6} wrap="nowrap">
            <RoleIcon size={16} color={`var(--mantine-color-${color}-4)`} />
            <Text size="sm" fw={600} c="gray.1" style={{ flex: 1 }} truncate>
              {d.label}
            </Text>
            <Text size="xs">{capEmoji}</Text>
            {canCancel && (
              <Tooltip label={t('team.cancelChildTip')} position="top" withArrow>
                <ActionIcon
                  size="xs"
                  variant="subtle"
                  color="red"
                  onClick={handleCancel}
                  aria-label={t('team.cancelChildLabel', { label: d.label })}
                >
                  <IconBan size={12} />
                </ActionIcon>
              </Tooltip>
            )}
          </Group>

          {/* Status + turns */}
          <Group gap={6} wrap="nowrap">
            <Badge
              size="xs"
              color={color}
              variant={d.status === 'running' ? 'filled' : 'light'}
              styles={{ root: { textTransform: 'none' } }}
            >
              {d.status === 'running' ? `● ${tt('agentNode.status.running')}` : d.status === 'done' ? `✓ ${tt('agentNode.status.done')}` : d.status === 'error' ? `✗ ${tt('agentNode.status.error')}` : d.status === 'waiting' ? `◌ ${tt('agentNode.status.waiting')}` : `○ ${tt('agentNode.status.idle')}`}
            </Badge>
            <Text size="xs" c="dimmed">T:{d.turns}</Text>
            {d.mode && (
              <Tooltip label={tt('agentNode.modeTip', { mode: d.mode })} position="top" withArrow>
                <Badge
                  size="xs"
                  variant="outline"
                  color="gray"
                  styles={{ root: { textTransform: 'none', fontWeight: 500 } }}
                >
                  {d.mode}
                </Badge>
              </Tooltip>
            )}
          </Group>

          {/* Summary line */}
          <Text size="xs" c="gray.4" lineClamp={1}>
            {d.summary}
          </Text>

          {/* Stats bar */}
          <Group gap={8} wrap="nowrap">
            <Group gap={2} wrap="nowrap">
              <IconTool size={10} color="var(--mantine-color-dimmed)" />
              <Text size="xs" c="dimmed">{d.toolCount}</Text>
            </Group>
            <Group gap={2} wrap="nowrap">
              <IconMessage size={10} color="var(--mantine-color-dimmed)" />
              <Text size="xs" c="dimmed">{d.promptCount}</Text>
            </Group>
            {d.turns > 0 && (
              <Group gap={2} wrap="nowrap">
                <IconClock size={10} color="var(--mantine-color-dimmed)" />
                <Text size="xs" c="dimmed">{d.turns * 3}s</Text>
              </Group>
            )}
          </Group>
        </Stack>
      </Paper>

      <Handle type="source" position={Position.Bottom} style={{ background: '#555', width: 8, height: 8 }} />
    </>
  )
}
