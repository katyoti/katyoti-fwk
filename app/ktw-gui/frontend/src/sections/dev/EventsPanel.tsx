// SPDX-License-Identifier: Apache-2.0
/**
 * EventsPanel — the debugger's Events tab: every declared
 * event type, grouped by the pack that declares it, and for the selected one
 * who subscribes to it in every layer (CWM's EventSubscriptions through
 * ktw-agents:reactions/events) — whether each applies and why not, its open
 * reactions — and, for the events pack's own types, the source's state (the
 * git HEAD last seen, the timers' last firings). The label and description
 * come from the event-type contribution's data layer (the catalog's index is
 * body-free), read when a type is selected.
 */
import { useEffect, useMemo, useState } from 'react'
import { Alert, Badge, Box, Code, Group, Loader, ScrollArea, Stack, Table, Text, TextInput, Title, Tooltip, UnstyledButton } from '@mantine/core'
import { Split } from '@gfazioli/mantine-split-pane'
import { IconSearch } from '@tabler/icons-react'

import { useAppStore } from '../../store'
import { ListContributions, OpenContributionLayer, WorkspaceRunAction } from '../../../wailsjs/go/main/App'

const EVENTS_READ = 'ktw-agents:reactions/events'

interface AsyncSpec {
  priority?: string
  key?: string
  attempts?: number
  accepts_caused?: boolean
  atomic?: boolean
}

interface Subscription {
  subscription: string
  source: string
  provider?: string
  declared_on: string
  uri: string
  when?: string
  async: AsyncSpec
  on?: { module: string; event: string }
  writes?: string[]
  applies: boolean
  reason?: string
}

interface SourceState {
  repo?: string
  head?: string
  timers?: Record<string, string>
}

interface EventView {
  event: string
  point?: string
  published: boolean
  provider?: string
  payload?: string
  reason?: string
  subscriptions: Subscription[]
  open?: Record<string, number>
  source?: SourceState
}

interface Described {
  label?: string
  description?: string
}

// The layer a subscription is declared in, as a short tag.
function layerOf(source: string): string {
  const i = source.indexOf(':')
  return i < 0 ? source : source.slice(0, i)
}

export function EventsPanel() {
  const universeName = useAppStore((s) => s.displayedUniverseName)
  const [events, setEvents]   = useState<EventView[]>([])
  const [uris, setURIs]       = useState<Record<string, string>>({})
  const [described, setDescribed] = useState<Record<string, Described>>({})
  const [loading, setLoading] = useState(false)
  const [error, setError]     = useState<string | null>(null)
  const [query, setQuery]     = useState('')
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    if (!universeName) {
      setEvents([])
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    WorkspaceRunAction(universeName, EVENTS_READ, '{}')
      .then((r) => {
        if (cancelled) return
        if (r.error || !r.value) {
          setError(r.error || 'No answer from ' + EVENTS_READ)
          setEvents([])
        } else {
          const out: { events?: EventView[] } = JSON.parse(r.value)
          // A type nobody subscribes to may come without its list.
          setEvents((out.events ?? []).map((e) => ({ ...e, subscriptions: e.subscriptions ?? [] })))
        }
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    // Each type's contribution URI, to read its label and description from.
    ListContributions(universeName)
      .then((r) => {
        if (cancelled || r.error) return
        const m: Record<string, string> = {}
        for (const c of r.contributions ?? []) {
          if (c.type === 'event-type') m[c.id] = c.uri
        }
        setURIs(m)
      })
      .catch(() => { /* the catalog unreachable: names only */ })
    return () => { cancelled = true }
  }, [universeName])

  // Grouped by the declaring pack (CWM's own first), filtered by the query.
  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    const by = new Map<string, EventView[]>()
    for (const e of events) {
      if (q && !e.event.toLowerCase().includes(q) && !(e.provider ?? '').toLowerCase().includes(q)) continue
      const p = e.provider || 'cwm'
      if (!by.has(p)) by.set(p, [])
      by.get(p)!.push(e)
    }
    return [...by.entries()].sort(([a], [b]) => (a === 'cwm' ? -1 : b === 'cwm' ? 1 : a.localeCompare(b)))
  }, [events, query])

  const current = events.find((e) => e.event === selected) ?? null

  // The selected type's label and description, from its data layer, once.
  useEffect(() => {
    if (!universeName || !selected || described[selected] || !uris[selected]) return
    let cancelled = false
    OpenContributionLayer(universeName, uris[selected], 'data')
      .then((l) => {
        if (cancelled || !l.content) return
        const body: Described = JSON.parse(l.content)
        setDescribed((d) => ({ ...d, [selected]: { label: body.label, description: body.description } }))
      })
      .catch(() => { /* no detail: the name only */ })
    return () => { cancelled = true }
  }, [universeName, selected, uris, described])

  if (!universeName) {
    return <Text p="md" c="dimmed">Open a universe to see its events.</Text>
  }

  return (
    <Split style={{ height: '100%' }}>
      <Split.Pane initialWidth={300} minWidth={220} maxWidth={480}>
        <Stack gap="xs" p="xs" style={{ height: '100%' }}>
          <TextInput
            size="xs"
            placeholder="Filter event types"
            leftSection={<IconSearch size={14} />}
            value={query}
            onChange={(e) => setQuery(e.currentTarget.value)}
          />
          {loading && <Loader size="sm" />}
          {error && <Alert color="red" title="Events unavailable">{error}</Alert>}
          <ScrollArea style={{ flex: 1 }}>
            {groups.map(([provider, list]) => (
              <Box key={provider} mb="sm">
                <Text size="xs" fw={600} c="dimmed" tt="uppercase">{provider}</Text>
                {list.map((e) => {
                  const live = e.subscriptions.filter((s) => s.applies).length
                  return (
                    <UnstyledButton
                      key={e.event}
                      onClick={() => setSelected(e.event)}
                      style={{
                        display: 'block', width: '100%', padding: '4px 6px', borderRadius: 4,
                        background: e.event === selected ? 'var(--mantine-color-blue-light)' : undefined,
                      }}
                    >
                      <Group gap={6} wrap="nowrap" justify="space-between">
                        <Text size="sm" truncate>{e.event}</Text>
                        <Group gap={4} wrap="nowrap">
                          {e.published && <Badge size="xs" color="teal" variant="light">published</Badge>}
                          {!e.point && <Badge size="xs" color="gray" variant="light">not subscribable</Badge>}
                          <Badge size="xs" color={live > 0 ? 'blue' : 'gray'} variant="outline">{live}</Badge>
                        </Group>
                      </Group>
                    </UnstyledButton>
                  )
                })}
              </Box>
            ))}
            {!loading && events.length === 0 && !error && <Text size="sm" c="dimmed">No event type declared.</Text>}
          </ScrollArea>
        </Stack>
      </Split.Pane>
      <Split.Resizer />
      <Split.Pane grow>
        <ScrollArea style={{ height: '100%' }}>
          {current ? <EventDetail e={current} d={described[current.event]} /> : (
            <Text p="md" c="dimmed">Select an event type to see who subscribes to it.</Text>
          )}
        </ScrollArea>
      </Split.Pane>
    </Split>
  )
}

function EventDetail({ e, d }: { e: EventView; d?: Described }) {
  return (
    <Stack gap="sm" p="md">
      <Group gap="xs">
        <Title order={4}>{e.event}</Title>
        {d?.label && d.label !== e.event && <Text c="dimmed">{d.label}</Text>}
        {e.published ? <Badge color="teal" variant="light">published</Badge> : <Badge color="gray" variant="light">private</Badge>}
      </Group>
      {d?.description && <Text size="sm">{d.description}</Text>}
      <Group gap="lg">
        <Text size="sm"><Text span c="dimmed">Point </Text>{e.point ? <Code>{e.point}</Code> : <Text span c="dimmed">none — {e.reason || 'not subscribable'}</Text>}</Text>
        <Text size="sm"><Text span c="dimmed">Declared by </Text>{e.provider || 'CWM'}</Text>
        {e.payload && <Text size="sm"><Text span c="dimmed">Payload </Text><Code>{e.payload}</Code></Text>}
      </Group>

      <Title order={5}>Subscribers ({e.subscriptions.length})</Title>
      {e.subscriptions.length === 0 ? (
        <Text size="sm" c="dimmed">Nobody subscribes to it.</Text>
      ) : (
        <Table striped highlightOnHover withTableBorder fz="xs">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Layer</Table.Th>
              <Table.Th>Declared on</Table.Th>
              <Table.Th>Action</Table.Th>
              <Table.Th>Spec</Table.Th>
              <Table.Th>Writes</Table.Th>
              <Table.Th>Open</Table.Th>
              <Table.Th>Applies</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {e.subscriptions.map((s) => (
              <Table.Tr key={s.subscription}>
                <Table.Td><Badge size="xs" variant="light">{layerOf(s.source)}</Badge></Table.Td>
                <Table.Td>
                  <Text size="xs">{s.declared_on}</Text>
                  {s.on && <Text size="xs" c="dimmed">on {s.on.module} · {s.on.event}</Text>}
                </Table.Td>
                <Table.Td>
                  <Code fz="xs">{s.uri}</Code>
                  {s.when && <Text size="xs" c="dimmed">when {s.when}</Text>}
                </Table.Td>
                <Table.Td>
                  <Text size="xs">{s.async.priority ?? ''} · {s.async.attempts ?? ''} attempt(s)</Text>
                  <Group gap={4}>
                    {s.async.atomic && <Badge size="xs" color="grape" variant="light">atomic</Badge>}
                    {s.async.accepts_caused && <Badge size="xs" color="orange" variant="light">accepts caused</Badge>}
                  </Group>
                </Table.Td>
                <Table.Td><Text size="xs">{(s.writes ?? []).join(', ')}</Text></Table.Td>
                <Table.Td><Text size="xs">{e.open?.[s.subscription] ?? 0}</Text></Table.Td>
                <Table.Td>
                  <Tooltip label={s.reason || ''} disabled={!s.reason} multiline w={320}>
                    <Badge size="xs" color={s.applies ? 'green' : 'red'} variant="light">{s.applies ? 'applies' : 'never'}</Badge>
                  </Tooltip>
                  {s.reason && <Text size="xs" c="dimmed">{s.reason}</Text>}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}

      {e.source && (
        <>
          <Title order={5}>Source</Title>
          {e.source.head && (
            <Text size="sm"><Text span c="dimmed">Git HEAD last seen </Text><Code>{e.source.head.slice(0, 12)}</Code> <Text span c="dimmed">in</Text> {e.source.repo}</Text>
          )}
          {e.source.timers && (
            <Table withTableBorder fz="xs">
              <Table.Thead>
                <Table.Tr><Table.Th>Timer</Table.Th><Table.Th>Last firing</Table.Th></Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {Object.entries(e.source.timers).map(([k, v]) => (
                  <Table.Tr key={k}><Table.Td>{k}</Table.Td><Table.Td>{v}</Table.Td></Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          )}
        </>
      )}
    </Stack>
  )
}
