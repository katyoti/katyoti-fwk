// SPDX-License-Identifier: Apache-2.0
/**
 * AgentContextView — "what a chat started on this object receives", drawn as a
 * UI extension. The panel is DECLARED (ktw-agents' `object-panels/agent-context.yaml`):
 * its source is the method the object-context plugin calls (AgentContext), and
 * that answer arrives here as `input` — this view calls nothing itself, so the
 * panel and the agent cannot disagree.
 *
 * Layout follows the method panels: a dimmed headline, uppercase section
 * titles, one card per system section coloured by the layer it came from
 * (farthest → closest), tools and skills as chips, and what was NOT received
 * grouped by why.
 */

import { useState } from 'react'
import {
  Badge, Box, Button, Collapse, Group, Progress, ScrollArea, Stack, Text, ThemeIcon, Title, Tooltip,
} from '@mantine/core'
import { ItemCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import {
  IconAlertTriangle, IconBook, IconChevronDown, IconChevronRight, IconCircleMinus, IconCut,
  IconEyeOff, IconFile, IconMessageChatbot, IconPlug, IconTool,
} from '@tabler/icons-react'
import { Markdown } from '../../components/Markdown'
import { useChatLauncher } from '../../hooks/useChatLauncher'
import { useT } from '../../i18n'
import { contribURI, useCT } from '../../i18n/contrib'

interface Section { id: string; title?: string; text: string; source: string; file?: string }
interface Tool { uri?: string; plugin?: string; source: string }
interface Skill { selector: string; source: string }
interface Note { list: string; key: string; source: string; kind: string; reason?: string }
interface AgentContext {
  object_id: string
  system?: Section[]
  tools?: Tool[]
  skills?: Skill[]
  notes?: Note[]
  bytes?: number
  budget?: number
}

/** The layer a source names — `capability:goals` → capability. */
const layerOf = (source: string) => source.split(':')[0]

/** One colour per layer, farthest to closest. */
const LAYER_COLOR: Record<string, string> = {
  project: 'gray', rule: 'red', extension: 'violet', capability: 'teal', type: 'blue', object: 'grape',
}
const layerColor = (source: string) => LAYER_COLOR[layerOf(source)] ?? 'gray'

/** Each kind of note, its colour and icon; its label is `agentContext.note.<kind>`. */
const NOTE_VISUAL: Record<string, { color: string; Icon: typeof IconCut }> = {
  error: { color: 'red', Icon: IconAlertTriangle },
  trimmed: { color: 'orange', Icon: IconCut },
  removed: { color: 'gray', Icon: IconCircleMinus },
  skipped: { color: 'gray', Icon: IconEyeOff },
  empty: { color: 'gray', Icon: IconFile },
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <Title order={6} mb="xs" c="dimmed" tt="uppercase" style={{ letterSpacing: 0.5 }}>{children}</Title>
  )
}

function SourceBadge({ source }: { source: string }) {
  return (
    <Badge size="xs" variant="light" color={layerColor(source)} style={{ textTransform: 'none', flexShrink: 0 }}>
      {source}
    </Badge>
  )
}

/** The URI of the contribution a section came from, when it is one the
 *  translation bundle may name: an extension's agent context
 *  (`extension:<provider>/<name>`) or a policy (`policy:<name>`). */
function sourceURI(source: string): string | undefined {
  const [layer, rest = ''] = source.split(/:(.*)/s)
  if (layer === 'extension') {
    const slash = rest.indexOf('/')
    return slash > 0 ? `${rest.slice(0, slash)}:/agent-context/${rest.slice(slash + 1)}` : contribURI('agent-context', rest)
  }
  if (layer === 'policy') return contribURI('policy', rest)
  return undefined
}

function ContextSection({ s, index }: { s: Section; index: number }) {
  const [open, setOpen] = useState(index < 3)
  const ct = useCT()
  // Its title in the GUI language: `agent.system.<id>.title` (a named file's
  // section, keyed `<capability>/<name>`: `agent.files.<name>.title`) of the
  // contribution it came from.
  const title = s.title
    ? ct(sourceURI(s.source), s.file ? `agent.files.${s.file.slice(s.file.indexOf('/') + 1)}.title` : `agent.system.${s.id}.title`, s.title)
    : ''
  const color = layerColor(s.source)
  return (
    <ItemCard tone={color}>
      <Group justify="space-between" wrap="nowrap" gap="xs" onClick={() => setOpen((o) => !o)} style={{ cursor: 'pointer' }}>
        <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }}>
          {open ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
          <Text size="sm" fw={600} truncate>{title || s.id}</Text>
          {title && <Text size="xs" c="dimmed" ff="monospace" truncate>{s.id}</Text>}
          {s.file && <Badge size="xs" variant="outline" color="gray" leftSection={<IconFile size={10} />}>{s.file}</Badge>}
        </Group>
        <SourceBadge source={s.source} />
      </Group>
      <Collapse in={open}>
        <Box mt="xs" style={{ fontSize: 'var(--mantine-font-size-sm)' }}>
          <Markdown text={s.text} />
        </Box>
      </Collapse>
    </ItemCard>
  )
}

export function AgentContextView({ input, objectId }: { input: unknown; objectId: string }) {
  const { launchTaskChat } = useChatLauncher()
  const t = useT('workspace')
  const ctx = (input ?? {}) as AgentContext
  const system = ctx.system ?? []
  const tools = ctx.tools ?? []
  const skills = ctx.skills ?? []
  const notes = ctx.notes ?? []
  const bytes = ctx.bytes ?? 0
  const budget = ctx.budget ?? 0
  const used = budget > 0 ? Math.min(100, (bytes / budget) * 100) : 0
  const layers = Array.from(new Set(system.map((s) => layerOf(s.source))))

  const byKind = notes.reduce<Record<string, Note[]>>((acc, n) => {
    (acc[n.kind] ??= []).push(n)
    return acc
  }, {})

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="lg" p="md">
        {/* Headline — what the agent gets, and how much of the budget it takes. */}
        <Group justify="space-between" wrap="nowrap" align="flex-start">
          <Box style={{ flex: 1 }}>
            <Text size="sm" c="dimmed">
              <b>{system.length}</b> {t('agentContext.sectionsFrom', { count: layers.length })}
              {' · '}<b>{tools.length}</b> {t('agentContext.toolsWord')} · <b>{skills.length}</b> {t('agentContext.skillsWord')}
              {notes.length > 0 && <> · <Text span c={byKind.error ? 'red' : 'orange'}>{t('agentContext.notReceivedCount', { count: notes.length })}</Text></>}
            </Text>
            <Tooltip label={t('agentContext.bytes', { bytes, budget })} withArrow>
              <Progress value={used} size="sm" mt={6} color={used > 90 ? 'orange' : 'teal'} />
            </Tooltip>
          </Box>
          <Button size="compact-sm" variant="light" color="violet"
            leftSection={<IconMessageChatbot size={14} />}
            onClick={() => void launchTaskChat('chat', { targetId: ctx.object_id || objectId, label: t('agentContext.chat') })}
            data-guide="object.chat-here">
            {t('agentContext.chatHere')}
          </Button>
        </Group>

        {/* The layers, in the order the agent reads them. */}
        <Group gap={6}>
          {['project', 'rule', 'extension', 'capability', 'type', 'object'].map((l) => (
            <Badge key={l} size="xs" variant={layers.includes(l) ? 'light' : 'outline'}
              color={layers.includes(l) ? LAYER_COLOR[l] : 'gray'}>{l}</Badge>
          ))}
        </Group>

        <Box>
          <SectionTitle>{t('agentContext.system', { count: system.length })}</SectionTitle>
          <Stack gap="sm">
            {system.map((s, i) => <ContextSection key={s.id} s={s} index={i} />)}
          </Stack>
        </Box>

        <Box>
          <SectionTitle>{t('agentContext.tools', { count: tools.length })}</SectionTitle>
          {tools.length === 0 ? (
            <Text size="sm" c="dimmed">{t('agentContext.noTools')}</Text>
          ) : (
            <Stack gap={6}>
              {tools.map((tl) => (
                <Group key={tl.uri || `plugin:${tl.plugin}`} gap={8} wrap="nowrap">
                  <ThemeIcon size="sm" variant="light" color={layerColor(tl.source)}>
                    {tl.plugin ? <IconPlug size={12} /> : <IconTool size={12} />}
                  </ThemeIcon>
                  <Text size="sm" ff="monospace" style={{ flex: 1, minWidth: 0 }} truncate>
                    {tl.uri || tl.plugin}
                  </Text>
                  <SourceBadge source={tl.source} />
                </Group>
              ))}
            </Stack>
          )}
        </Box>

        <Box>
          <SectionTitle>{t('agentContext.skills', { count: skills.length })}</SectionTitle>
          {skills.length === 0 ? (
            <Text size="sm" c="dimmed">{t('agentContext.none')}</Text>
          ) : (
            <Group gap={6}>
              {skills.map((s) => (
                <Tooltip key={s.selector} label={t('badges.from', { capability: s.source })} withArrow>
                  <Badge variant="light" color={layerColor(s.source)} leftSection={<IconBook size={11} />}
                    style={{ textTransform: 'none' }}>{s.selector}</Badge>
                </Tooltip>
              ))}
            </Group>
          )}
        </Box>

        {notes.length > 0 && (
          <Box>
            <SectionTitle>{t('agentContext.notReceived', { count: notes.length })}</SectionTitle>
            <Stack gap="sm">
              {Object.entries(byKind).map(([kind, list]) => {
                const v = NOTE_VISUAL[kind] ?? NOTE_VISUAL.empty
                return (
                  <Box key={kind}>
                    <Group gap={6} mb={4}>
                      <v.Icon size={14} color={`var(--mantine-color-${v.color}-6)`} />
                      <Text size="xs" fw={600} c={v.color}>{t(`agentContext.note.${NOTE_VISUAL[kind] ? kind : 'empty'}`)} ({list.length})</Text>
                    </Group>
                    <Stack gap={4} pl="md">
                      {list.map((n, i) => (
                        <Group key={`${n.list}:${n.key}:${i}`} gap={8} wrap="nowrap" align="flex-start">
                          <Badge size="xs" variant="outline" color="gray" w={56} style={{ flexShrink: 0 }}>{n.list}</Badge>
                          <Box style={{ flex: 1, minWidth: 0 }}>
                            <Text size="sm" ff="monospace" truncate>{n.key || '—'}</Text>
                            {n.reason && <Text size="xs" c="dimmed">{n.reason}</Text>}
                          </Box>
                          <SourceBadge source={n.source} />
                        </Group>
                      ))}
                    </Stack>
                  </Box>
                )
              })}
            </Stack>
          </Box>
        )}
      </Stack>
    </ScrollArea>
  )
}
