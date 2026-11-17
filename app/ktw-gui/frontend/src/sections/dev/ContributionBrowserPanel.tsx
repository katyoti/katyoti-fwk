// SPDX-License-Identifier: Apache-2.0
/**
 * ContributionBrowserPanel — debug-time inventory of every Contribution
 * registered in the active universe's catalog, across every sidecar
 * AND every type (skills, tasks, data-types, services, sidecar-info,
 * config-files, env, contribution-types, …).
 *
 * Two views:
 *   • "By Sidecar" — tree grouped by Provider; type sub-buckets per
 *     provider. The canonical lens for "what does sidecar X publish?".
 *   • "By Type"    — tree grouped by Contribution.Type; provider
 *     sub-buckets per type. The lens for "every skill / every
 *     service / every data-type across the universe".
 *
 * Right pane: header card with URI / Provider / Type / Schema /
 * picker-level Name + Description (the catalog's ContributionMeta).
 * Body fetching is deferred — the row click selects the URI but the
 * heavy body payload stays out of the list view to keep the panel
 * cheap even with hundreds of contributions.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Button,
  Card,
  Center,
  Chip,
  Code,
  Group,
  Loader,
  ScrollArea,
  SegmentedControl,
  Stack,
  Table,
  Tabs,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { Split } from '@gfazioli/mantine-split-pane'
import {
  IconAlertCircle,
  IconArrowUpRight,
  IconChevronDown,
  IconChevronRight,
  IconPackage,
  IconSearch,
} from '@tabler/icons-react'

import { useAppStore } from '../../store'
import { ListContributions, OpenContributionLayer, ReadContribution, WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { DocumentView } from '../../components/document/DocumentView'

// 'provider' → tree grouped by source sidecar with type sub-buckets.
// 'type'     → tree grouped by Contribution.Type with provider
//              sub-buckets — the inverse lens.
type GroupBy = 'provider' | 'type' | 'name'

interface TreeNode {
  id:       string
  label:    string
  hint?:    string
  children: TreeNode[]
  entries:  main.ContributionSummary[]
}

export function ContributionBrowserPanel({ typeFilter }: { typeFilter?: string } = {}) {
  const universeName = useAppStore((s) => s.displayedUniverseName)

  const [items, setItems]       = useState<main.ContributionSummary[]>([])
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState<string | null>(null)
  const [groupBy, setGroupBy]   = useState<GroupBy>('provider')
  const [query, setQuery]       = useState('')
  // Visibility, the second axis of the query surface (concepts §7): `system`
  // is the meta-model every binary embeds, `local` is binary-private. Both
  // are carried by the provider name, so the toggle is a filter over rows the
  // pane already holds — reach (universe) is fixed for a GUI.
  const [showSystem, setShowSystem] = useState(true)
  const [showLocal, setShowLocal]   = useState(true)
  const [selectedURI, setSelectedURI] = useState<string | null>(null)
  const [detail, setDetail]               = useState<main.ContributionDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError]     = useState<string | null>(null)

  // Fetch the contribution list when the universe changes.
  useEffect(() => {
    if (!universeName) {
      setItems([])
      setSelectedURI(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    ListContributions(universeName)
      .then((r) => {
        if (cancelled) return
        if (r.error) {
          setError(r.error)
          setItems([])
        } else {
          setItems(r.contributions ?? [])
        }
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName])

  // Fetch the FULL detail (complete metadata + read body) when a row is
  // selected — a separate per-URI round-trip so the list stays cheap.
  useEffect(() => {
    if (!universeName || !selectedURI) {
      setDetail(null)
      setDetailError(null)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    setDetailError(null)
    setDetail(null)
    ReadContribution(universeName, selectedURI)
      .then((d) => {
        if (cancelled) return
        setDetail(d)
        setDetailError(d.error ? d.error : null)
        setDetailLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setDetailError(String(e))
        setDetailLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, selectedURI])

  // When a typeFilter is set (the Skills / Tasks tabs), scope the
  // inventory to that one contribution type — the rest of the panel
  // (tree, search, detail) is identical to the all-types Resources view.
  const scoped = useMemo(
    () => items.filter((c) =>
      (!typeFilter || c.type === typeFilter) &&
      (showSystem || c.provider !== 'system') &&
      (showLocal || c.provider !== 'local')),
    [items, typeFilter, showSystem, showLocal],
  )

  // Search filter: full-text across name, description, type, provider, URI.
  const filtered = useMemo(() => {
    if (!query.trim()) return scoped
    const q = query.toLowerCase()
    return scoped.filter((c) =>
      (c.name ?? '').toLowerCase().includes(q) ||
      (c.description ?? '').toLowerCase().includes(q) ||
      c.type.toLowerCase().includes(q) ||
      c.provider.toLowerCase().includes(q) ||
      c.uri.toLowerCase().includes(q) ||
      c.id.toLowerCase().includes(q),
    )
  }, [scoped, query])

  // Build the tree per the grouping choice. 'name' is a FLAT list
  // (no grouping) rendered separately below.
  const tree = useMemo<TreeNode[]>(() => {
    if (groupBy === 'provider') return groupByProviderThenType(filtered)
    if (groupBy === 'type')     return groupByTypeThenProvider(filtered)
    return []
  }, [filtered, groupBy])

  // Flat alphabetical-by-name list for the 'By Name' lens.
  const flatByName = useMemo(
    () => [...filtered].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id)),
    [filtered],
  )

  // Counts for the header chip (total + filtered).
  const totalCount    = scoped.length
  const filteredCount = filtered.length

  return (
    <Split style={{ height: '100%' }}>
      <Split.Pane initialWidth={380} minWidth={260} maxWidth={600}>
        <Box style={{ height: '100%', display: 'flex', flexDirection: 'column', borderRight: '1px solid var(--mantine-color-default-border)' }}>
          <Group gap="xs" px="sm" py="xs" justify="space-between" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Text size="sm" fw={600} tt="capitalize">{typeFilter ? `${typeFilter}s` : 'Resources'}</Text>
            <Group gap={4}>
              {query && filteredCount !== totalCount && (
                <Badge size="xs" variant="light" color="blue">{filteredCount}</Badge>
              )}
              <Badge size="xs" variant="light" color="gray">{totalCount}</Badge>
            </Group>
          </Group>

          <Box px="sm" py="xs" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
            <Stack gap={6}>
              <TextInput
                size="xs"
                placeholder="Filter by name, type, sidecar, URI…"
                value={query}
                onChange={(e) => setQuery(e.currentTarget.value)}
                leftSection={<IconSearch size={12} />}
              />
              {/* Type-scoped views (Skills / Tasks) toggle By Sidecar / By
                  Name; the all-types Resources view toggles By Sidecar /
                  By Type (the inverse-lens tree). */}
              <SegmentedControl
                fullWidth
                size="xs"
                value={groupBy}
                onChange={(v) => setGroupBy(v as GroupBy)}
                data={typeFilter
                  ? [
                      { value: 'provider', label: 'By Sidecar' },
                      { value: 'name',     label: 'By Name' },
                    ]
                  : [
                      { value: 'provider', label: 'By Sidecar' },
                      { value: 'type',     label: 'By Type' },
                    ]}
              />
              <Group gap={6}>
                <Text size="xs" c="dimmed">Include</Text>
                <Chip size="xs" checked={showSystem} onChange={setShowSystem}>system</Chip>
                <Chip size="xs" checked={showLocal} onChange={setShowLocal}>local</Chip>
              </Group>
              {!typeFilter && universeName && <DeclarationFindings universeName={universeName} />}
            </Stack>
          </Box>

          <Box style={{ flex: 1, overflow: 'auto' }}>
            {!universeName && (
              <Box p="md"><Text size="sm" c="dimmed">No universe active. Activate one from the ribbon to inspect resources.</Text></Box>
            )}
            {universeName && loading && (
              <Center py="xl"><Loader size="sm" /></Center>
            )}
            {error && !loading && (
              <Box p="sm">
                <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Catalog unavailable">{error}</Alert>
              </Box>
            )}
            {universeName && !loading && !error && items.length === 0 && (
              <Box p="md"><Text size="sm" c="dimmed">Catalog has no resources yet. Run <Code>ktw-hub catalog refresh</Code> to populate it.</Text></Box>
            )}
            {!loading && !error && groupBy === 'name' && flatByName.length > 0 && (
              <Stack gap={0} px="xs" py="xs">
                {flatByName.map((e) => (
                  <EntryRow
                    key={e.uri}
                    entry={e}
                    selected={e.uri === selectedURI}
                    onSelect={() => setSelectedURI(e.uri)}
                    indent={12}
                    showURI
                  />
                ))}
              </Stack>
            )}
            {!loading && !error && groupBy !== 'name' && tree.length > 0 && (
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
          </Box>
        </Box>
      </Split.Pane>

      <Split.Resizer />

      <Split.Pane grow>
        <ContributionDetailPane
          universeName={universeName ?? ''}
          summary={items.find((c) => c.uri === selectedURI) ?? null}
          detail={detail}
          loading={detailLoading}
          error={detailError}
          onOpenURI={(uri) => setSelectedURI(uri)}
          onShowEntries={(kind) => { setGroupBy('type'); setQuery(kind) }}
        />
      </Split.Pane>
    </Split>
  )
}

// ── Tree node row ───────────────────────────────────────────────────

function TreeNodeRow({
  node,
  depth,
  selectedURI,
  onSelect,
}: {
  node: TreeNode
  depth: number
  selectedURI: string | null
  onSelect: (t: main.ContributionSummary) => void
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
          {node.entries.map((e) => (
            <EntryRow
              key={e.uri}
              entry={e}
              selected={e.uri === selectedURI}
              onSelect={() => onSelect(e)}
              indent={6 + (depth + 1) * 10 + 16}
            />
          ))}
        </Box>
      )}
    </Box>
  )
}

// EntryRow — one selectable contribution row. Shared by the grouped tree
// (TreeNodeRow) and the flat 'By Name' list. Label = picker Name (or id);
// the bus-id is a secondary line so the friendly name maps to the wire id.
function EntryRow({
  entry,
  selected,
  onSelect,
  indent,
  showURI,
}: {
  entry:    main.ContributionSummary
  selected: boolean
  onSelect: () => void
  indent:   number
  // showURI: render the full canonical URI as the primary line (the
  // 'By Name' lens — the address actually used for the skill/task, like
  // the Types view), with the friendly name as the secondary line.
  showURI?: boolean
}) {
  const name = entry.name?.trim() || entry.id
  const primary   = showURI ? entry.uri : name
  const secondary = showURI ? name : (name !== entry.id ? entry.id : '')
  return (
    <Box
      onClick={onSelect}
      style={{
        cursor: 'pointer',
        padding: '4px 6px',
        paddingLeft: indent,
        borderRadius: 4,
        background: selected ? 'var(--mantine-color-blue-light)' : 'transparent',
        color: selected ? 'var(--mantine-color-blue-light-color)' : 'inherit',
      }}
      onMouseEnter={(ev) => { if (!selected) ev.currentTarget.style.background = 'var(--mantine-color-default-hover)' }}
      onMouseLeave={(ev) => { if (!selected) ev.currentTarget.style.background = 'transparent' }}
    >
      <Text size="sm" ff="monospace" truncate title={entry.uri}>{primary}</Text>
      {secondary && (
        <Text size="xs" c="dimmed" ff="monospace" truncate>{secondary}</Text>
      )}
    </Box>
  )
}

// ── Right pane: contribution detail ─────────────────────────────────

/** ContributionReader — one contribution read in place: its detail (meta,
 *  layers, provider) fetched by its URI, shown by the browser's detail pane.
 *  A URI the pane links to opens in place. Used outside the Debug tab (the
 *  Contributions panel of sys-contributions). */
export function ContributionReader({ universeName, summary }: { universeName: string; summary: main.ContributionSummary }) {
  const [uri, setURI] = useState(summary.uri)
  const [detail, setDetail] = useState<main.ContributionDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => { setURI(summary.uri) }, [summary.uri])
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setDetail(null)
    ReadContribution(universeName, uri)
      .then((d) => {
        if (cancelled) return
        setDetail(d)
        setError(d.error ? d.error : null)
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, uri])
  const shown = uri === summary.uri ? summary : ({ ...summary, uri, id: uri, name: uri } as main.ContributionSummary)
  return (
    <ContributionDetailPane
      universeName={universeName}
      summary={shown}
      detail={detail}
      loading={loading}
      error={error}
      onOpenURI={setURI}
      onShowEntries={() => {}}
    />
  )
}

function ContributionDetailPane({
  universeName,
  summary,
  detail,
  loading,
  error,
  onOpenURI,
  onShowEntries,
}: {
  universeName: string
  summary: main.ContributionSummary | null
  detail:  main.ContributionDetail | null
  loading: boolean
  error:   string | null
  onOpenURI: (uri: string) => void
  onShowEntries: (kind: string) => void
}) {
  if (!summary) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap={4}>
          <IconPackage size={32} color="var(--mantine-color-dimmed)" />
          <Text size="sm" c="dimmed" fw={500}>No resource selected</Text>
          <Text size="xs" c="dimmed">Pick an entry from the tree on the left.</Text>
        </Stack>
      </Center>
    )
  }

  // Scope = the bus-boundary classifier: `system` = bootstrap metadata;
  // `local` = binary-private (CLI-only); anything else = bus-visible sidecar.
  const scope: 'system' | 'local' | 'sidecar' =
    summary.provider === 'system' ? 'system' :
    summary.provider === 'local'  ? 'local'  : 'sidecar'
  const isKind = summary.type === 'contribution-type'

  // Validation badge color: green = valid, red = invalid, gray = unchecked.
  const v = detail?.validation
  const vColor = v === 'valid' ? 'green' : v === 'invalid' ? 'red' : 'gray'

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">
        <Stack gap="md">
          {/* Header — instant from the list summary; the kind link once the detail is in. */}
          <Card withBorder padding="md" radius="md">
            <Stack gap={6}>
              <Title order={4} style={{ wordBreak: 'break-all' }}>
                {detail?.iconEmoji ? `${detail.iconEmoji} ` : ''}{summary.name?.trim() || summary.id}
              </Title>
              <Group gap={6} wrap="wrap">
                <Badge size="sm" variant="filled" color="blue">{summary.type}</Badge>
                <Badge
                  size="sm"
                  variant="light"
                  color={scope === 'system' ? 'grape' : scope === 'local' ? 'orange' : 'gray'}
                >
                  {scope}: {summary.provider}
                </Badge>
                {detail && (
                  <Badge size="sm" variant="light" color={vColor}>{detail.validation}</Badge>
                )}
                {detail?.kind?.uri && (
                  <Button size="compact-xs" variant="subtle" rightSection={<IconArrowUpRight size={12} />}
                    onClick={() => onOpenURI(detail.kind.uri!)}>
                    kind: {detail.kind.name}
                  </Button>
                )}
                {isKind && (
                  <Button size="compact-xs" variant="subtle" rightSection={<IconArrowUpRight size={12} />}
                    onClick={() => onShowEntries(summary.id)}>
                    entries of {summary.id}
                  </Button>
                )}
              </Group>
              <Text size="xs" c="dimmed" ff="monospace" style={{ wordBreak: 'break-all' }}>{summary.uri}</Text>
              {(detail?.description ?? summary.description) && (
                <Text size="sm" mt={4}>{detail?.description ?? summary.description}</Text>
              )}
            </Stack>
          </Card>

          {error && (
            <Alert color="red" icon={<IconAlertCircle size={14} />} variant="light" title="Detail unavailable">{error}</Alert>
          )}
          {loading && <Center py="md"><Loader size="sm" /></Center>}

          {detail && !loading && (
            <>
              {/* Full metadata — the complete ContributionMeta. */}
              <Card withBorder padding="md" radius="md">
                <Stack gap={6}>
                  <Text size="xs" c="dimmed" fw={600}>Metadata</Text>
                  {detail.category && <KVRow label="Category" value={detail.category} />}
                  {detail.mediaType && <KVRow label="Media type" value={detail.mediaType} />}
                  <KVRow label="Advertised" value={detail.advertised ? 'yes' : 'no'} />
                  {detail.iconTabler && <KVRow label="Icon" value={detail.iconTabler} />}
                  {detail.hashtags && detail.hashtags.length > 0 && (
                    <Group gap={6} wrap="nowrap" align="flex-start">
                      <Text size="xs" c="dimmed" style={{ minWidth: 70 }}>Hashtags</Text>
                      <Group gap={4} wrap="wrap">
                        {detail.hashtags.map((h) => (
                          <Badge key={h} size="xs" variant="light" color="blue">{h}</Badge>
                        ))}
                      </Group>
                    </Group>
                  )}
                  {detail.annotations && Object.keys(detail.annotations).length > 0 && (
                    <>
                      <Text size="xs" c="dimmed" mt={4}>Annotations</Text>
                      {Object.entries(detail.annotations).map(([k, val]) => (
                        <KVRow key={k} label={k} value={String(val)} />
                      ))}
                    </>
                  )}
                </Stack>
              </Card>

              {/* The kind that governs this entry — its declaration, one click away. */}
              <KindCard kind={detail.kind} onOpenURI={onOpenURI} />

              {/* Every layer: declared vs carried, band, hash, size — and one tab per layer. */}
              <LayersCard universeName={universeName} uri={summary.uri} detail={detail} />

              {/* Validation verdict — READ from ingest, never re-derived here. */}
              <Card withBorder padding="md" radius="md">
                <Stack gap={6}>
                  <Group gap={6}>
                    <Text size="xs" c="dimmed" fw={600}>Validation</Text>
                    <Badge size="xs" variant="filled" color={vColor}>{detail.validation}</Badge>
                  </Group>
                  {detail.validationErrors && detail.validationErrors.length > 0 && (
                    <Stack gap={2}>
                      {detail.validationErrors.map((e, i) => (
                        <Text key={i} size="xs" c="red" ff="monospace" style={{ wordBreak: 'break-word' }}>• {e}</Text>
                      ))}
                    </Stack>
                  )}
                  {detail.schema && <KVRow label="Schema" value={detail.schema} />}
                </Stack>
              </Card>

              {/* The producer's identity, as its manifest recorded it. */}
              <ProviderCard p={detail.providerInfo} />
            </>
          )}
        </Stack>
      </Box>
    </ScrollArea>
  )
}

function KindCard({ kind, onOpenURI }: { kind: main.KindView; onOpenURI: (uri: string) => void }) {
  return (
    <Card withBorder padding="md" radius="md">
      <Stack gap={6}>
        <Group gap={6} justify="space-between" wrap="nowrap">
          <Group gap={6}>
            <Text size="xs" c="dimmed" fw={600}>Kind</Text>
            <Badge size="xs" variant={kind.declared ? 'light' : 'filled'} color={kind.declared ? 'grape' : 'red'}>
              {kind.declared ? 'declared' : 'undeclared'}
            </Badge>
          </Group>
          {kind.uri && (
            <Button size="compact-xs" variant="subtle" rightSection={<IconArrowUpRight size={12} />} onClick={() => onOpenURI(kind.uri!)}>
              open declaration
            </Button>
          )}
        </Group>
        <KVRow label="Name" value={kind.name} />
        {kind.schema && <KVRow label="Schema" value={kind.schema} />}
        {kind.validate && <KVRow label="Severity" value={kind.validate} />}
        {kind.execution && <KVRow label="Execution" value={kind.execution} />}
        {kind.defaultLayer && <KVRow label="Default layer" value={kind.defaultLayer} />}
        {kind.budget ? <KVRow label="Meta budget" value={`${kind.budget} B`} /> : null}
        {kind.findings && kind.findings.length > 0 && (
          <Stack gap={2}>
            {kind.findings.map((f, i) => (
              <Text key={i} size="xs" c="orange" ff="monospace" style={{ wordBreak: 'break-word' }}>• {f}</Text>
            ))}
          </Stack>
        )}
      </Stack>
    </Card>
  )
}

function ProviderCard({ p }: { p: main.ProviderView }) {
  const idColor = !p.known ? 'gray' : !p.declared ? 'red' : p.mutable ? 'orange' : 'green'
  const idLabel = !p.known ? 'no manifest' : !p.declared ? 'identity undeclared' : p.mutable ? 'mutable' : 'immutable'
  return (
    <Card withBorder padding="md" radius="md">
      <Stack gap={6}>
        <Group gap={6}>
          <Text size="xs" c="dimmed" fw={600}>Provider</Text>
          <Badge size="xs" variant="light" color={idColor}>{idLabel}</Badge>
          {p.drops > 0 && <Badge size="xs" variant="filled" color="red">{p.drops} dropped file{p.drops > 1 ? 's' : ''}</Badge>}
        </Group>
        <KVRow label="Producer" value={`${p.name} / ${p.type}`} />
        {p.digest && <KVRow label="Digest" value={p.digest} />}
      </Stack>
    </Card>
  )
}

// LayersCard: the Layers table (declared ∪ carried) and one tab per carried
// layer. The default layer arrives with the detail; every other tab opens on
// demand through the content lane (OpenContributionLayer), and a refused bulk
// layer shows its address instead of bytes — never a substitute body.
function LayersCard({ universeName, uri, detail }: { universeName: string; uri: string; detail: main.ContributionDetail }) {
  const layers = detail.layers ?? []
  const carried = layers.filter((l) => l.carried)
  const defaultName = detail.bodySource?.startsWith('layer:') ? detail.bodySource.slice('layer:'.length) : undefined
  const [active, setActive] = useState<string | null>(defaultName ?? carried[0]?.name ?? null)
  const [contents, setContents] = useState<Record<string, main.LayerContent | 'loading'>>(() =>
    defaultName
      ? { [defaultName]: lcFrom({
          name: defaultName, hash: '', size: detail.bodySize, band: '', mediaType: detail.mediaType,
          refused: detail.refused, isBinary: detail.isBinary, content: detail.body, error: '' }) }
      : {})

  // Reset when the entry changes.
  useEffect(() => {
    setActive(defaultName ?? carried[0]?.name ?? null)
    setContents(defaultName
      ? { [defaultName]: lcFrom({
          name: defaultName, hash: '', size: detail.bodySize, band: '', mediaType: detail.mediaType,
          refused: detail.refused, isBinary: detail.isBinary, content: detail.body, error: '' }) }
      : {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uri])

  useEffect(() => {
    if (!active || !universeName || contents[active]) return
    let cancelled = false
    setContents((c) => ({ ...c, [active]: 'loading' }))
    OpenContributionLayer(universeName, uri, active)
      .then((lc) => { if (!cancelled) setContents((c) => ({ ...c, [active]: lc })) })
      .catch((e) => { if (!cancelled) setContents((c) => ({ ...c, [active]: lcFrom({ name: active, error: String(e), size: 0, isBinary: false }) })) })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, universeName, uri])

  return (
    <Card withBorder padding="md" radius="md">
      <Stack gap={8}>
        <Text size="xs" c="dimmed" fw={600}>Layers</Text>
        {layers.length === 0 && <Text size="sm" c="dimmed">No layers: the metadata is the whole resource.</Text>}
        {layers.length > 0 && (
          <Table withTableBorder={false} verticalSpacing={2} fz="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Layer</Table.Th><Table.Th>Declared</Table.Th><Table.Th>Carried</Table.Th>
                <Table.Th>Band</Table.Th><Table.Th>Size</Table.Th><Table.Th>Media type</Table.Th><Table.Th>Hash</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {layers.map((l) => (
                <Table.Tr key={l.name}>
                  <Table.Td><Code>{l.name}</Code>{l.default ? <Badge ml={4} size="xs" variant="light">default</Badge> : null}</Table.Td>
                  <Table.Td>{l.declared ? (l.required ? 'required' : 'yes') : <Text span c="orange">no</Text>}{l.resident ? ' · resident' : ''}</Table.Td>
                  <Table.Td>{l.carried ? 'yes' : <Text span c={l.required ? 'red' : 'dimmed'}>no</Text>}</Table.Td>
                  <Table.Td>{l.band ?? ''}</Table.Td>
                  <Table.Td>{l.carried ? `${l.size} B` : ''}</Table.Td>
                  <Table.Td>{l.mediaType ?? ''}</Table.Td>
                  <Table.Td><Text span ff="monospace" style={{ wordBreak: 'break-all' }}>{l.hash ? l.hash.slice(0, 19) + '…' : ''}</Text></Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
        {carried.length > 0 && (
          <Tabs value={active} onChange={setActive} keepMounted={false}>
            <Tabs.List>
              {carried.map((l) => <Tabs.Tab key={l.name} value={l.name}>{l.name}</Tabs.Tab>)}
            </Tabs.List>
            {carried.map((l) => (
              <Tabs.Panel key={l.name} value={l.name} pt="xs">
                <LayerBody lc={contents[l.name]} />
              </Tabs.Panel>
            ))}
          </Tabs>
        )}
      </Stack>
    </Card>
  )
}

// lcFrom types a plain object as the generated model — the model is imported
// as a type, so its class constructor is not available here.
function lcFrom(o: Partial<main.LayerContent>): main.LayerContent { return o as main.LayerContent }

function LayerBody({ lc }: { lc: main.LayerContent | 'loading' | undefined }) {
  if (!lc || lc === 'loading') return <Center py="sm"><Loader size="xs" /></Center>
  if (lc.error) return <Alert color="red" variant="light" icon={<IconAlertCircle size={14} />}>{lc.error}</Alert>
  if (lc.refused) {
    return (
      <Alert color="yellow" variant="light" title="Not carried on the control plane">
        <Text size="xs">{lc.refused}</Text>
        <Text size="xs" ff="monospace" mt={4}>{lc.hash} · {lc.size} B · {lc.band}</Text>
      </Alert>
    )
  }
  if (lc.isBinary) return <Text size="sm" c="dimmed">Binary content ({lc.size} bytes{lc.mediaType ? `, ${lc.mediaType}` : ''}) — not rendered.</Text>
  return (
    <Stack gap={4}>
      <Group gap={4} wrap="wrap">
        {lc.mediaType && <Badge size="xs" variant="outline" color="gray">{lc.mediaType}</Badge>}
        <Badge size="xs" variant="light" color="gray">{lc.size} B</Badge>
        {lc.band && <Badge size="xs" variant="light" color="teal">{lc.band}</Badge>}
      </Group>
      <Box style={{ height: 420, border: '1px solid var(--mantine-color-default-border)', borderRadius: 4 }}>
        <DocumentView doc={{ mediaType: lc.mediaType ?? '', title: lc.name, content: lc.content ?? '' }} host={{ fillHeight: true }} />
      </Box>
    </Stack>
  )
}

function KVRow({ label, value }: { label: string; value: string }) {
  return (
    <Group gap={6} wrap="nowrap" align="flex-start">
      <Text size="xs" c="dimmed" style={{ minWidth: 70 }}>{label}</Text>
      <Text size="xs" ff="monospace" style={{ wordBreak: 'break-all' }}>{value}</Text>
    </Group>
  )
}

// ── Tree builders ───────────────────────────────────────────────────

/**
 * Provider → Type two-level tree. Framework / system contributions
 * float to the top (they're the bootstrap metadata every binary
 * carries); local-scope contributions sink to the bottom (they're
 * binary-private). Sidecar providers are sorted alphabetically.
 */
function groupByProviderThenType(entries: main.ContributionSummary[]): TreeNode[] {
  const byProvider = new Map<string, main.ContributionSummary[]>()
  for (const e of entries) {
    const key = e.provider || '(unnamed)'
    const arr = byProvider.get(key) ?? []
    arr.push(e)
    byProvider.set(key, arr)
  }
  return [...byProvider.entries()]
    .sort(([a], [b]) => providerSortKey(a) - providerSortKey(b) || a.localeCompare(b))
    .map(([provider, items]): TreeNode => ({
      id:       `provider:${provider}`,
      label:    provider,
      hint:     `${items.length}`,
      children: bucketByType(items, `provider:${provider}`),
      entries:  [],
    }))
}

/**
 * Type → Provider two-level tree. Use this lens when the question is
 * "show me every X across the universe" — every skill, every service,
 * every data-type. Types are sorted alphabetically.
 */
function groupByTypeThenProvider(entries: main.ContributionSummary[]): TreeNode[] {
  const byType = new Map<string, main.ContributionSummary[]>()
  for (const e of entries) {
    const key = e.type || '(unknown)'
    const arr = byType.get(key) ?? []
    arr.push(e)
    byType.set(key, arr)
  }
  return [...byType.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([type, items]): TreeNode => ({
      id:       `type:${type}`,
      label:    type,
      hint:     `${items.length}`,
      children: bucketByProvider(items, `type:${type}`),
      entries:  [],
    }))
}

function bucketByType(entries: main.ContributionSummary[], parentId: string): TreeNode[] {
  const byType = new Map<string, main.ContributionSummary[]>()
  for (const e of entries) {
    const key = e.type || '(unknown)'
    const arr = byType.get(key) ?? []
    arr.push(e)
    byType.set(key, arr)
  }
  return [...byType.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([type, items]): TreeNode => ({
      id:       `${parentId}:${type}`,
      label:    type,
      hint:     `${items.length}`,
      children: [],
      entries:  items,
    }))
}

function bucketByProvider(entries: main.ContributionSummary[], parentId: string): TreeNode[] {
  const byProvider = new Map<string, main.ContributionSummary[]>()
  for (const e of entries) {
    const key = e.provider || '(unnamed)'
    const arr = byProvider.get(key) ?? []
    arr.push(e)
    byProvider.set(key, arr)
  }
  return [...byProvider.entries()]
    .sort(([a], [b]) => providerSortKey(a) - providerSortKey(b) || a.localeCompare(b))
    .map(([provider, items]): TreeNode => ({
      id:       `${parentId}:${provider}`,
      label:    provider,
      hint:     `${items.length}`,
      children: [],
      entries:  items,
    }))
}

/**
 * Provider sort key: `system` always first (bootstrap metadata),
 * `local` always last (binary-private artifacts), real sidecar
 * providers in between. The labels group by SCOPE first, then by name
 * — matches the report's three-tier classification.
 */
function providerSortKey(provider: string): number {
  if (provider === 'system') return 0
  if (provider === 'local')  return 2
  return 1
}

// ── Declaration findings ────────────────────────────────────────────

// DECLARATION_FINDINGS is CWM's answer to "what is wrong with the universe's
// declarations": one skipped (it does not decode, or its kind's format is a
// newer major), read leniently (a newer minor), not registered, or a gate
// entry narrowed to its provider's scope (cwmapi.DeclarationFindings).
const DECLARATION_FINDINGS = 'ktw-agents:cwm/declaration-findings'

interface DeclarationFinding {
  rule: string
  severity: string
  kind: string
  name?: string
  uri?: string
  provider?: string
  message: string
}

// DeclarationFindings — the universe's declaration health, above the tree:
// nothing when all is well, else a collapsible list, errors first.
function DeclarationFindings({ universeName }: { universeName: string }) {
  const [findings, setFindings] = useState<DeclarationFinding[]>([])
  const [open, setOpen] = useState(false)
  useEffect(() => {
    let cancelled = false
    WorkspaceRunAction(universeName, DECLARATION_FINDINGS, '{}')
      .then((r) => {
        if (cancelled || r.error || !r.value) return
        const out: { findings?: DeclarationFinding[] } = JSON.parse(r.value)
        setFindings(out.findings ?? [])
      })
      .catch(() => { /* no CWM in this universe: nothing to show */ })
    return () => { cancelled = true }
  }, [universeName])
  if (findings.length === 0) return null
  const errors = findings.filter((f) => f.severity === 'error').length
  const sorted = [...findings].sort((a, b) => (a.severity === b.severity ? 0 : a.severity === 'error' ? -1 : 1))
  const Chev = open ? IconChevronDown : IconChevronRight
  return (
    <Alert
      p="xs"
      color={errors > 0 ? 'red' : 'yellow'}
      icon={<IconAlertCircle size={14} />}
      title={
        <Group gap={4} onClick={() => setOpen((v) => !v)} style={{ cursor: 'pointer' }}>
          <Chev size={12} />
          <Text size="xs" fw={600}>Declaration findings</Text>
          <Badge size="xs" variant="light" color={errors > 0 ? 'red' : 'yellow'}>{findings.length}</Badge>
        </Group>
      }
    >
      {open && (
        <Stack gap={4}>
          {sorted.map((f, i) => (
            <Box key={i}>
              <Group gap={4} wrap="nowrap">
                <Badge size="xs" variant="light" color={f.severity === 'error' ? 'red' : 'yellow'}>{f.rule}</Badge>
                <Text size="xs" fw={500} truncate>{f.uri || `${f.provider ?? ''} ${f.kind} ${f.name ?? ''}`}</Text>
              </Group>
              <Text size="xs" c="dimmed">{f.message}</Text>
            </Box>
          ))}
        </Stack>
      )}
    </Alert>
  )
}
