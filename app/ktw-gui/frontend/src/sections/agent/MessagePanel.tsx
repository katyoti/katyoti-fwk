// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from 'react'
import { Center, ScrollArea, Stack, Text } from '@mantine/core'
import { IconBolt } from '@tabler/icons-react'
import { useSessionMessages, useSessionStatus } from '../../store'
import { MessageList } from './MessageList'
import { useT } from '../../i18n'

/** Scrollable message area — fills available height between ChatHeader and
 *  ChatInputPanel. Bound to a session: reads THIS session's messages, not
 *  the global mirror (so parallel chats don't share content). */
export function MessagePanel({ sessionId }: { sessionId?: string } = {}) {
  const sessionStatus = useSessionStatus(sessionId)
  const messages      = useSessionMessages(sessionId)
  const viewport      = useRef<HTMLDivElement>(null)
  const t = useT('chat')

  // Auto-scroll to bottom on new messages or content changes
  useEffect(() => {
    requestAnimationFrame(() => {
      if (viewport.current) {
        viewport.current.scrollTop = viewport.current.scrollHeight
      }
    })
  }, [messages])

  if (sessionStatus === 'idle') {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap="sm" style={{ opacity: 0.35 }}>
          <IconBolt size={40} />
          <Text size="sm" c="dimmed">{t('panel.selectTask')}</Text>
        </Stack>
      </Center>
    )
  }

  return (
    <ScrollArea
      style={{ height: '100%' }}
      viewportRef={viewport}
      scrollbarSize={6}
    >
      {/* Bottom padding clears the floating input / Done card (absolute,
          ~70px tall at bottom:24) so the last message scrolls above it. */}
      <div style={{ paddingBottom: 120 }}>
        <MessageList sessionId={sessionId} />
      </div>
    </ScrollArea>
  )
}
