// SPDX-License-Identifier: Apache-2.0
import {
  Code,
  ScrollArea,
  Stack,
  Text,
} from '@mantine/core'
import type { MsgItem, ToolMsg } from '../../../types/messages'
import { useT } from '../../../i18n'

interface Props { msg: MsgItem }

function prettyJSON(raw: string): string {
  try { return JSON.stringify(JSON.parse(raw), null, 2) }
  catch { return raw }
}

function CodeSection({ label, content, isError = false }: { label: string; content: string; isError?: boolean }) {
  return (
    <Stack gap={4}>
      <Text size="xs" fw={600} c="dimmed">{label}</Text>
      <ScrollArea mah={200} scrollbarSize={6}>
        <Code block c={isError ? 'red.4' : undefined} style={{ fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
          {content}
        </Code>
      </ScrollArea>
    </Stack>
  )
}

/** Tool content body — input/output/error, rendered inside MessageWrapper's expanded Paper */
export function ToolMessageContent({ msg }: Props) {
  const m = msg as ToolMsg
  const t = useT('chat')

  const hasBody = !!(m.input || m.output || m.error)
  if (!hasBody) return null

  return (
    <Stack gap="sm" p="sm">
      {m.input  && <CodeSection label={t('tool.input')}       content={prettyJSON(m.input)} />}
      {m.output && <CodeSection label={t('tool.output')}      content={m.output} />}
      {m.error  && <CodeSection label={t('tool.errorOutput')} content={m.error} isError />}
    </Stack>
  )
}
