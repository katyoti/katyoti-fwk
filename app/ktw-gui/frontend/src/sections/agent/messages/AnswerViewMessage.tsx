// SPDX-License-Identifier: Apache-2.0
import { Stack, Text } from '@mantine/core'
import type { MsgItem, AnswerViewMsg } from '../../../types/messages'
import { InlineUIView } from '../InlineUIView'

/** Renders the user's submitted ask-user-data answer as a read-only,
 *  schema-driven view card in the chat thread. */
export function AnswerViewContent({ msg }: { msg: MsgItem }) {
  const m = msg as AnswerViewMsg
  return (
    <Stack gap={4}>
      {m.title && (
        <Text size="xs" c="dimmed" fw={600}>{m.title}</Text>
      )}
      <InlineUIView schema={m.schema} item={m.value} />
    </Stack>
  )
}
