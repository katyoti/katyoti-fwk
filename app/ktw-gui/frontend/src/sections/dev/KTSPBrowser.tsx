// SPDX-License-Identifier: Apache-2.0
/**
 * KTSPBrowser — debug-time inspector + invoker for every KTSP slot
 * advertised by the active universe (the GUI-side equivalent of the
 * `ktw-utils inspect` CLI tool).
 *
 * Layout: Split — left pane is the service tree (default "By Service"
 * grouping, optional "By Sidecar" toggle), right pane is the per-method
 * invocation card (slot path, description, input/output schemas, JSON
 * editor, response viewer).
 *
 * Data source: Wails `ListKTSPManifests(universe)` → one round-trip to
 * `ktw-hub:/CatalogService/ListManifests`. Manifests are read from the
 * hub's hydrated inventory cache; no per-sidecar spawn.
 *
 * Invocation: Wails `CallKTSPMethod(universe, slotPath, inputJSON)` →
 * pure pass-through to the substrate `Send(slotPath, decodedInput)`.
 * The slot path is constructed from the manifest as
 * `<provider>:<slot.Name>/<method.Name>` (the canonical wire form auto-KTSP
 * publishes).
 *
 * "Schema driven" today = the user sees the input/output JSON Schemas
 * inline above the editor and writes JSON that matches them. A future
 * iteration plugs a JSON-Schema → DSL-AST converter so the input form
 * becomes a real FormWidgetPanel form; we leave a marker comment at the
 * editor.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  ActionIcon,
  Alert,
  Badge,
  Box,
  Button,
  Card,
  Center,
  Code,
  CopyButton,
  Group,
  Loader,
  ScrollArea,
  SegmentedControl,
  Stack,
  Text,
  Textarea,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { Split } from '@gfazioli/mantine-split-pane'
import {
  IconAlertCircle,
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconCopy,
  IconLayersIntersect,
  IconPlayerPlay,
  IconRefresh,
  IconSearch,
  IconServer2,
} from '@tabler/icons-react'

import { useAppStore } from '../../store'
import { ListKTSPManifests, CallKTSPMethod } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'

type Grouping = 'service' | 'sidecar'

/** Tree leaf — one (provider, slot, method) addressable as
 *  `<provider>:<slot>/<method>`. */
interface MethodEntry {
  provider:     string
  slotName:     string
  slotType:     string
  slotDescription: string
  method:       main.KTSPMethodSummary
  /** Canonical KTSP wire address. */
  slotPath:     string
}

/** A grouped tree node — its label + the methods (and groupings) it owns. */
interface TreeGroup {
  id:       string
  label:    string
  hint?:    string
  children: TreeGroup[]
  methods:  MethodEntry[]
}

export function KTSPBrowser() {
  const universeName = useAppStore((s) => s.displayedUniverseName)

  const [manifests, setManifests] = useState<main.KTSPManifestSummary[] | null>(null)
  const [loading,   setLoading]   = useState(false)
  const [error,     setError]     = useState<string | null>(null)
  const [unavailable, setUnavailable] = useState<string[]>([])
  const [grouping, setGrouping]  = useState<Grouping>('service')
  const [selected,  setSelected]  = useState<MethodEntry | null>(null)
  const [refreshTick, setRefreshTick] = useState(0)
  const [filterText,  setFilterText]  = useState('')

  // Fetch the manifest list whenever the displayed universe changes or
  // the user hits Refresh. The hub call is in-memory on its side so
  // this is cheap.
  useEffect(() => {
    if (!universeName) {
      setManifests(null)
      setSelected(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    setUnavailable([])
    ListKTSPManifests(universeName)
      .then((r) => {
        if (cancelled) return
        setUnavailable(r.unavailable ?? [])
        if (r.error) {
          setError(r.error)
          setManifests(null)
        } else {
          setManifests(r.manifests ?? [])
        }
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, refreshTick])

  // Flatten manifests → method entries (one per provider/slot/method).
  const allMethods: MethodEntry[] = useMemo(() => {
    if (!manifests) return []
    const out: MethodEntry[] = []
    for (const m of manifests) {
      for (const s of m.slots ?? []) {
        for (const sm of s.methods ?? []) {
          out.push({
            provider:         m.provider,
            slotName:         s.name,
            slotType:         s.type,
            slotDescription:  s.description ?? '',
            method:           sm,
            slotPath:         `${m.provider}:${s.name}/${sm.name}`,
          })
        }
      }
    }
    return out
  }, [manifests])

  // Filter methods by a case-insensitive substring against
  // provider / slot / method name / slot path / description. Empty
  // filter passes everything through.
  const filteredMethods: MethodEntry[] = useMemo(() => {
    const q = filterText.trim().toLowerCase()
    if (!q) return allMethods
    return allMethods.filter((m) =>
      m.provider.toLowerCase().includes(q)
      || m.slotName.toLowerCase().includes(q)
      || m.slotType.toLowerCase().includes(q)
      || m.method.name.toLowerCase().includes(q)
      || m.slotPath.toLowerCase().includes(q)
      || (m.method.description ?? '').toLowerCase().includes(q)
      || m.slotDescription.toLowerCase().includes(q),
    )
  }, [allMethods, filterText])

  // Build the tree according to the grouping choice.
  const tree: TreeGroup[] = useMemo(() => {
    if (grouping === 'sidecar') return groupBySidecar(filteredMethods)
    return groupByService(filteredMethods)
  }, [filteredMethods, grouping])

  return (
    <Split style={{ height: '100%' }}>
      <Split.Pane initialWidth={320} minWidth={220} maxWidth={520}>
        <Box style={{ height: '100%', display: 'flex', flexDirection: 'column', borderRight: '1px solid var(--mantine-color-default-border)' }}>
          <Group gap="xs" px="sm" py="xs" justify="space-between" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Text size="sm" fw={600}>KTSP catalog</Text>
            <Tooltip label="Refresh">
              <ActionIcon size="sm" variant="subtle" onClick={() => setRefreshTick((n) => n + 1)} disabled={loading}>
                <IconRefresh size={14} />
              </ActionIcon>
            </Tooltip>
          </Group>

          <Stack
            gap="xs"
            px="sm"
            py="xs"
            style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}
          >
            <SegmentedControl
              fullWidth
              size="xs"
              value={grouping}
              onChange={(v) => setGrouping(v as Grouping)}
              data={[
                { value: 'service', label: 'By Service' },
                { value: 'sidecar', label: 'By Sidecar' },
              ]}
            />
            <TextInput
              size="xs"
              placeholder="Search provider, slot, method…"
              leftSection={<IconSearch size={12} />}
              value={filterText}
              onChange={(e) => setFilterText(e.currentTarget.value)}
            />
          </Stack>

          <Box style={{ flex: 1, overflow: 'auto' }}>
            {!universeName && (
              <Box p="md"><Text size="sm" c="dimmed">No universe is active. Activate one from the ribbon to inspect its KTSP catalog.</Text></Box>
            )}
            {universeName && loading && (
              <Center py="xl"><Loader size="sm" /></Center>
            )}
            {error && !loading && (
              <Box p="sm">
                <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Catalog unavailable">{error}</Alert>
              </Box>
            )}
            {!loading && !error && unavailable.length > 0 && (
              <Box p="sm">
                <Alert color="orange" icon={<IconAlertCircle size={14} />} variant="light" title="Members without a manifest">
                  {unavailable.map((u) => <Text key={u} size="xs">{u}</Text>)}
                </Alert>
              </Box>
            )}
            {universeName && !loading && !error && manifests && manifests.length === 0 && (
              <Box p="md"><Text size="sm" c="dimmed">Hub catalog is empty. Run <Code>ktw-hub catalog refresh</Code> to populate it.</Text></Box>
            )}
            {!loading && !error && tree.length > 0 && (
              <Stack gap={0} px="xs" py="xs">
                {tree.map((g) => (
                  <TreeNode
                    key={g.id}
                    node={g}
                    depth={0}
                    selectedSlotPath={selected?.slotPath ?? null}
                    onSelect={setSelected}
                  />
                ))}
              </Stack>
            )}
          </Box>
        </Box>
      </Split.Pane>

      <Split.Resizer />

      <Split.Pane grow>
        <KTSPInvokePanel universe={universeName} entry={selected} />
      </Split.Pane>
    </Split>
  )
}

// ── Tree rendering ─────────────────────────────────────────────────

function TreeNode({
  node,
  depth,
  selectedSlotPath,
  onSelect,
}: {
  node: TreeGroup
  depth: number
  selectedSlotPath: string | null
  onSelect: (m: MethodEntry) => void
}) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = node.children.length > 0 || node.methods.length > 0
  const Chev = open ? IconChevronDown : IconChevronRight
  return (
    <Box>
      <Group
        gap={4}
        wrap="nowrap"
        onClick={() => setOpen((v) => !v)}
        style={{ cursor: 'pointer', padding: '4px 6px', paddingLeft: 6 + depth * 10, userSelect: 'none' }}
      >
        {hasChildren ? <Chev size={12} /> : <Box style={{ width: 12 }} />}
        <Text size="sm" fw={500} truncate>{node.label}</Text>
        {node.hint && <Badge size="xs" variant="light" color="gray">{node.hint}</Badge>}
      </Group>
      {open && (
        <Box>
          {node.children.map((c) => (
            <TreeNode key={c.id} node={c} depth={depth + 1} selectedSlotPath={selectedSlotPath} onSelect={onSelect} />
          ))}
          {node.methods.map((m) => {
            const active = m.slotPath === selectedSlotPath
            return (
              <Box
                key={m.slotPath}
                onClick={() => onSelect(m)}
                style={{
                  cursor: 'pointer',
                  padding: '4px 6px',
                  paddingLeft: 6 + (depth + 1) * 10 + 16,
                  borderRadius: 4,
                  background: active ? 'var(--mantine-color-blue-light)' : 'transparent',
                  color: active ? 'var(--mantine-color-blue-light-color)' : 'inherit',
                }}
                onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = 'var(--mantine-color-default-hover)' }}
                onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent' }}
              >
                <Group gap={6} wrap="nowrap">
                  <Text size="sm" ff="monospace" truncate>{m.method.name}</Text>
                </Group>
                {m.method.description && (
                  <Text size="xs" c="dimmed" truncate>{m.method.description}</Text>
                )}
              </Box>
            )
          })}
        </Box>
      )}
    </Box>
  )
}

// ── Right pane: invoke + response ─────────────────────────────────

function KTSPInvokePanel({
  universe,
  entry,
}: {
  universe: string | null
  entry: MethodEntry | null
}) {
  const [inputJSON, setInputJSON] = useState('{}')
  const [calling,   setCalling]   = useState(false)
  const [response,  setResponse]  = useState<string | null>(null)
  const [callError, setCallError] = useState<string | null>(null)

  // Reset the editor whenever the selection changes — start from a
  // template that matches the input schema's top-level shape so the
  // user has a working scaffold to edit.
  useEffect(() => {
    if (!entry) return
    setInputJSON(schemaToTemplate(entry.method.inputSchema))
    setResponse(null)
    setCallError(null)
  }, [entry?.slotPath])

  // Hooks-rules: every hook MUST run on every render of this component,
  // so this useMemo lives ABOVE the `if (!entry) return ...` early-out.
  // Returning null when there is no response is fine; what's NOT fine is
  // skipping a hook call between renders (React error #310).
  const responseStats = useMemo(() => {
    if (response === null) return null
    const bytes = response.length
    let parsed: unknown
    try { parsed = JSON.parse(response) } catch { /* keep raw */ }
    if (parsed && typeof parsed === 'object') {
      if (Array.isArray(parsed)) return { bytes, kind: 'array' as const, count: parsed.length }
      const keys = Object.keys(parsed as Record<string, unknown>)
      const counts = keys.map((k) => {
        const v = (parsed as Record<string, unknown>)[k]
        if (Array.isArray(v))            return `${k}: ${v.length} item${v.length === 1 ? '' : 's'}`
        if (v === null)                  return `${k}: null`
        if (typeof v === 'object')       return `${k}: object`
        return `${k}: ${typeof v}`
      })
      return { bytes, kind: 'object' as const, keys: counts }
    }
    return { bytes, kind: typeof parsed }
  }, [response])

  if (!entry) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap={4}>
          <IconLayersIntersect size={32} color="var(--mantine-color-dimmed)" />
          <Text size="sm" c="dimmed" fw={500}>No method selected</Text>
          <Text size="xs" c="dimmed">Pick a method from the catalog on the left.</Text>
        </Stack>
      </Center>
    )
  }

  const handleCall = async () => {
    if (!universe) return
    setCalling(true)
    setCallError(null)
    setResponse(null)
    try {
      // eslint-disable-next-line no-console
      console.log('[KTSPBrowser] →', entry.slotPath, JSON.parse(inputJSON || 'null'))
      const r = await CallKTSPMethod(universe, entry.slotPath, inputJSON)
      // eslint-disable-next-line no-console
      console.log('[KTSPBrowser] ←', entry.slotPath, r)
      if (r.error) setCallError(r.error)
      else setResponse(prettyJSON(r.response))
    } catch (e) {
      setCallError(String(e))
    } finally {
      setCalling(false)
    }
  }

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">
        <Stack gap="md">

          {/* Header card */}
          <Card withBorder padding="md" radius="md">
            <Stack gap={6}>
              <Group justify="space-between" align="flex-start">
                <Stack gap={2}>
                  <Title order={4} style={{ fontFamily: 'monospace' }}>{entry.slotPath}</Title>
                  {entry.method.description && (
                    <Text size="sm" c="dimmed">{entry.method.description}</Text>
                  )}
                </Stack>
                <Group gap={6}>
                  <Badge size="sm" variant="light">{entry.slotType}</Badge>
                  {entry.method.tags?.map((t) => (
                    <Badge key={t} size="sm" variant="light" color="gray">{t}</Badge>
                  ))}
                </Group>
              </Group>
              <Group gap={6}>
                <Text size="xs" c="dimmed">Provider: <Code>{entry.provider}</Code></Text>
                <Text size="xs" c="dimmed">Slot: <Code>{entry.slotName}</Code></Text>
              </Group>
              {entry.slotDescription && (
                <Text size="xs" c="dimmed">Slot description: {entry.slotDescription}</Text>
              )}
            </Stack>
          </Card>

          {/* Input schema reference + editor */}
          <Card withBorder padding="md" radius="md">
            <Stack gap="sm">
              <Group justify="space-between">
                <Text size="sm" fw={600}>Input</Text>
                <CopyButton value={inputJSON} timeout={1200}>
                  {({ copied, copy }) => (
                    <Tooltip label={copied ? 'Copied' : 'Copy input JSON'}>
                      <ActionIcon size="sm" variant="subtle" onClick={copy}>
                        {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                      </ActionIcon>
                    </Tooltip>
                  )}
                </CopyButton>
              </Group>
              {/* TODO(schema-driven-ui-v2): replace this JSON editor with
                  the FormWidgetPanel form once we add a JSON-Schema → DSL
                  AST converter to the schema service. The KTSP wire
                  contract only exposes JSON Schema today (auto-derived
                  from Go input types), but FormWidgetPanel expects the
                  canonical AST. */}
              <Textarea
                value={inputJSON}
                onChange={(e) => setInputJSON(e.currentTarget.value)}
                autosize
                minRows={6}
                maxRows={24}
                styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
                disabled={calling}
              />
              <SchemaPreview title="Input schema" schema={entry.method.inputSchema} />
              <Group justify="flex-end">
                <Button
                  leftSection={<IconPlayerPlay size={14} />}
                  onClick={handleCall}
                  loading={calling}
                  disabled={!universe}
                >
                  Call
                </Button>
              </Group>
            </Stack>
          </Card>

          {/* Response */}
          {callError && (
            <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Call failed">
              <Code block style={{ whiteSpace: 'pre-wrap' }}>{callError}</Code>
            </Alert>
          )}

          {response !== null && !callError && (
            <Card withBorder padding="md" radius="md">
              <Stack gap="sm">
                <Group justify="space-between">
                  <Group gap={6}>
                    <IconServer2 size={14} />
                    <Text size="sm" fw={600}>Response</Text>
                    {responseStats && (
                      <Badge size="xs" variant="light" color="gray">
                        {responseStats.bytes} B · {responseStats.kind}
                        {responseStats.kind === 'array' && ` · ${responseStats.count}`}
                      </Badge>
                    )}
                  </Group>
                  <CopyButton value={response} timeout={1200}>
                    {({ copied, copy }) => (
                      <Tooltip label={copied ? 'Copied' : 'Copy response'}>
                        <ActionIcon size="sm" variant="subtle" onClick={copy}>
                          {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                        </ActionIcon>
                      </Tooltip>
                    )}
                  </CopyButton>
                </Group>
                {responseStats && responseStats.kind === 'object' && responseStats.keys && (
                  <Group gap={6}>
                    {responseStats.keys.map((k) => (
                      <Badge key={k} size="xs" variant="outline" color="gray">{k}</Badge>
                    ))}
                  </Group>
                )}
                <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 12, maxHeight: 480, overflow: 'auto' }}>{response}</Code>
              </Stack>
            </Card>
          )}

          <SchemaPreview title="Output schema" schema={entry.method.outputSchema} />

        </Stack>
      </Box>
    </ScrollArea>
  )
}

// ── Helpers ────────────────────────────────────────────────────────

function groupByService(entries: MethodEntry[]): TreeGroup[] {
  // Top: SlotType. Children: one node per (provider, slot.Name)
  // instance. Methods hang off the instance node.
  const byType = new Map<string, MethodEntry[]>()
  for (const e of entries) {
    const arr = byType.get(e.slotType) ?? []
    arr.push(e)
    byType.set(e.slotType, arr)
  }
  const out: TreeGroup[] = []
  for (const [type, list] of [...byType.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    const byInstance = new Map<string, MethodEntry[]>()
    for (const e of list) {
      const key = `${e.provider}:${e.slotName}`
      const arr = byInstance.get(key) ?? []
      arr.push(e)
      byInstance.set(key, arr)
    }
    const children: TreeGroup[] = []
    for (const [key, methods] of [...byInstance.entries()].sort(([a], [b]) => a.localeCompare(b))) {
      children.push({
        id:       `service:${type}:${key}`,
        label:    key,
        hint:     `${methods.length}`,
        children: [],
        methods,
      })
    }
    out.push({
      id:       `service:${type}`,
      label:    type,
      hint:     `${list.length}`,
      children,
      methods:  [],
    })
  }
  return out
}

function groupBySidecar(entries: MethodEntry[]): TreeGroup[] {
  // Top: provider. Children: one node per slot. Methods hang off slot.
  const byProvider = new Map<string, MethodEntry[]>()
  for (const e of entries) {
    const arr = byProvider.get(e.provider) ?? []
    arr.push(e)
    byProvider.set(e.provider, arr)
  }
  const out: TreeGroup[] = []
  for (const [provider, list] of [...byProvider.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    const bySlot = new Map<string, MethodEntry[]>()
    for (const e of list) {
      const arr = bySlot.get(e.slotName) ?? []
      arr.push(e)
      bySlot.set(e.slotName, arr)
    }
    const children: TreeGroup[] = []
    for (const [slot, methods] of [...bySlot.entries()].sort(([a], [b]) => a.localeCompare(b))) {
      children.push({
        id:       `sidecar:${provider}:${slot}`,
        label:    slot,
        hint:     methods[0]?.slotType,
        children: [],
        methods,
      })
    }
    out.push({
      id:       `sidecar:${provider}`,
      label:    provider,
      hint:     `${list.length}`,
      children,
      methods:  [],
    })
  }
  return out
}

function SchemaPreview({ title, schema }: { title: string; schema?: unknown }) {
  const [open, setOpen] = useState(false)
  if (!schema) {
    return (
      <Group gap={6}>
        <Text size="xs" c="dimmed">{title}: <em>none advertised</em></Text>
      </Group>
    )
  }
  return (
    <Box>
      <Group gap={4} onClick={() => setOpen((v) => !v)} style={{ cursor: 'pointer', userSelect: 'none' }}>
        {open ? <IconChevronDown size={12} /> : <IconChevronRight size={12} />}
        <Text size="xs" c="dimmed" fw={500}>{title}</Text>
      </Group>
      {open && (
        <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, marginTop: 4, maxHeight: 280, overflow: 'auto' }}>
          {prettyJSON(schema)}
        </Code>
      )}
    </Box>
  )
}

function schemaToTemplate(schema?: unknown): string {
  // Best-effort: if the schema has a top-level `properties` object,
  // emit `{key: <type-default>}` for every property. Otherwise just
  // give the user `{}` so they can write whatever they need.
  if (!schema || typeof schema !== 'object') return '{}'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const s = schema as any
  const props = s.properties as Record<string, { type?: string; default?: unknown }> | undefined
  if (!props) return '{}'
  const obj: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(props)) {
    if (v && Object.prototype.hasOwnProperty.call(v, 'default')) {
      obj[k] = v.default
      continue
    }
    switch (v?.type) {
      case 'string':   obj[k] = '';   break
      case 'integer':  obj[k] = 0;    break
      case 'number':   obj[k] = 0;    break
      case 'boolean':  obj[k] = false; break
      case 'array':    obj[k] = [];   break
      case 'object':   obj[k] = {};   break
      default:         obj[k] = null
    }
  }
  return JSON.stringify(obj, null, 2)
}

function prettyJSON(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}
