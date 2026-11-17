// SPDX-License-Identifier: Apache-2.0
/**
 * DocumentsDemoPage — every display engine the registry holds, each with a
 * sample document shown through the one entry point (DocumentView).
 *
 * The list IS the registry: a new engine (built in or contributed by a
 * sidecar) appears here without touching this page. The ui-components engine
 * has a sample per component type, with the schema inline and by id; the
 * events pane shows what a component sends back (submit, row click, action). For an engine that can
 * stream, "Replay as a stream" feeds its sample in chunks the way an LLM
 * writes it; for one that can edit, the header's Edit button is live.
 */
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { Badge, Box, Button, Group, NavLink, ScrollArea, Select, Stack, Text } from '@mantine/core'
import { IconPlayerPlay } from '@tabler/icons-react'
import { DocumentView } from '../../components/document/DocumentView'
import { displayEngines, type Document, type DisplayEngine } from '../../components/document/engines'
import { useAppStore } from '../../store'
import { SAMPLES } from './document-samples'

function useEngines(): readonly DisplayEngine[] {
  useSyncExternalStore((fn) => displayEngines.subscribe(fn), () => displayEngines.getVersion())
  return displayEngines.list()
}

/** Replays a text sample in chunks, like an LLM writing it. */
function useStreamReplay(doc: Document | undefined) {
  const [shown, setShown] = useState<Document | undefined>(doc)
  const [streaming, setStreaming] = useState(false)
  const timer = useRef<number>()
  useEffect(() => { window.clearInterval(timer.current); setShown(doc); setStreaming(false) }, [doc])
  useEffect(() => () => window.clearInterval(timer.current), [])
  const replay = () => {
    if (!doc || typeof doc.content !== 'string') return
    const full = doc.content
    const step = Math.max(8, Math.ceil(full.length / 60))
    let at = 0
    setStreaming(true)
    window.clearInterval(timer.current)
    timer.current = window.setInterval(() => {
      at = Math.min(full.length, at + step)
      setShown({ ...doc, content: full.slice(0, at) })
      if (at >= full.length) { window.clearInterval(timer.current); setStreaming(false) }
    }, 60)
  }
  return { shown, streaming, replay }
}

export function DocumentsDemoPage() {
  const engines = useEngines()
  const universe = useAppStore((s) => s.displayedUniverseName ?? undefined)
  const [selected, setSelected] = useState<string>('markdown')
  const [sampleIndex, setSampleIndex] = useState(0)
  const engine = engines.find((e) => e.id === selected) ?? engines[0]
  const samples = engine ? SAMPLES[engine.id] ?? [] : []
  const sample = useMemo(() => samples[Math.min(sampleIndex, samples.length - 1)], [samples, sampleIndex])
  const [edited, setEdited] = useState<Record<string, unknown>>({})
  const editKey = `${engine?.id}#${sampleIndex}`
  const doc = sample && edited[editKey] !== undefined ? { ...sample, content: edited[editKey] } : sample
  const { shown, streaming, replay } = useStreamReplay(doc)
  // What the component sent back — form / wizard submits, row clicks, actions.
  const [events, setEvents] = useState<string[]>([])
  const logEvent = (what: string, payload: unknown) => {
    let body = ''
    try { body = JSON.stringify(payload) } catch { body = String(payload) }
    setEvents((e) => [`${new Date().toLocaleTimeString()}  ${what}  ${body.slice(0, 300)}`, ...e].slice(0, 50))
  }

  return (
    <Group align="stretch" gap={0} wrap="nowrap" style={{ height: '100%' }}>
      <ScrollArea w={260} style={{ borderRight: '1px solid var(--mantine-color-default-border)' }}>
        <Stack gap={0} p="xs">
          <Text size="xs" c="dimmed" p="xs">Display engines ({engines.length})</Text>
          {engines.map((e) => (
            <NavLink
              key={e.id}
              active={e.id === engine?.id}
              label={e.label}
              description={e.accepts.length ? e.accepts.join(', ') : 'fallback — any other type'}
              rightSection={e.builtin ? null : <Badge size="xs" variant="outline">sidecar</Badge>}
              onClick={() => { setSelected(e.id); setSampleIndex(0) }}
            />
          ))}
        </Stack>
      </ScrollArea>
      <Stack gap={0} style={{ flex: 1, minWidth: 0 }}>
        {engine && (
          <Group gap={6} px="sm" py={6} style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Text size="sm" fw={600}>{engine.label}</Text>
            {(['stream', 'edit', 'save'] as const).map((c) => (
              <Badge key={c} size="xs" variant={engine.capabilities[c] ? 'light' : 'outline'} color={engine.capabilities[c] ? 'teal' : 'gray'}>
                {c}
              </Badge>
            ))}
            <div style={{ flex: 1 }} />
            {samples.length > 1 && (
              <Select
                size="xs"
                w={300}
                value={String(Math.min(sampleIndex, samples.length - 1))}
                onChange={(v) => setSampleIndex(Number(v ?? 0))}
                data={samples.map((d, i) => ({ value: String(i), label: d.title ?? `sample ${i + 1}` }))}
                allowDeselect={false}
                maxDropdownHeight={420}
              />
            )}
            {engine.capabilities.stream && typeof sample?.content === 'string' && (
              <Button size="compact-xs" variant="light" leftSection={<IconPlayerPlay size={12} />} onClick={replay} disabled={streaming}>
                Replay as a stream
              </Button>
            )}
          </Group>
        )}
        <Box style={{ flex: 1, minHeight: 0 }}>
          {shown ? (
            <DocumentView
              doc={shown}
              universe={universe}
              streaming={streaming}
              header
              host={{
                fillHeight: true,
                streamingComplete: !streaming,
                onSubmit: (value) => logEvent('submit', value),
                onAction: (action, args) => logEvent(`action ${action}`, args),
              }}
              onContentChange={(content) => setEdited((m) => ({ ...m, [editKey]: content }))}
            />
          ) : (
            <Text size="sm" c="dimmed" p="md">
              {engine ? `No sample for ${engine.label} — a contributed engine shows the documents its sidecar produces.` : 'No engine registered.'}
            </Text>
          )}
        </Box>
        <Box style={{ height: 110, flexShrink: 0, borderTop: '1px solid var(--mantine-color-default-border)' }}>
          <Group gap={6} px="sm" py={4}>
            <Text size="xs" fw={600}>Events</Text>
            <Text size="xs" c="dimmed">what the component sent back (submit, row click, action)</Text>
            <div style={{ flex: 1 }} />
            {events.length > 0 && <Button size="compact-xs" variant="subtle" onClick={() => setEvents([])}>Clear</Button>}
          </Group>
          <ScrollArea style={{ height: 80 }} px="sm">
            {events.length === 0
              ? <Text size="xs" c="dimmed">No event yet — submit a form or click a table row.</Text>
              : events.map((e, i) => <Text key={i} size="xs" ff="monospace" style={{ whiteSpace: 'pre' }}>{e}</Text>)}
          </ScrollArea>
        </Box>
      </Stack>
    </Group>
  )
}
