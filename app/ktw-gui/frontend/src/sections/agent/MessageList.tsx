// SPDX-License-Identifier: Apache-2.0
import { Box, Button, Center, Group, Stack, Text } from '@mantine/core'
import { IconFilter, IconX } from '@tabler/icons-react'
import { MessageRenderer } from './MessageRenderer'
import { useSessionMessages } from '../../store'
import { useTeamStore, selectViewScope } from '../../store/teamSlice'
import { shortRunID } from '../team/useTeamLiveSource'
import type { MsgItem }    from '../../types/messages'
import { useT } from '../../i18n'

function chainKey(msg: MsgItem): string {
  return msg.sender
}

export function MessageList({ sessionId }: { sessionId?: string } = {}) {
  const all = useSessionMessages(sessionId)
  // Reuse the Team panel's node selection: clicking a delegate card focuses
  // the chat on that delegate's own stream. Cleared (null) → the whole tree.
  const focusedRun = useTeamStore((s) => s.selectedTaskRunId)
  const selectTask = useTeamStore((s) => s.selectTask)
  const scope = useTeamStore(selectViewScope)
  const t = useT('chat')

  const messages = focusedRun ? all.filter((m) => m.sourceRun === focusedRun) : all
  const focusedName = focusedRun ? (scope?.children.get(focusedRun)?.name ?? shortRunID(focusedRun)) : ''
  // The sentence around the (bold) delegate name, split at its placeholder.
  const NAME_SLOT = '\u0000'
  const [showingBefore, showingAfter = ''] = t('list.showing', { name: NAME_SLOT }).split(NAME_SLOT)

  return (
    <Stack gap={6} p="md" pb={4}>
      {focusedRun && (
        <Group
          gap="xs"
          justify="space-between"
          px="sm"
          py={4}
          style={{
            borderRadius: 6,
            background: 'var(--mantine-color-dark-6)',
            border: '1px solid var(--mantine-color-dark-4)',
          }}
        >
          <Group gap={6}>
            <IconFilter size={13} />
            <Text size="xs" c="dimmed">
              {showingBefore}<b>{focusedName}</b>{showingAfter}
            </Text>
          </Group>
          <Button
            size="compact-xs"
            variant="subtle"
            color="gray"
            leftSection={<IconX size={12} />}
            onClick={() => selectTask(null)}
          >
            {t('list.showAll')}
          </Button>
        </Group>
      )}

      {focusedRun && messages.length === 0 && (
        <Center py="lg">
          <Text size="xs" c="dimmed">{t('list.noDelegateMessages')}</Text>
        </Center>
      )}

      {messages.map((msg, idx) => {
        const prev           = idx > 0 ? messages[idx - 1] : null
        const isFirstInChain = !prev || chainKey(prev) !== chainKey(msg)

        return (
          <MessageRenderer
            key={msg.id}
            msg={msg}
            isFirstInChain={isFirstInChain}
          />
        )
      })}
      <Box h={8} />
    </Stack>
  )
}
