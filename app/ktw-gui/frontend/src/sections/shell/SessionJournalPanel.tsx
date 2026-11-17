// SPDX-License-Identifier: Apache-2.0
/**
 * SessionJournalPanel — LLM-level journal browser for a single session.
 *
 * Mirrors `ktw-utils.exe journal` (the TUI) in the GUI: same data
 * source (`<sessionDir>/llm.jsonl`), same projected shape (turns + user
 * text + assistant text + tool calls), different rendering substrate.
 *
 * Header card shows session-wide metadata (model / provider / token
 * counts / duration), the available tool list, and the system prompt
 * in a collapsible block. The per-turn cards stack below with a sticky
 * turn-number gutter on the left so long transcripts stay navigable.
 *
 * Per-tool-call rendering: the input is pretty-printed JSON in a
 * collapsible code block; the result is also collapsible and tagged
 * with a red border when the result looks like an error envelope (the
 * Go side sets `error: true` for these — same `{"status":"error",...}`
 * heuristic ktw-utils uses).
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Card,
  Center,
  Code,
  Group,
  Loader,
  ScrollArea,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core'
import { ItemCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import {
  IconAlertCircle,
  IconChevronDown,
  IconChevronRight,
  IconClock,
  IconRobot,
  IconTool,
  IconUser,
} from '@tabler/icons-react'

import { GetSessionJournal } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'
import type { main } from '../../../wailsjs/go/models'

interface Props {
  universeName: string
  sessionId:    string
  /** Changes while the session runs: the journal is read again, in place. */
  refreshKey?:  number
}

export function SessionJournalPanel({ universeName, sessionId, refreshKey = 0 }: Props) {
  const t = useT('shell')
  const [conv, setConv]           = useState<main.SessionJournalConversation | null>(null)
  const [loading, setLoading]     = useState(true)
  const [error, setError]         = useState<string | null>(null)
  // Empty set = show all runs; otherwise show only the labels in the set.
  // Toggled by the run-legend pills in SessionMetaCard.
  const [runFilter, setRunFilter] = useState<Set<string>>(new Set())

  // A new session starts clean; a refresh of the same one keeps what is shown
  // (and the run filter) until the journal read again replaces it.
  useEffect(() => {
    setLoading(true)
    setConv(null)
    setRunFilter(new Set())
  }, [universeName, sessionId])

  useEffect(() => {
    let cancelled = false
    setError(null)
    GetSessionJournal(universeName, sessionId)
      .then((r) => {
        if (cancelled) return
        if (r.error) {
          // A missing journal (a chat session, a run that died before its first
          // write, or one still starting) is NOT an error surface — degrade to
          // the friendly "No journal recorded" empty state below instead of a
          // red "Journal unavailable". Only unexpected errors bubble up.
          if (!/not found|no such|does not exist|no journal/i.test(r.error)) {
            setError(r.error)
          }
        } else {
          setConv(r.conversation ?? null)
        }
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, sessionId, refreshKey])

  if (loading) {
    return <Center py="xl"><Loader size="sm" /></Center>
  }
  if (error) {
    return (
      <Box p="md">
        <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light" title={t('sessionJournal.unavailable')}>
          {error}
        </Alert>
      </Box>
    )
  }
  if (!conv) {
    return (
      <Center py="xl"><Text size="sm" c="dimmed">{t('sessionJournal.empty')}</Text></Center>
    )
  }

  const colorByRun = runColorMap(conv.runs)
  const runs = conv.runs ?? []
  const multiRun = runs.length > 1

  const toggleRun = (label: string) =>
    setRunFilter((prev) => {
      const next = new Set(prev)
      if (next.has(label)) next.delete(label)
      else next.add(label)
      return next
    })

  // Empty filter = all runs; otherwise only the selected labels.
  const filteredTurns = (conv.turns ?? []).filter(
    (t) => runFilter.size === 0 || runFilter.has(t.runLabel ?? ''),
  )

  // Per-run dedup of the triggering user prompt: every turn of a run sees
  // the same original human message in its history, so show it only on the
  // run's first turn. Keyed by run so a child's prompt isn't hidden just
  // because the parent's previous turn carried the same text.
  const lastUserTextByRun: Record<string, string | undefined> = {}

  return (
    <ScrollArea style={{ height: '100%' }} scrollbarSize={6}>
      <Box p="md">
        <Stack gap="md">
          <SessionMetaCard
            conv={conv}
            colorByRun={colorByRun}
            runFilter={runFilter}
            onToggleRun={toggleRun}
            onClearRuns={() => setRunFilter(new Set())}
          />
          {filteredTurns.length === 0 && (
            <Text size="sm" c="dimmed" ta="center" py="xl">
              {(conv.turns ?? []).length === 0
                ? 'No LLM turns recorded — session may still be initializing or the journal was lost.'
                : 'No turns for this run.'}
            </Text>
          )}
          {filteredTurns.map((t, i) => {
            const runKey = t.runLabel ?? ''
            const showUserText = !!t.userText && t.userText !== lastUserTextByRun[runKey]
            if (t.userText) lastUserTextByRun[runKey] = t.userText
            return (
              <TurnCard
                key={`${t.runLabel ?? ''}-${t.number}-${i}`}
                turn={t}
                showUserText={showUserText}
                runColor={colorByRun[t.runLabel ?? ''] ?? 'blue'}
                showRun={multiRun}
              />
            )
          })}
        </Stack>
      </Box>
    </ScrollArea>
  )
}

// RUN_COLORS cycles distinct Mantine palette hues so interleaved runs are
// visually separable. T1 = blue (the root task), T2 = grape, etc.
const RUN_COLORS = ['blue', 'grape', 'teal', 'orange', 'cyan', 'pink', 'lime', 'violet']

function runColorMap(runs?: main.SessionJournalRun[]): Record<string, string> {
  const m: Record<string, string> = {}
  ;(runs ?? []).forEach((r, i) => { m[r.label] = RUN_COLORS[i % RUN_COLORS.length] })
  return m
}

// SessionMetaCard is the journal-context card: the run legend (now the
// interactive run filter — click a pill to toggle it), the tool list, and the
// system message. Session identity + totals (id / started / finished / model /
// turns / tokens) are owned by the SessionViewer header, so they're NOT
// repeated here. The card hides entirely when it would carry nothing.
function SessionMetaCard({
  conv,
  colorByRun,
  runFilter,
  onToggleRun,
  onClearRuns,
}: {
  conv: main.SessionJournalConversation
  colorByRun: Record<string, string>
  runFilter: Set<string>
  onToggleRun: (label: string) => void
  onClearRuns: () => void
}) {
  const runs = conv.runs ?? []
  const hasRuns = runs.length > 1
  const hasTools = !!conv.toolNames?.length
  const hasSystem = !!conv.systemMessage
  const t = useT('shell')
  if (!hasRuns && !hasTools && !hasSystem) return null
  const allActive = runFilter.size === 0
  return (
    <Card withBorder padding="md" radius="md">
      <Stack gap="xs">
        {hasRuns && (
          <Group gap={8} align="center">
            <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ letterSpacing: 0.5 }}>{t('sessionJournal.runs')}</Text>
            <Tooltip label={t('sessionJournal.showAllRuns')} withArrow openDelay={300}>
              <Badge
                size="sm"
                variant={allActive ? 'filled' : 'outline'}
                color="gray"
                onClick={onClearRuns}
                style={{ cursor: 'pointer', opacity: allActive ? 1 : 0.55 }}
              >
                {t('sessionJournal.all')}
              </Badge>
            </Tooltip>
            {runs.map((r) => {
              const active = allActive || runFilter.has(r.label)
              return (
                <Tooltip key={r.label} label={allActive ? `${r.runId} · ${t('sessionJournal.clickToFilter')}` : r.runId} withArrow openDelay={300}>
                  <Badge
                    size="sm"
                    variant={active ? 'filled' : 'outline'}
                    color={colorByRun[r.label] ?? 'blue'}
                    onClick={() => onToggleRun(r.label)}
                    style={{ cursor: 'pointer', opacity: active ? 1 : 0.55 }}
                  >
                    {r.label}{r.task ? ` · ${r.task}` : ''}{r.model ? ` · ${r.model}` : ''}
                  </Badge>
                </Tooltip>
              )
            })}
          </Group>
        )}
        {!!conv.toolNames?.length && (
          <Group gap={4}>
            <Text size="xs" c="dimmed" fw={500}>{t('sessionJournal.tools')}</Text>
            {conv.toolNames.map((name) => (
              // TODO: real description requires a Wails round-trip to the
              // agent's tool registry. v1 surfaces the bare name on hover
              // (useful when long names get truncated by the badge).
              // Future: GetAgentToolDescription(toolName) → string.
              <Tooltip key={name} label={name} withArrow openDelay={300}>
                <Badge size="xs" variant="outline" color="gray" style={{ cursor: 'help' }}>{name}</Badge>
              </Tooltip>
            ))}
          </Group>
        )}
        {conv.systemMessage && (
          <Collapsible title={t('sessionJournal.systemMessage')} defaultOpen={false}>
            <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, maxHeight: 320, overflow: 'auto' }}>
              {conv.systemMessage}
            </Code>
          </Collapsible>
        )}
      </Stack>
    </Card>
  )
}

function TurnCard({
  turn,
  showUserText = true,
  runColor = 'blue',
  showRun = false,
}: {
  turn: main.SessionJournalTurn
  showUserText?: boolean
  runColor?: string
  showRun?: boolean
}) {
  const t = useT('shell')
  return (
    <ItemCard padding="md" tone={showRun ? runColor : undefined}>
      <Stack gap="sm">
        <Group justify="space-between">
          <Group gap={6}>
            {showRun && turn.runLabel && (
              <Tooltip label={turn.runId} withArrow openDelay={300}>
                <Badge size="sm" variant="filled" color={runColor} style={{ cursor: 'help' }}>
                  {turn.runLabel}
                </Badge>
              </Tooltip>
            )}
            <Badge size="sm" variant="light" color={runColor}>{t('sessionJournal.turn', { number: turn.number })}</Badge>
          </Group>
          <Group gap={10}>
            {!!turn.durationMs && (
              <Group gap={4}>
                <IconClock size={12} color="var(--mantine-color-dimmed)" />
                <Text size="xs" c="dimmed">{formatDuration(turn.durationMs)}</Text>
              </Group>
            )}
            {(turn.inputTok || turn.outputTok) ? (
              <Text size="xs" c="dimmed" ff="monospace">
                {t('sessionJournal.tokens', { input: turn.inputTok ?? 0, output: turn.outputTok ?? 0 })}
              </Text>
            ) : null}
          </Group>
        </Group>

        {showUserText && turn.userText && (
          <Section icon={<IconUser size={14} />} label={t('sessionJournal.user')} color="teal">
            <Text size="sm" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {turn.userText}
            </Text>
          </Section>
        )}

        {turn.assistText && (
          <Section icon={<IconRobot size={14} />} label={t('sessionJournal.assistant')} color="blue">
            <Text size="sm" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {turn.assistText}
            </Text>
          </Section>
        )}

        {(turn.toolCalls ?? []).map((tc, i) => (
          <ToolCallRow key={i} call={tc} />
        ))}
      </Stack>
    </ItemCard>
  )
}

function Section({
  icon,
  label,
  color,
  children,
}: {
  icon: React.ReactNode
  label: string
  color: string
  children: React.ReactNode
}) {
  return (
    <Box>
      <Group gap={6} mb={4}>
        {icon}
        <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ letterSpacing: 0.5 }}>{label}</Text>
      </Group>
      <Box style={{
        borderLeft: `2px solid var(--mantine-color-${color}-6)`,
        paddingLeft: 8,
        marginLeft: 2,
      }}>
        {children}
      </Box>
    </Box>
  )
}

function ToolCallRow({ call }: { call: main.SessionJournalToolCall }) {
  const t = useT('shell')
  const inputPretty = useMemo(() => {
    if (!call.input) return ''
    try { return JSON.stringify(call.input, null, 2) } catch { return String(call.input) }
  }, [call.input])

  const inputSummary = useMemo(() => extractToolInputSummary(call.name, call.input), [call.name, call.input])

  return (
    <Box>
      <Group gap={6} mb={4}>
        <IconTool size={14} color={call.error ? 'var(--mantine-color-red-6)' : 'var(--mantine-color-grape-6)'} />
        <Text size="xs" c="dimmed" fw={600} tt="uppercase" style={{ letterSpacing: 0.5 }}>
          {call.error ? t('sessionJournal.toolError') : t('sessionJournal.tool')}
        </Text>
        {/* Friendly label from the tool's `display` block (e.g. "Write",
            "Ask") with the raw tool name shown in a tooltip on hover.
            Falls back to the bare name when display.label is absent. */}
        {call.label ? (
          <Tooltip label={call.name} withArrow openDelay={300}>
            <Group gap={4} style={{ cursor: 'help' }}>
              {call.icon && <Text size="sm">{call.icon}</Text>}
              <Text size="sm" fw={500}>{call.label}</Text>
            </Group>
          </Tooltip>
        ) : (
          <Text size="sm" ff="monospace" fw={500}>{call.name}</Text>
        )}
        {inputSummary && (
          <Text size="xs" c="dimmed" truncate style={{ maxWidth: 320 }}>{inputSummary}</Text>
        )}
      </Group>
      <Box style={{
        borderLeft: `2px solid ${call.error ? 'var(--mantine-color-red-6)' : 'var(--mantine-color-grape-6)'}`,
        paddingLeft: 8,
        marginLeft: 2,
      }}>
        <Stack gap={6}>
          {inputPretty && (
            <Collapsible title={t('sessionJournal.input')} defaultOpen={false}>
              <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, maxHeight: 320, overflow: 'auto' }}>
                {inputPretty}
              </Code>
            </Collapsible>
          )}
          {call.result && (
            <Collapsible title={t('sessionJournal.result')} defaultOpen={false}>
              <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, maxHeight: 320, overflow: 'auto' }}>
                {call.result}
              </Code>
            </Collapsible>
          )}
        </Stack>
      </Box>
    </Box>
  )
}

function Collapsible({
  title,
  defaultOpen,
  children,
}: {
  title: string
  defaultOpen: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  const Chev = open ? IconChevronDown : IconChevronRight
  return (
    <Box>
      <Group gap={4} onClick={() => setOpen((v) => !v)} style={{ cursor: 'pointer', userSelect: 'none' }}>
        <Chev size={12} />
        <Text size="xs" c="dimmed" fw={600}>{title}</Text>
      </Group>
      {open && <Box mt={4}>{children}</Box>}
    </Box>
  )
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

/** Mirror ktw-utils' summarizeInput heuristic: pull the first
 *  recognised string field so the tool-call header shows
 *  "file_read path/to/foo.go" without having to expand. Generic
 *  fallback returns the first string-valued field. */
function extractToolInputSummary(tool: string, input: unknown): string {
  if (!input || typeof input !== 'object') return ''
  const obj = input as Record<string, unknown>
  const named = (key: string): string => {
    const v = obj[key]
    return typeof v === 'string' ? truncate(v, 80) : ''
  }
  switch (tool) {
    case 'ask-user':              return named('question')
    case 'write_file':
    case 'file_write':
    case 'read_file':
    case 'file_read':
    case 'edit_file':
    case 'file_edit':             return named('path') || named('file_path') || named('absolute_path')
    case 'notify':                return named('message')
    case 'complete':
    case 'task_complete':         return named('summary')
    case 'call_program':          return named('program')
    case 'skills-get-instruction': return named('skill_name')
  }
  for (const v of Object.values(obj)) {
    if (typeof v === 'string') return truncate(v, 80)
  }
  return ''
}

function truncate(s: string, max: number): string {
  if (s.length <= max) return s
  return s.slice(0, max) + '…'
}
