/**
 * TypeBrowserPanel — debug-time discoverability surface for every
 * data-type Contribution registered in the active universe's catalog
 * (ms-schema-driven-ui US-S10, v1 scope).
 *
 * Left pane: tree of types grouped by source sidecar (provider). Each
 * leaf shows the type's logical id (e.g. `task/hello-world/input`).
 * A flat search box at the top filters the tree across name + URI.
 *
 * Right pane: when a type is selected,
 *  - Header card: URI, source sidecar, format, optional refresh.
 *  - **Types** card: every named type in the schema, with its primitive
 *    kind, label/description, and (where applicable) enum / fields /
 *    content. Rendered as a stack of mini-cards so the user can see
 *    the data shape at a glance.
 *  - **Extensions** card: every `ui-*` extension declared on each
 *    type, grouped by `(type, name)` — that's the address the
 *    GetSchemaAST filter accepts.
 *  - **Raw source** card: the embedded DSL bytes, lazy-collapsible.
 *
 * Deferred (full US-S10 spec): "+ create instance" wiring (needs US-S4
 * generic Form widget), CWM node-type/item-type/edge-type kinds.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Button,
  Card,
  Center,
  Code,
  Group,
  Loader,
  Modal,
  ScrollArea,
  SegmentedControl,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { Split } from '@gfazioli/mantine-split-pane'
import {
  IconAlertCircle,
  IconBraces,
  IconChevronDown,
  IconChevronRight,
  IconPlayerPlay,
  IconPlus,
  IconPuzzle,
  IconSearch,
  IconShape,
} from '@tabler/icons-react'

import { useAppStore } from '../../store'
import { ListDataTypes, GetSchemaAST } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { FormWidgetPanel } from '../agent/FormWidgetPanel'
import { ViewWidgetPanel } from '../agent/ViewWidgetPanel'
import { TableWidgetPanel } from '../agent/TableWidgetPanel'

// 'provider' → tree grouped by source sidecar (the canonical view).
// 'name'     → flat alphabetical list of every data-type's FULL URI;
//              no tree, no grouping — just every entry as one row,
//              sortable by URI for ctrl-F navigation. Replaces the
//              former 'kind' grouping (split id on '/'), which always
//              clustered everything under one or two pseudo-buckets
//              like 'task' / 'skill-body' and added no signal.
type GroupBy = 'provider' | 'name'

// DedupedEntry wraps a DataTypeSummary with the dedup metadata the
// 'By Name' view consumes (cwm-v2-providers US-15, 2026-06-08).
interface DedupedEntry {
  entry: main.DataTypeSummary
  contributors: string[]      // every provider that publishes this URI
  duplicateNonSystem: boolean  // true when 2+ non-system providers collide on the same URI (a bug)
}

interface TreeNode {
  id:        string
  label:     string
  hint?:     string
  children:  TreeNode[]
  entries:   main.DataTypeSummary[]
}

interface DataTypeWithSchema {
  summary: main.DataTypeSummary
  schema?: SchemaAST
}

interface SchemaAST {
  name?:        string
  root?:        string
  description?: string
  types?: Record<string, SchemaType>
}

interface SchemaType {
  type?:         string
  label?:        string
  description?:  string
  content?:      unknown
  enum?:         unknown[]
  default?:      unknown
  optional?:     boolean
  fields?:       Record<string, SchemaType>
  items?:        unknown
  extensions?:   SchemaExtension[]
}

interface SchemaExtension {
  type: string
  name?: string
  [k: string]: unknown
}

export function TypeBrowserPanel() {
  const universeName = useAppStore((s) => s.displayedUniverseName)

  const [dataTypes, setDataTypes] = useState<main.DataTypeSummary[]>([])
  const [loading, setLoading]     = useState(false)
  const [error, setError]         = useState<string | null>(null)
  const [groupBy, setGroupBy]     = useState<GroupBy>('provider')
  const [query, setQuery]         = useState('')
  const [selectedURI, setSelectedURI] = useState<string | null>(null)
  const [refreshTick, setRefreshTick] = useState(0)

  // Fetch the type list on universe change + refresh.
  useEffect(() => {
    if (!universeName) {
      setDataTypes([])
      setSelectedURI(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    ListDataTypes(universeName)
      .then((r) => {
        if (cancelled) return
        if (r.error) {
          setError(r.error)
          setDataTypes([])
        } else {
          setDataTypes(r.dataTypes ?? [])
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

  // Apply the search filter (name + URI substring, case-insensitive).
  const filtered = useMemo(() => {
    if (!query.trim()) return dataTypes
    const q = query.toLowerCase()
    return dataTypes.filter((t) =>
      t.id.toLowerCase().includes(q) ||
      t.provider.toLowerCase().includes(q) ||
      t.uri.toLowerCase().includes(q),
    )
  }, [dataTypes, query])

  // 'provider' view → grouped tree; 'name' view → flat list (rendered
  // directly in the JSX below, not through the TreeNode shape).
  const tree = useMemo<TreeNode[]>(() => {
    if (groupBy === 'provider') return groupByProvider(filtered)
    return [] // 'name' view rendered as a flat list, not a tree
  }, [filtered, groupBy])

  // Stable alphabetical sort by full URI + URI-level dedup for the
  // 'By Name' view (cwm-v2-providers US-15, 2026-06-08).
  //
  // `system:` URIs are CORRECTLY multi-provider by design (every
  // sidecar bootstraps the framework's system data-types so it can
  // run alone). The catalog returns N copies; the GUI collapses them
  // into a single row + a "+N contributors" chip + a tooltip listing
  // who contributed. Non-`system:` URIs appearing twice are bugs —
  // those keep one row per provider AND surface a yellow warning
  // badge so the duplication is loud.
  //
  // Also fixes a pre-existing React `key` collision when system URIs
  // produced duplicate keys in the list — composing key from
  // `uri + provider` makes it deterministic regardless of dedup mode.
  const flatByName = useMemo<DedupedEntry[]>(() => {
    if (groupBy !== 'name') return []
    const byURI = new Map<string, main.DataTypeSummary[]>()
    for (const e of filtered) {
      const bucket = byURI.get(e.uri) ?? []
      bucket.push(e)
      byURI.set(e.uri, bucket)
    }
    const out: DedupedEntry[] = []
    for (const [uri, list] of byURI) {
      const isSystem = list.some((e) => e.provider === 'system')
      if (isSystem) {
        // System URIs: collapse to one row. Providers chip carries
        // the full list.
        out.push({
          entry: list[0],
          contributors: list.map((e) => e.provider).sort(),
          duplicateNonSystem: false,
        })
      } else if (list.length > 1) {
        // Non-system collision — bug surface. Keep every row but
        // flag each one.
        for (const e of list) {
          out.push({
            entry: e,
            contributors: list.map((x) => x.provider),
            duplicateNonSystem: true,
          })
        }
      } else {
        out.push({ entry: list[0], contributors: [list[0].provider], duplicateNonSystem: false })
      }
    }
    return out.sort((a, b) => a.entry.uri.localeCompare(b.entry.uri))
  }, [filtered, groupBy])

  return (
    <Split style={{ height: '100%' }}>
      <Split.Pane initialWidth={340} minWidth={240} maxWidth={560}>
        <Box style={{ height: '100%', display: 'flex', flexDirection: 'column', borderRight: '1px solid var(--mantine-color-default-border)' }}>
          <Group gap="xs" px="sm" py="xs" justify="space-between" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Text size="sm" fw={600}>Data types</Text>
            <Badge size="xs" variant="light" color="gray">{dataTypes.length}</Badge>
          </Group>

          <Box px="sm" py="xs" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Stack gap={6}>
              <TextInput
                size="xs"
                placeholder="Filter by name, sidecar, URI…"
                value={query}
                onChange={(e) => setQuery(e.currentTarget.value)}
                leftSection={<IconSearch size={12} />}
              />
              <SegmentedControl
                fullWidth
                size="xs"
                value={groupBy}
                onChange={(v) => setGroupBy(v as GroupBy)}
                data={[
                  { value: 'provider', label: 'By Sidecar' },
                  { value: 'name',     label: 'By Name' },
                ]}
              />
            </Stack>
          </Box>

          <Box style={{ flex: 1, overflow: 'auto' }}>
            {!universeName && (
              <Box p="md"><Text size="sm" c="dimmed">No universe active. Activate one from the ribbon to inspect its types.</Text></Box>
            )}
            {universeName && loading && (
              <Center py="xl"><Loader size="sm" /></Center>
            )}
            {error && !loading && (
              <Box p="sm">
                <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Catalog unavailable">{error}</Alert>
              </Box>
            )}
            {universeName && !loading && !error && dataTypes.length === 0 && (
              <Box p="md"><Text size="sm" c="dimmed">Catalog has no data-types yet. Run <Code>ktw-hub catalog refresh</Code> to populate it.</Text></Box>
            )}
            {!loading && !error && groupBy === 'provider' && tree.length > 0 && (
              <Stack gap={0} px="xs" py="xs">
                {tree.map((g) => (
                  <TreeNodeRow
                    key={g.id}
                    node={g}
                    depth={0}
                    selectedURI={selectedURI}
                    onSelect={(t) => setSelectedURI(t.uri)}
                  />
                ))}
              </Stack>
            )}
            {!loading && !error && groupBy === 'name' && flatByName.length > 0 && (
              <Stack gap={0} px="xs" py="xs">
                {flatByName.map((d) => {
                  const e = d.entry
                  const active = e.uri === selectedURI
                  // Key composes URI + provider so non-system duplicates
                  // (which keep one row per provider) don't collide.
                  // System duplicates collapse to one row so the URI
                  // alone would suffice for those — but composing
                  // is uniformly safe.
                  const rowKey = e.uri + ' ' + e.provider
                  const isSystemMulti = d.contributors.length > 1 && !d.duplicateNonSystem
                  return (
                    <Box
                      key={rowKey}
                      onClick={() => setSelectedURI(e.uri)}
                      style={{
                        cursor: 'pointer',
                        padding: '4px 6px',
                        borderRadius: 4,
                        background: active ? 'var(--mantine-color-blue-light)' : 'transparent',
                        color: active ? 'var(--mantine-color-blue-light-color)' : 'inherit',
                      }}
                      onMouseEnter={(ev) => { if (!active) ev.currentTarget.style.background = 'var(--mantine-color-default-hover)' }}
                      onMouseLeave={(ev) => { if (!active) ev.currentTarget.style.background = 'transparent' }}
                    >
                      <Group gap={6} wrap="nowrap" align="center">
                        <Text size="xs" ff="monospace" truncate title={e.uri} style={{ flex: 1, minWidth: 0 }}>{e.uri}</Text>
                        {isSystemMulti && (
                          <Badge
                            size="xs"
                            variant="light"
                            color="grape"
                            title={`Contributed by: ${d.contributors.join(', ')}`}
                          >
                            +{d.contributors.length}
                          </Badge>
                        )}
                        {d.duplicateNonSystem && (
                          <Badge
                            size="xs"
                            variant="light"
                            color="yellow"
                            title={`Duplicate URI in providers: ${d.contributors.join(', ')} — likely a defect`}
                          >
                            dup
                          </Badge>
                        )}
                      </Group>
                    </Box>
                  )
                })}
              </Stack>
            )}
          </Box>
        </Box>
      </Split.Pane>

      <Split.Resizer />

      <Split.Pane grow>
        <TypeDetail
          universeName={universeName}
          dataType={dataTypes.find((d) => d.uri === selectedURI) ?? null}
          refreshKey={refreshTick}
          onRefresh={() => setRefreshTick((n) => n + 1)}
        />
      </Split.Pane>
    </Split>
  )
}

// ── Tree node ──────────────────────────────────────────────────────

function TreeNodeRow({
  node,
  depth,
  selectedURI,
  onSelect,
}: {
  node: TreeNode
  depth: number
  selectedURI: string | null
  onSelect: (t: main.DataTypeSummary) => void
}) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = node.children.length > 0 || node.entries.length > 0
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
            <TreeNodeRow key={c.id} node={c} depth={depth + 1} selectedURI={selectedURI} onSelect={onSelect} />
          ))}
          {node.entries.map((e) => {
            const active = e.uri === selectedURI
            return (
              <Box
                key={e.uri}
                onClick={() => onSelect(e)}
                style={{
                  cursor: 'pointer',
                  padding: '4px 6px',
                  paddingLeft: 6 + (depth + 1) * 10 + 16,
                  borderRadius: 4,
                  background: active ? 'var(--mantine-color-blue-light)' : 'transparent',
                  color: active ? 'var(--mantine-color-blue-light-color)' : 'inherit',
                }}
                onMouseEnter={(ev) => { if (!active) ev.currentTarget.style.background = 'var(--mantine-color-default-hover)' }}
                onMouseLeave={(ev) => { if (!active) ev.currentTarget.style.background = 'transparent' }}
              >
                <Text size="sm" ff="monospace" truncate>{e.id}</Text>
              </Box>
            )
          })}
        </Box>
      )}
    </Box>
  )
}

// ── Right pane: type detail ────────────────────────────────────────

function TypeDetail({
  universeName,
  dataType,
  refreshKey,
}: {
  universeName: string | null
  dataType: main.DataTypeSummary | null
  refreshKey: number
  onRefresh: () => void
}) {
  const [schema, setSchema] = useState<SchemaAST | null>(null)
  const [schemaErr, setSchemaErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  // Hooks-rules: every hook MUST run on every render of this component,
  // so these useState calls live ABOVE the `if (!dataType) return …`
  // early-out.
  //
  //   selected* — current pick in the Select dropdown (sticky across
  //   modal opens, defaults to 'default' which the lib always supplies).
  //   try* — set to a variant name when the modal is OPEN; null = closed.
  const [selectedFormVariant,  setSelectedFormVariant]  = useState<string>('default')
  const [selectedViewVariant,  setSelectedViewVariant]  = useState<string>('default')
  const [selectedTableVariant, setSelectedTableVariant] = useState<string>('default')
  const [tryFormVariant,  setTryFormVariant]  = useState<string | null>(null)
  const [tryViewVariant,  setTryViewVariant]  = useState<string | null>(null)
  const [tryTableVariant, setTryTableVariant] = useState<string | null>(null)

  useEffect(() => {
    if (!universeName || !dataType) {
      setSchema(null)
      setSchemaErr(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setSchemaErr(null)
    // Ask the hub for the parsed AST so we can render structured
    // Types + Extensions panels (more useful than the raw YAML alone).
    // Pass `*`/`*` so the SchemaService doesn't filter any extension
    // out — the Type Browser wants to see them all.
    GetSchemaAST(universeName, dataType.uri, '*', '*')
      .then((r) => {
        if (cancelled) return
        if (r.error) {
          setSchemaErr(r.error)
          setSchema(null)
        } else {
          setSchema((r.ast as unknown as SchemaAST) ?? null)
        }
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setSchemaErr(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, dataType?.uri, refreshKey])

  if (!dataType) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap={4}>
          <IconBraces size={32} color="var(--mantine-color-dimmed)" />
          <Text size="sm" c="dimmed" fw={500}>No type selected</Text>
          <Text size="xs" c="dimmed">Pick a type from the tree on the left.</Text>
        </Stack>
      </Center>
    )
  }

  const types = schema?.types ?? {}
  const typeNames = Object.keys(types).sort()
  const rootName = schema?.root && typeNames.includes(schema.root) ? schema.root : undefined

  // Collect ui-form / ui-view variants declared on the root type, then
  // ALWAYS prepend a 'default' entry. Per the display-lib builders
  // (`buildUIForms` / `buildUIViews`), a synthesized 'default' variant
  // exists for every schema even when the type ships no `ui-form` /
  // `ui-view` extension — basic CRUD is possible for every type. So
  // the user always has at least one Try option per widget kind.
  const rootType = rootName ? types[rootName] : undefined
  const collectVariants = (kind: 'ui-form' | 'ui-view'): string[] => {
    const declared = (rootType?.extensions ?? [])
      .filter((e) => e.type === kind)
      .map((e) => e.name || 'default')
    return Array.from(new Set(['default', ...declared]))
  }
  const formVariantsUnique  = collectVariants('ui-form')
  const viewVariantsUnique  = collectVariants('ui-view')
  const tableVariantsUnique = collectVariants('ui-table' as 'ui-form' | 'ui-view')

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">
        <Stack gap="md">
          {/* Header */}
          <Card withBorder padding="md" radius="md">
            <Stack gap={6}>
              <Title order={4} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{dataType.id}</Title>
              <Group gap={6}>
                <Badge size="sm" variant="light">{dataType.provider}</Badge>
                {dataType.format && <Badge size="sm" variant="light" color="gray">{dataType.format}</Badge>}
                {rootName && <Badge size="sm" variant="filled" color="blue">root: {rootName}</Badge>}
              </Group>
              <Text size="xs" c="dimmed" ff="monospace" style={{ wordBreak: 'break-all' }}>{dataType.uri}</Text>
              {schema?.description && (
                <Text size="sm">{schema.description}</Text>
              )}
              {/* Try-as-widget actions: two button+select pairs, one
                  per widget kind. The Select offers every declared
                  variant plus the synthesised 'default' that the lib's
                  builder always supplies — so the user can render any
                  type, even one with no ui-* extension declared. */}
              <Stack gap={6} mt={4}>
                <Group gap={6} wrap="nowrap" align="center">
                  <Text size="xs" c="dimmed" style={{ minWidth: 40 }}>Form:</Text>
                  <Select
                    size="xs"
                    value={selectedFormVariant}
                    onChange={(v) => setSelectedFormVariant(v ?? 'default')}
                    data={formVariantsUnique}
                    allowDeselect={false}
                    style={{ minWidth: 140 }}
                  />
                  <Button
                    size="xs"
                    variant="light"
                    leftSection={<IconPlayerPlay size={12} />}
                    onClick={() => setTryFormVariant(selectedFormVariant)}
                  >
                    Try as form
                  </Button>
                </Group>
                <Group gap={6} wrap="nowrap" align="center">
                  <Text size="xs" c="dimmed" style={{ minWidth: 40 }}>View:</Text>
                  <Select
                    size="xs"
                    value={selectedViewVariant}
                    onChange={(v) => setSelectedViewVariant(v ?? 'default')}
                    data={viewVariantsUnique}
                    allowDeselect={false}
                    style={{ minWidth: 140 }}
                  />
                  <Button
                    size="xs"
                    variant="light"
                    leftSection={<IconPlayerPlay size={12} />}
                    onClick={() => setTryViewVariant(selectedViewVariant)}
                  >
                    Try as view
                  </Button>
                </Group>
                <Group gap={6} wrap="nowrap" align="center">
                  <Text size="xs" c="dimmed" style={{ minWidth: 40 }}>Table:</Text>
                  <Select
                    size="xs"
                    value={selectedTableVariant}
                    onChange={(v) => setSelectedTableVariant(v ?? 'default')}
                    data={tableVariantsUnique}
                    allowDeselect={false}
                    style={{ minWidth: 140 }}
                  />
                  <Button
                    size="xs"
                    variant="light"
                    leftSection={<IconPlayerPlay size={12} />}
                    onClick={() => setTryTableVariant(selectedTableVariant)}
                  >
                    Try as table
                  </Button>
                </Group>
              </Stack>
            </Stack>
          </Card>

          {loading && <Center py="md"><Loader size="sm" /></Center>}
          {schemaErr && !loading && (
            <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Schema parse failed">
              {schemaErr}
            </Alert>
          )}

          {/* Types card */}
          {!loading && !schemaErr && typeNames.length > 0 && (
            <Card withBorder padding="md" radius="md">
              <Group gap={6} mb="sm">
                <IconShape size={14} />
                <Text size="sm" fw={600}>Types</Text>
                <Badge size="xs" variant="light" color="gray">{typeNames.length}</Badge>
              </Group>
              <Stack gap="sm">
                {typeNames.map((name) => (
                  <TypeBlock key={name} name={name} type={types[name]} isRoot={name === rootName} />
                ))}
              </Stack>
            </Card>
          )}

          {/* Extensions card — aggregated across all types */}
          {!loading && !schemaErr && (() => {
            const exts = collectExtensions(schema)
            if (exts.length === 0) return null
            return (
              <Card withBorder padding="md" radius="md">
                <Group gap={6} mb="sm">
                  <IconPuzzle size={14} />
                  <Text size="sm" fw={600}>Extensions</Text>
                  <Badge size="xs" variant="light" color="gray">{exts.length}</Badge>
                </Group>
                <Stack gap="xs">
                  {exts.map((e, i) => (
                    <Box key={i}>
                      <Group gap={6} mb={2}>
                        <Badge size="sm" variant="filled">{e.type}</Badge>
                        {e.name && <Badge size="sm" variant="light" color="gray">{e.name}</Badge>}
                        <Text size="xs" c="dimmed" ff="monospace">on {e.typeName}</Text>
                      </Group>
                      <Code block style={{ fontSize: 11, whiteSpace: 'pre-wrap', maxHeight: 240, overflow: 'auto' }}>
                        {JSON.stringify(omitTypeName(e), null, 2)}
                      </Code>
                    </Box>
                  ))}
                </Stack>
              </Card>
            )
          })()}

          {/* Raw source */}
          {dataType.source && (
            <Card withBorder padding="md" radius="md">
              <Collapsible title={`Raw source (${dataType.format || 'yaml'})`} defaultOpen={false}>
                <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, maxHeight: 480, overflow: 'auto' }}>
                  {dataType.source}
                </Code>
              </Collapsible>
            </Card>
          )}
        </Stack>
      </Box>

      {/* Try-as-form modal — mounts the FormWidgetPanel pointing at this
          type's URI. Submit logs the captured value to the JS console
          (this is a debug tool — no real action is taken). */}
      {universeName && (
        <Modal
          opened={tryFormVariant !== null}
          onClose={() => setTryFormVariant(null)}
          size="xl"
          centered
          withCloseButton
          padding={0}
          styles={{
            body: { padding: 0, minHeight: 480, maxHeight: '80vh', display: 'flex', flexDirection: 'column' },
            content: { background: 'var(--mantine-color-body)' },
          }}
        >
          {tryFormVariant !== null && (
            <Box style={{ flex: 1, overflow: 'auto' }}>
              <FormWidgetPanel
                universeName={universeName}
                schemaURI={dataType.uri}
                formName={tryFormVariant}
                header={{
                  title:       dataType.id,
                  description: `ui-form variant: ${tryFormVariant}`,
                  iconName:    'shape',
                }}
                // Preview mode: no buttons. The user closes the modal
                // via Mantine's close affordances. If the schema's own
                // ui-form ext declares actions, those still surface via
                // the default 'append' mode below — comment out the
                // pair to opt back in to the lib defaults.
                actionMode="replace"
                actionRefs={[]}
              />
            </Box>
          )}
        </Modal>
      )}

      {/* Try-as-view modal — mounts the ViewWidgetPanel pointing at
          this type's URI. Read-only; rendered with `{}` as the value
          so the user sees the layout (em-dashes per field per the
          lib's default empty strategy). */}
      {universeName && (
        <Modal
          opened={tryViewVariant !== null}
          onClose={() => setTryViewVariant(null)}
          size="xl"
          centered
          withCloseButton
          padding={0}
          styles={{
            body: { padding: 0, minHeight: 480, maxHeight: '80vh', display: 'flex', flexDirection: 'column' },
            content: { background: 'var(--mantine-color-body)' },
          }}
        >
          {tryViewVariant !== null && (
            <Box style={{ flex: 1, overflow: 'auto' }}>
              <ViewWidgetPanel
                universeName={universeName}
                schemaURI={dataType.uri}
                viewName={tryViewVariant}
                header={{
                  title:       dataType.id,
                  description: `ui-view variant: ${tryViewVariant}`,
                  iconName:    'shape',
                }}
              />
            </Box>
          )}
        </Modal>
      )}

      {/* Try-as-table modal — mounts the TableWidgetPanel pointing at
          this type's URI. Read-only; rendered with `[]` as the data so
          the user sees the column headers + an empty-state row. */}
      {universeName && (
        <Modal
          opened={tryTableVariant !== null}
          onClose={() => setTryTableVariant(null)}
          size="xl"
          centered
          withCloseButton
          padding={0}
          styles={{
            body: { padding: 0, minHeight: 480, maxHeight: '80vh', display: 'flex', flexDirection: 'column' },
            content: { background: 'var(--mantine-color-body)' },
          }}
        >
          {tryTableVariant !== null && (
            <Box style={{ flex: 1, overflow: 'auto' }}>
              <TableWidgetPanel
                universeName={universeName}
                schemaURI={dataType.uri}
                tableName={tryTableVariant}
                header={{
                  title:       dataType.id,
                  description: `ui-table variant: ${tryTableVariant}`,
                  iconName:    'shape',
                }}
              />
            </Box>
          )}
        </Modal>
      )}
    </ScrollArea>
  )
}

// ── Type block (one named type in a schema) ────────────────────────

function TypeBlock({ name, type, isRoot }: { name: string; type: SchemaType; isRoot: boolean }) {
  const fields = type.fields ? Object.entries(type.fields) : []
  return (
    <Box style={{ borderLeft: `2px solid var(--mantine-color-${isRoot ? 'blue' : 'gray'}-6)`, paddingLeft: 8 }}>
      <Group gap={6} mb={2}>
        <Text size="sm" fw={600} ff="monospace">{name}</Text>
        {type.type && <Badge size="xs" variant="outline" color="gray">{type.type}</Badge>}
        {isRoot && <Badge size="xs" variant="filled" color="blue">root</Badge>}
        {type.label && <Text size="xs" c="dimmed">— {type.label}</Text>}
      </Group>
      {type.description && (
        <Text size="xs" c="dimmed" mb={4}>{type.description}</Text>
      )}
      {type.enum && type.enum.length > 0 && (
        <Group gap={4} mb={4}>
          <Text size="xs" c="dimmed" fw={500}>enum:</Text>
          {type.enum.map((v, i) => (
            <Badge key={i} size="xs" variant="light">{String(v)}</Badge>
          ))}
        </Group>
      )}
      {fields.length > 0 && (
        <Stack gap={2} pl={6} mt={4}>
          {fields.map(([fname, ftype]) => (
            <Group key={fname} gap={6} wrap="nowrap">
              <Text size="xs" ff="monospace" fw={500} style={{ minWidth: 90 }}>{fname}</Text>
              <Badge size="xs" variant="light" color="gray">{ftype.type || '?'}</Badge>
              {ftype.label && <Text size="xs" c="dimmed" truncate>{ftype.label}</Text>}
              {ftype.optional && <Badge size="xs" variant="outline" color="gray">optional</Badge>}
              {ftype.default !== undefined && (
                <Text size="xs" c="dimmed">= <Code style={{ fontSize: 10 }}>{String(ftype.default)}</Code></Text>
              )}
              {ftype.enum && ftype.enum.length > 0 && (
                <Text size="xs" c="dimmed">[{ftype.enum.map((v) => String(v)).join(', ')}]</Text>
              )}
            </Group>
          ))}
        </Stack>
      )}
    </Box>
  )
}

// ── Helpers ────────────────────────────────────────────────────────

function groupByProvider(entries: main.DataTypeSummary[]): TreeNode[] {
  const byProvider = new Map<string, main.DataTypeSummary[]>()
  for (const e of entries) {
    // Framework-shipped data-types (the canonical `skill-body`,
    // `task-body`, `config-body`, `reference-body` envelopes) arrive
    // with an empty Provider field — the manifest extension contributes
    // them on behalf of every binary, not from a single owner. Bucket
    // them under a "(framework)" group rather than leaving the tree
    // node nameless.
    const key = e.provider || '(framework)'
    const arr = byProvider.get(key) ?? []
    arr.push(e)
    byProvider.set(key, arr)
  }
  return [...byProvider.entries()]
    .sort(([a], [b]) => {
      // Framework group floats to the top so the user finds the
      // canonical envelope types first.
      if (a === '(framework)') return -1
      if (b === '(framework)') return 1
      return a.localeCompare(b)
    })
    .map(([provider, items]): TreeNode => ({
      id:       `provider:${provider}`,
      label:    provider,
      hint:     `${items.length}`,
      children: [],
      entries:  items,
    }))
}

/** Walk the schema's types and collect every declared extension along
 *  with the name of the type it sits on. */
function collectExtensions(schema: SchemaAST | null): Array<SchemaExtension & { typeName: string }> {
  if (!schema?.types) return []
  const out: Array<SchemaExtension & { typeName: string }> = []
  for (const [typeName, t] of Object.entries(schema.types)) {
    for (const ext of t.extensions ?? []) {
      out.push({ ...ext, typeName })
    }
  }
  return out
}

function omitTypeName(e: SchemaExtension & { typeName: string }): SchemaExtension {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { typeName, ...rest } = e
  return rest
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
