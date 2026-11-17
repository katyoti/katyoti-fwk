// SPDX-License-Identifier: Apache-2.0
/**
 * MailboxPanel — side panel showing one teammate's mailbox: messages received
 * from other agents (or from the GUI user) plus a send-message input.
 *
 * Direct messages only for v1. Group broadcasts and create-team / spawn-
 * teammate UIs are deferred to gui-team-tab-interactive.
 */
import { useState } from 'react'
import { ActionIcon, Group, Paper, Stack, Text, Textarea, Button } from '@mantine/core'
import { IconX, IconSend } from '@tabler/icons-react'
import { useTeamStore, selectViewScope } from '../../store/teamSlice'
import { SendTeamMessage } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

interface Props {
  teammate: string
  onClose: () => void
}

export function MailboxPanel({ teammate, onClose }: Props) {
  const t = useT('team')
  const messages = useTeamStore((s) => selectViewScope(s)?.mailboxes.get(teammate) ?? [])
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)

  const handleSend = async () => {
    const text = draft.trim()
    if (!text) return
    setSending(true)
    try {
      await SendTeamMessage(teammate, text)
      setDraft('')
    } finally {
      setSending(false)
    }
  }

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group
        justify="space-between"
        px="sm"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Text size="sm" fw={600}>{t('mailbox.title', { teammate })}</Text>
        <ActionIcon size="sm" variant="subtle" color="gray" onClick={onClose}>
          <IconX size={14} />
        </ActionIcon>
      </Group>

      <Stack
        gap="xs"
        p="sm"
        style={{ flex: 1, overflowY: 'auto' }}
      >
        {messages.length === 0 && (
          <Text size="xs" c="dimmed">{t('mailbox.empty')}</Text>
        )}
        {messages.map((m, i) => (
          <Paper key={i} p="xs" radius="sm" bg="dark.6" withBorder>
            <Group gap="xs" mb={4}>
              <Text size="xs" fw={600} c={m.from === 'gui-user' ? 'teal' : 'blue'}>
                {m.from}
              </Text>
              <Text size="xs" c="dimmed">{new Date(m.ts).toLocaleTimeString()}</Text>
            </Group>
            <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>{m.message}</Text>
          </Paper>
        ))}
      </Stack>

      <Stack
        gap="xs"
        p="sm"
        style={{ borderTop: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Textarea
          placeholder={t('mailbox.placeholder', { teammate })}
          value={draft}
          onChange={(e) => setDraft(e.currentTarget.value)}
          autosize
          minRows={2}
          maxRows={5}
          disabled={sending}
        />
        <Button
          leftSection={<IconSend size={14} />}
          size="xs"
          onClick={handleSend}
          disabled={!draft.trim() || sending}
          loading={sending}
        >
          {t('mailbox.send')}
        </Button>
      </Stack>
    </Stack>
  )
}
