// SPDX-License-Identifier: Apache-2.0
import { Box, Text } from '@mantine/core'
import type { MsgItem, SysMsg } from '../../../types/messages'

interface Props { msg: MsgItem }

/** Pure system message content — rendered inside MessageWrapper's expanded Paper */
export function SystemMessageContent({ msg }: Props) {
  const m = msg as SysMsg
  return (
    <Box px="sm" py={6}>
      <Text size="xs" ff="monospace" c="dimmed" style={{ lineHeight: 1.5 }}>
        {m.text}
      </Text>
    </Box>
  )
}
