// SPDX-License-Identifier: Apache-2.0
/**
 * HttpPanel — DevTools-Network-style inspector over a session's
 * llm-model-calls.jsonl: one row per model call (status color-coded,
 * duration, first-byte, model), detail pane per selected call with
 * Headers / Request / Response / Stream sections. Polls while the
 * session is live; manual refresh otherwise.
 *
 * Empty states: logging disabled (points at `log_model_calls` in
 * LLMProfiles.yaml) vs no calls yet.
 */
import { useCallback, useEffect, useState } from 'react'
import {
  ActionIcon, Badge, Code, Group, Loader, ScrollArea, Stack, Table, Tabs, Text, Tooltip,
} from '@mantine/core'
import { StateBadge } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconRefresh } from '@tabler/icons-react'
import { GetSessionModelCalls } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

interface SSEEvent {
  type: string
  ts?: string
  data?: unknown
}

interface SSEContent {
  type: string
  name?: string
  id?: string
  text?: string
  input?: unknown
}

interface SSESummary {
  event_count?: number
  delta_count?: number
  text?: string
  contents?: SSEContent[]
}

interface ModelCall {
  callId: string
  provider?: string
  model?: string
  method?: string
  url?: string
  ts?: string
  status?: number
  durationMs?: number
  firstByteMs?: number
  error?: string
  inFlight?: boolean
  requestHeaders?: Record<string, string>
  requestBody?: unknown
  responseHeaders?: Record<string, string>
  responseBody?: unknown
  sseEvents?: SSEEvent[]
  sseSummary?: unknown
}

interface Props {
  universeName: string
  sessionId: string
  /** Poll the file every 2s while true (running session). */
  live?: boolean
}

export function HttpPanel({ universeName, sessionId, live }: Props) {
  const t = useT('screens')
  const [calls, setCalls] = useState<ModelCall[]>([])
  const [loggingOff, setLoggingOff] = useState(false)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  const refresh = useCallback(() => {
    GetSessionModelCalls(universeName, sessionId)
      .then((res) => {
        setLoaded(true)
        setError(res.error ?? '')
        setLoggingOff(!!res.loggingOff)
        setCalls((res.calls as unknown as ModelCall[]) ?? [])
      })
      .catch((e) => {
        setLoaded(true)
        setError(String(e))
      })
  }, [universeName, sessionId])

  useEffect(() => {
    refresh()
    if (!live) return
    const id = window.setInterval(refresh, 2000)
    return () => window.clearInterval(id)
  }, [refresh, live])

  if (!loaded) {
    return <Group p="md"><Loader size="sm" /><Text size="sm" c="dimmed">{t('http.loading')}</Text></Group>
  }
  if (error) {
    return <Text p="md" size="sm" c="red.4">{error}</Text>
  }
  if (loggingOff) {
    return (
      <Stack p="md" gap="xs">
        <Text size="sm" c="dimmed">{t('http.loggingOff')}</Text>
        <Text size="xs" c="dimmed">
          {t('http.enableHint', { flag: 'log_model_calls: true', file: '.katyoti/config/LLMProfiles.yaml' })}
        </Text>
      </Stack>
    )
  }
  if (calls.length === 0) {
    return <Text p="md" size="sm" c="dimmed">{t('http.noCalls')}</Text>
  }

  const sel = calls.find((c) => c.callId === selected)

  return (
    <Stack gap={0} style={{ height: '100%', overflow: 'hidden' }}>
      {/* Call rows */}
      <div style={{ flex: sel ? '0 0 40%' : 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <Group px="sm" py={4} gap="xs" style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}>
          <Text size="xs" fw={600}>{t('http.calls', { count: calls.length })}</Text>
          <Text size="xs" c="dimmed">
            {t('http.msTotal', { ms: calls.reduce((s, c) => s + (c.durationMs ?? 0), 0) })}
          </Text>
          <div style={{ flex: 1 }} />
          {!live && (
            <Tooltip label={t('http.refresh')} withArrow>
              <ActionIcon size="xs" variant="subtle" color="gray" onClick={refresh}>
                <IconRefresh size={12} />
              </ActionIcon>
            </Tooltip>
          )}
          {live && <Badge size="xs" color="teal" variant="light">{t('http.live')}</Badge>}
        </Group>
        <ScrollArea style={{ flex: 1 }}>
          <Table highlightOnHover withRowBorders={false} verticalSpacing={4} styles={{ td: { fontSize: 12 }, th: { fontSize: 11 } }}>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>#</Table.Th>
                <Table.Th>{t('http.model')}</Table.Th>
                <Table.Th>{t('http.path')}</Table.Th>
                <Table.Th>{t('http.status')}</Table.Th>
                <Table.Th ta="right">{t('http.firstByte')}</Table.Th>
                <Table.Th ta="right">{t('http.duration')}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {calls.map((c) => (
                <Table.Tr
                  key={c.callId}
                  onClick={() => setSelected(c.callId === selected ? null : c.callId)}
                  style={{ cursor: 'pointer', background: c.callId === selected ? 'var(--mantine-color-dark-6)' : undefined }}
                >
                  <Table.Td c="dimmed">{c.callId}</Table.Td>
                  <Table.Td>{c.model || c.provider}</Table.Td>
                  <Table.Td c="dimmed" style={{ maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {pathOf(c.url)}
                  </Table.Td>
                  <Table.Td><CallState call={c} /></Table.Td>
                  <Table.Td ta="right" c="dimmed">{c.firstByteMs ? `${c.firstByteMs} ms` : '—'}</Table.Td>
                  <Table.Td ta="right">{c.durationMs ? `${c.durationMs} ms` : '—'}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      </div>

      {/* Detail pane */}
      {sel && (
        <div style={{ flex: 1, borderTop: '1px solid var(--mantine-color-dark-4)', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <Group px="sm" py={4} gap="xs" style={{ flexShrink: 0 }}>
            <CallState call={sel} />
            <Text size="xs" fw={600}>{sel.method} {pathOf(sel.url)}</Text>
            <Text size="xs" c="dimmed">{sel.model}</Text>
            {sel.error && <Text size="xs" c="red.4" truncate>{sel.error}</Text>}
          </Group>
          <Tabs defaultValue="response" styles={{ root: { flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }, panel: { flex: 1, overflow: 'auto' }, tab: { fontSize: 12, padding: '4px 10px' } }}>
            <Tabs.List>
              <Tabs.Tab value="headers">{t('http.headers')}</Tabs.Tab>
              <Tabs.Tab value="request">{t('http.request')}</Tabs.Tab>
              <Tabs.Tab value="response">{t('http.response')}</Tabs.Tab>
              {(!!sel.sseEvents?.length || sel.sseSummary != null) && <Tabs.Tab value="stream">{t('http.stream')}</Tabs.Tab>}
            </Tabs.List>

            <Tabs.Panel value="headers" p="xs">
              <HeaderTable title={t('http.requestHeaders')} headers={sel.requestHeaders} />
              <HeaderTable title={t('http.responseHeaders')} headers={sel.responseHeaders} />
            </Tabs.Panel>

            <Tabs.Panel value="request" p="xs">
              <JsonBlock value={sel.requestBody} empty={t('http.noRequestBody')} />
            </Tabs.Panel>

            <Tabs.Panel value="response" p="xs">
              {sel.responseBody != null ? (
                <JsonBlock value={sel.responseBody} empty="" />
              ) : sel.sseSummary != null ? (
                <ContentsList summary={sel.sseSummary as SSESummary} />
              ) : (
                <Text size="xs" c="dimmed">{sel.inFlight ? t('http.inFlight') : t('http.noResponseBody')}</Text>
              )}
            </Tabs.Panel>

            <Tabs.Panel value="stream" p="xs">
              <Stack gap={6}>
                {(sel.sseEvents ?? []).map((e, i) => (
                  <div key={i}>
                    <Group gap={6}>
                      <Badge size="xs" variant="light" color="violet" styles={{ root: { textTransform: 'none' } }}>{e.type}</Badge>
                      {e.ts && <Text size="xs" c="dimmed">{timeOf(e.ts)}</Text>}
                    </Group>
                    {e.data != null && <JsonBlock value={e.data} empty="" compact />}
                  </div>
                ))}
                {sel.sseSummary != null && (
                  <div>
                    <Badge size="xs" variant="light" color="gray" styles={{ root: { textTransform: 'none' } }}>{t('http.deltas')}</Badge>
                    <JsonBlock value={sel.sseSummary} empty="" compact />
                  </div>
                )}
              </Stack>
            </Tabs.Panel>
          </Tabs>
        </div>
      )}
    </Stack>
  )
}

/** Streamed response rendered as a list of contents — one card per
 *  received content block (text / tool_use / thinking), in order. */
function ContentsList({ summary }: { summary: SSESummary }) {
  const t = useT('screens')
  const contents = summary.contents ?? []
  if (contents.length === 0) {
    // Older logs (pre-contents) or empty streams: fall back to the
    // assembled text / raw summary.
    return summary.text
      ? <Code block style={{ fontSize: 12, whiteSpace: 'pre-wrap' }}>{summary.text}</Code>
      : <JsonBlock value={summary} empty={t('http.emptyStream')} />
  }
  return (
    <Stack gap="xs">
      {contents.map((c, i) => (
        <div key={i}>
          <Group gap={6} mb={2}>
            <Badge
              size="xs"
              variant="light"
              color={c.type === 'tool_use' ? 'orange' : c.type === 'thinking' ? 'gray' : 'blue'}
              styles={{ root: { textTransform: 'none' } }}
            >
              {c.type}{c.name ? ` · ${c.name}` : ''}
            </Badge>
            {c.id && <Text size="xs" c="dimmed" ff="monospace">{c.id}</Text>}
          </Group>
          {c.text != null && c.text !== '' && (
            <Code block style={{ fontSize: 12, whiteSpace: 'pre-wrap' }}>{c.text}</Code>
          )}
          {c.input != null && <JsonBlock value={c.input} empty="" compact />}
        </div>
      ))}
    </Stack>
  )
}


function HeaderTable({ title, headers }: { title: string; headers?: Record<string, string> }) {
  const entries = Object.entries(headers ?? {})
  return (
    <Stack gap={2} mb="sm">
      <Text size="xs" fw={600} c="dimmed" tt="uppercase" style={{ fontSize: 10, letterSpacing: 0.5 }}>{title}</Text>
      {entries.length === 0 && <Text size="xs" c="dimmed">—</Text>}
      {entries.map(([k, v]) => (
        <Group key={k} gap={6} wrap="nowrap" align="flex-start">
          <Text size="xs" fw={600} style={{ minWidth: 180 }}>{k}</Text>
          <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>{v}</Text>
        </Group>
      ))}
    </Stack>
  )
}

function JsonBlock({ value, empty, compact }: { value: unknown; empty: string; compact?: boolean }) {
  if (value == null) {
    return empty ? <Text size="xs" c="dimmed">{empty}</Text> : null
  }
  let text: string
  try {
    text = typeof value === 'string' ? value : JSON.stringify(value, null, 2)
  } catch {
    text = String(value)
  }
  return (
    <Code block style={{ fontSize: 11, whiteSpace: 'pre-wrap', maxHeight: compact ? 160 : undefined, overflowY: 'auto' }}>
      {text}
    </Code>
  )
}

function pathOf(url?: string): string {
  if (!url) return ''
  try {
    return new URL(url).pathname
  } catch {
    return url
  }
}

function timeOf(ts: string): string {
  const t = Date.parse(ts)
  return isNaN(t) ? ts : new Date(t).toLocaleTimeString()
}

/** A model call's state: pending while in flight, an error, or its HTTP status. */
function CallState({ call }: { call: ModelCall }) {
  if (call.inFlight) return <StateBadge value="pending" tones={CALL_TONES} loading />
  if (call.error && !call.status) return <StateBadge value="error" tones={CALL_TONES} />
  const ok = (call.status ?? 0) < 400
  return <StateBadge value={ok ? 'ok' : 'error'} tones={CALL_TONES} label={call.status || '—'} />
}

const CALL_TONES: Record<string, string> = { pending: 'blue', ok: 'green', error: 'red' }
