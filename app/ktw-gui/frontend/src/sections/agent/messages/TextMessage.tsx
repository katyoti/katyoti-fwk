// SPDX-License-Identifier: Apache-2.0
/**
 * TextMessageContent — naked text content renderer
 * ─────────────────────────────────────────────────────────────────────────
 * Plain text for user messages, markdown for assistant messages.
 * No box, no border — sender header + indentation handled by MessageRenderer.
 */
import { Loader, Stack, Text } from '@mantine/core'
import { Markdown } from '../../../components/Markdown'
import type { MsgItem, TextMsg } from '../../../types/messages'

/**
 * Normalize text for markdown rendering:
 * - Replace literal \n (escaped newlines) with real newlines
 * - Replace literal \t with real tabs
 */
function normalizeMarkdown(text: string): string {
  return text
    .replace(/\\n/g, '\n')
    .replace(/\\t/g, '\t')
}

/** Content renderer used by MessageWrapperRaw via the registry */
export function TextMessageContent({ msg }: { msg: MsgItem }) {
  const m = msg as TextMsg

  if (m.sender === 'user') {
    return <Text size="sm" style={{ lineHeight: 1.6 }}>{m.text}</Text>
  }

  return (
    <Stack gap={0}>
      <Markdown text={normalizeMarkdown(m.text)} />
      {!m.done && <Loader size="xs" type="dots" mt={6} />}
    </Stack>
  )
}
