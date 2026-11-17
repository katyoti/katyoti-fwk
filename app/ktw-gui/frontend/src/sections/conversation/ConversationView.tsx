// SPDX-License-Identifier: Apache-2.0
/**
 * ConversationView — the read-only speaker→message transcript for a
 * session or a live run. One shared component behind both the session
 * viewer's "Conversation" tab and the live chat's debug "Conversation"
 * tab. Pure rendering over a normalized TranscriptEntry list; callers
 * supply the entries (a fetched session journal, or the live message
 * store mapped via liveMessagesToTranscript).
 */

import { useState } from 'react'
import { Box, Code, Group, ScrollArea, Stack, Text } from '@mantine/core'
import {
  IconCircleX,
  IconInfoCircle,
  IconRobot,
  IconTool,
  IconUser,
} from '@tabler/icons-react'
import type { MsgItem } from '../../types/messages'
import { useT, type TFn } from '../../i18n'

// Wire shape mirrors `app/ktw-gui/app_sessions.go::SessionTranscriptEntry`.
// Kept inline because the wails-generated `main.SessionTranscriptEntry`
// types data as `any` and we specialise it here for rendering.
export interface TranscriptEntry {
  type:      string
  turn?:     number
  timestamp: string
  data?:     unknown
}

interface Props {
  transcript: TranscriptEntry[]
  /** Optional initial-prompt panel pinned at the top — conceptually
   *  part of the reading experience, not the host's chrome. */
  initialPrompt?: string | null
}

const INITIAL_PROMPT_INLINE_CHARS = 200

export function ConversationView({ transcript, initialPrompt }: Props) {
  const t = useT('screens')
  return (
    <Stack gap={0} style={{ height: '100%' }}>
      {initialPrompt && <InitialContextPanel prompt={initialPrompt} />}
      <ScrollArea style={{ flex: 1 }} scrollbarSize={6}>
        <Stack gap="sm" p="md">
          {transcript.length === 0 && (
            <Text size="sm" c="dimmed" ta="center" py="xl">
              {t('conversation.empty')}
            </Text>
          )}
          {transcript.map((entry, i) => (
            <TranscriptRow key={i} entry={entry} />
          ))}
        </Stack>
      </ScrollArea>
    </Stack>
  )
}

function InitialContextPanel({ prompt }: { prompt: string }) {
  const t = useT('screens')
  const [expanded, setExpanded] = useState(false)
  const truncated = prompt.length > INITIAL_PROMPT_INLINE_CHARS
  const shown = !truncated || expanded ? prompt : prompt.slice(0, INITIAL_PROMPT_INLINE_CHARS) + '…'
  return (
    <Box
      px="md"
      py="xs"
      style={{
        borderBottom: '1px solid var(--mantine-color-default-border)',
        background: 'var(--mantine-color-dark-7)',
        flexShrink: 0,
      }}
    >
      <Text size="xs" c="dimmed" fw={600} tt="uppercase" mb={4} style={{ letterSpacing: 0.5 }}>
        {t('conversation.initialPrompt')}
      </Text>
      <Text size="sm" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {shown}
      </Text>
      {truncated && (
        <Text
          size="xs"
          c="blue"
          mt={4}
          style={{ cursor: 'pointer', userSelect: 'none' }}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? t('conversation.showLess') : t('conversation.showMore')}
        </Text>
      )}
    </Box>
  )
}

function TranscriptRow({ entry }: { entry: TranscriptEntry }) {
  const t = useT('screens')
  const { color, icon, label } = roleStyle(entry.type, t)
  const text = renderEntryText(entry)
  return (
    <Box>
      <Group gap={6} mb={2} wrap="nowrap">
        {icon}
        <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ letterSpacing: 0.5 }}>{label}</Text>
        {entry.turn ? <Text size="xs" c="dimmed">{t('conversation.turn', { turn: entry.turn })}</Text> : null}
      </Group>
      <Box style={{
        borderLeft: `2px solid var(--mantine-color-${color}-6)`,
        paddingLeft: 8,
        marginLeft: 2,
      }}>
        {typeof text === 'string'
          ? <Text size="sm" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{text}</Text>
          : <Code block style={{ fontSize: 11 }}>{JSON.stringify(entry.data, null, 2)}</Code>}
      </Box>
    </Box>
  )
}

function roleStyle(type: string, t: TFn): { color: string; icon: JSX.Element; label: string } {
  switch (type) {
    case 'message_shown':
      return { color: 'blue',   icon: <IconRobot size={12} />,       label: t('conversation.assistant') }
    case 'prompt_shown':
      // The agent asking the user a question (ask-user). It's the
      // agent talking, NOT a system notice — labeling it "System"
      // is what made the transcript read as too machine-y.
      return { color: 'blue',   icon: <IconRobot size={12} />,       label: t('conversation.assistant') }
    case 'user_input':
      return { color: 'teal',   icon: <IconUser size={12} />,        label: t('conversation.you') }
    case 'tool_feedback':
      return { color: 'grape',  icon: <IconTool size={12} />,        label: t('conversation.tool') }
    case 'system_message':
      return { color: 'gray',   icon: <IconInfoCircle size={12} />,  label: t('conversation.system') }
    case 'error_shown':
      return { color: 'red',    icon: <IconCircleX size={12} />,     label: t('conversation.error') }
    default:
      return { color: 'gray',   icon: <IconInfoCircle size={12} />,  label: type }
  }
}

function renderEntryText(entry: TranscriptEntry): string | null {
  const d = entry.data as Record<string, unknown> | null | undefined
  if (!d) return null
  if (typeof d.text === 'string') return d.text
  if (typeof d.message === 'string') return d.message
  if (typeof d.content === 'string') return d.content
  if (typeof d.result === 'string') {
    const head = typeof d.tool === 'string' ? `${d.tool}: ` : ''
    return head + d.result
  }
  if (typeof d.value === 'string') return d.value
  if (typeof d.prompt === 'string') return d.prompt
  return null
}

/** Find the first user-supplied prompt that started the session. Tries
 *  `user_input` then `prompt_shown`, in transcript order. Returns null
 *  if no usable starting prompt is in the journal. */
export function findInitialPrompt(transcript: TranscriptEntry[]): string | null {
  for (const entry of transcript) {
    if (entry.type === 'user_input') {
      const text = renderEntryText(entry)
      if (text) return text
    }
  }
  for (const entry of transcript) {
    if (entry.type === 'prompt_shown') {
      const text = renderEntryText(entry)
      if (text) return text
    }
  }
  return null
}

/** liveMessagesToTranscript adapts the live chat message store
 *  (MsgItem[]) into the TranscriptEntry shape ConversationView renders,
 *  so the live debug "Conversation" tab reuses the exact same view as
 *  the session viewer. The mapping mirrors roleStyle's type vocabulary:
 *    TextMsg(user)  → user_input        TextMsg(other) → message_shown
 *    ToolMsg        → tool_feedback      SysMsg(error)  → error_shown
 *    SysMsg(other)  → prompt_shown       PromptMsg      → prompt_shown
 */
export function liveMessagesToTranscript(messages: MsgItem[]): TranscriptEntry[] {
  const out: TranscriptEntry[] = []
  for (const m of messages) {
    switch (m.kind) {
      case 'text':
        out.push({
          type: isUser(m.sender) ? 'user_input' : 'message_shown',
          timestamp: '',
          data: { text: m.text },
        })
        break
      case 'tool':
        out.push({
          type: 'tool_feedback',
          timestamp: '',
          data: { tool: m.name, result: m.output || m.error || m.input || '' },
        })
        break
      case 'system':
        out.push({
          type: m.level === 'error' ? 'error_shown' : 'system_message',
          timestamp: '',
          data: { text: m.text },
        })
        break
      case 'prompt':
        out.push({
          type: 'prompt_shown',
          timestamp: '',
          data: { prompt: m.answered && m.answer ? `${m.question}\n→ ${m.answer}` : m.question },
        })
        break
    }
  }
  return out
}

function isUser(sender: string): boolean {
  const s = sender.toLowerCase()
  return s === 'user' || s === 'you'
}
