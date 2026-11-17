// SPDX-License-Identifier: Apache-2.0
// UniversalHarness — single configurable React harness for ALL widget kinds
// (form / view / table / wizard / page / proto-walker).
//
// Replaces the 5 per-kind harnesses (ui-form-harness.tsx, ui-view-harness.tsx,
// ui-table-harness.tsx, ui-wizard-harness.tsx, ui-page-harness.tsx) plus the
// proto walker (harness.tsx). Net: ~713 LOC of duplication eliminated.
//
// Per display-package-restructure D5: accepts `config + kind + renderInner`.
// The harness owns ALL shared logic (postMessage emit, ready event, value
// subscription, layout chrome, outcome panel, debug panes); per-kind logic
// is a tiny ~20 LOC `renderInner(ctx)` function (per kind).
//
// Per D6: the per-kind accent color in OutcomePanel is gone — single accent
// (theme primary). The 5 different colors carried no semantic meaning.
//
// Display mode controlled by URL params:
//   ?display=ui       (host integration default, recommended) — only the
//                     rendered widget, full-bleed, NO Card / NO title /
//                     NO description / NO debug panes. The host's tab UI
//                     provides any chrome.
//   ?display=schema   only the source schema panel, full-bleed. Used by
//                     the host's Schema tab to render the literal schema
//                     without duplicating the JSON in TS.
//   ?display=full     full chrome — title + description + Card + the debug
//                     panes selected by the legacy flags below. This is
//                     the direct-browser-debugging surface.
//
// Legacy flags (used only when `?display` is NOT set; preserved for
// backwards-compat with bookmarked dev URLs):
//   ?debug=true       enable all debug panes (alias for the 3 below)
//   ?showSchema=true  show the schema source pane
//   ?showEvents=true  show the event log pane (postMessage outcomes)
//   ?showState=true   show the artifact state pane (current value / dslTree)
//   ?embedded=true    same as `?display=ui` (strips chrome).

import { Anchor, Box, Card, Code, Notification, Stack, Text, Title } from '@mantine/core'
import { useEffect, useMemo, useState, type ReactNode } from 'react'

import { listenToHost } from '../postmessage.ts'
import { makeEmit, resolveWidgetId, type WidgetMessage } from './postmessage.ts'
import type { HarnessConfig } from './harness.tsx'

// =================================================================================================
// Types
// =================================================================================================

export type UniversalHarnessKind = 'form' | 'view' | 'table' | 'tree-table' | 'wizard' | 'page' | 'drawio' | 'deck' | 'proto'

/** Outcome event recorded for the showEvents debug pane. */
export type OutcomeEvent = { ts: string; label: string; payload: unknown }

/** Context passed to per-kind renderInner. Carries the resolved state +
 *  handlers the inner view needs. */
export interface RenderInnerCtx {
  config: HarnessConfig
  /** Reactive item value — seeded from `config.initialValue`, updated when
   *  the host pushes a `value` postMessage. For non-page kinds this is
   *  the form/view/table/wizard data. */
  item: unknown
  /** Reactive DSL tree — set ONLY for `kind: 'page'`. */
  tree: unknown
  /** Outer data tree for `dataField` bindings — set ONLY for `kind: 'page'`.
   *  Wired by display-package-restructure Phase 3 (2026-05-06). */
  data: unknown
  /** postMessage emit — fires events back to the host. */
  emit: (msg: WidgetMessage) => void
  /** Append an outcome event to the debug pane. No-op when showEvents is off. */
  appendOutcome: (label: string, payload: unknown) => void
}

export interface UniversalHarnessProps {
  config: HarnessConfig
  kind: UniversalHarnessKind
  /** Per-kind renderer. Returns the ReactNode to mount inside the
   *  content box. ~20 LOC per kind; selects the right UIxxxView. */
  renderInner: (ctx: RenderInnerCtx) => ReactNode
}

// =================================================================================================
// URL-param debug toggles
// =================================================================================================

type DisplayMode = 'ui' | 'schema' | 'full'

interface DebugFlags {
  /** Resolved display mode. `ui` = widget only (no chrome). `schema` = schema
   *  panel only. `full` = legacy debug chrome with optional debug panes. */
  display: DisplayMode
  showSchema: boolean
  showEvents: boolean
  showState: boolean
}

function readDebugFlags(config: HarnessConfig): DebugFlags {
  let params: URLSearchParams
  try {
    params = new URLSearchParams(window.location.search)
  } catch {
    return {
      display: config.embedded ? 'ui' : 'full',
      showSchema: !!config.showSchemaPane,
      showEvents: !!config.showOutcome,
      showState: false,
    }
  }
  // Resolve display mode. New `?display=` wins; otherwise legacy `?embedded=`
  // maps to `display=ui`; otherwise `full`.
  const displayParam = params.get('display')
  let display: DisplayMode
  if (displayParam === 'ui' || displayParam === 'schema' || displayParam === 'full') {
    display = displayParam
  } else if (params.get('embedded') === 'true' || config.embedded) {
    display = 'ui'
  } else {
    display = 'full'
  }
  const debug = params.get('debug') === 'true'
  return {
    display,
    showSchema: debug || params.get('showSchema') === 'true' || !!config.showSchemaPane,
    showEvents: debug || params.get('showEvents') === 'true' || !!config.showOutcome,
    showState: debug || params.get('showState') === 'true',
  }
}

// =================================================================================================
// OutcomePanel — single shared component, no per-kind color
// =================================================================================================

function OutcomePanel({ events }: { events: OutcomeEvent[] }): ReactNode {
  if (events.length === 0) {
    return (
      <Box mt="md" p="sm" style={{ background: 'var(--mantine-color-gray-1)', borderRadius: 4 }}>
        <Text size="xs" c="dimmed" data-testid="case.outcome">
          No events yet.
        </Text>
      </Box>
    )
  }
  return (
    <Box
      mt="md"
      p="sm"
      data-testid="case.outcome"
      style={{
        background: 'var(--mantine-color-gray-0)',
        borderLeft: '3px solid var(--mantine-primary-color-filled)',
        borderRadius: 4,
        fontFamily: 'ui-monospace, SFMono-Regular, monospace',
        fontSize: 12,
        maxHeight: 200,
        overflow: 'auto',
      }}
    >
      {events.map((e, i) => (
        <Box key={i} mb="xs">
          <Text size="xs" component="span" c="dimmed">
            [{e.ts}]
          </Text>{' '}
          <Text size="xs" component="span" fw={600}>
            {e.label}
          </Text>
          <Code block style={{ fontSize: 11, marginTop: 2 }}>
            {JSON.stringify(e.payload, null, 2)}
          </Code>
        </Box>
      ))}
    </Box>
  )
}

// =================================================================================================
// Debug panes (right column when not embedded)
// =================================================================================================

function SchemaSourcePane({ config }: { config: HarnessConfig }) {
  return (
    <Card withBorder padding="md" radius="md">
      <Text fw={600} size="sm" mb="xs">
        Source schema
      </Text>
      <Code
        block
        data-testid="case.schema"
        style={{ fontSize: 11, maxHeight: 480, overflow: 'auto' }}
      >
        {JSON.stringify(config.schema, null, 2)}
      </Code>
      <Text size="xs" c="dimmed" mt="xs">
        ↑ Schema literal. Toggle with{' '}
        <Anchor href="?showSchema=true" target="_self">
          ?showSchema=true
        </Anchor>
        .
      </Text>
    </Card>
  )
}

function ArtifactStatePane({ item, tree }: { item: unknown; tree: unknown }) {
  const payload = tree !== undefined && tree !== null ? { tree } : { item }
  return (
    <Card withBorder padding="md" radius="md">
      <Text fw={600} size="sm" mb="xs">
        Artifact state
      </Text>
      <Code block style={{ fontSize: 11, maxHeight: 480, overflow: 'auto' }}>
        {JSON.stringify(payload, null, 2)}
      </Code>
    </Card>
  )
}

// =================================================================================================
// UniversalHarness
// =================================================================================================

export function UniversalHarness({ config, kind, renderInner }: UniversalHarnessProps): ReactNode {
  const widgetId = resolveWidgetId(config.widgetName)
  const emit = useMemo(
    () => makeEmit({ widget: config.widgetName, widgetId }),
    [config.widgetName, widgetId],
  )

  const flags = useMemo(() => readDebugFlags(config), [config])

  // Reactive item / tree / data state — seeded from config; updated when
  // the host pushes a `value` postMessage. Cross-turn `patch-ui-component`
  // updates propagate through here. `data` is the outer data tree for
  // `dataField` JSONPath bindings (page kind only — Phase 3).
  const [item, setItem] = useState<unknown>(config.initialValue)
  const [tree, setTree] = useState<unknown>(config.dslTree)
  const [data, setData] = useState<unknown>(config.data)

  const [outcomeEvents, setOutcomeEvents] = useState<OutcomeEvent[]>([])

  const appendOutcome = (label: string, payload: unknown) => {
    if (!flags.showEvents) return
    const ts = new Date().toISOString().slice(11, 19)
    setOutcomeEvents((prev) => [{ ts, label, payload }, ...prev].slice(0, 5))
  }

  // Emit `ready` once on mount.
  useEffect(() => {
    emit({ type: 'ready', ts: Date.now() })
  }, [emit])

  // Subscribe to host `value` postMessage. The shape differs per kind:
  // - page: msg.value IS the dslTree (whole tree)
  // - others: msg.value IS the item value (record / array)
  // We update both setters opportunistically; the renderInner picks
  // whichever is right for its kind.
  useEffect(() => {
    const unsub = listenToHost((msg) => {
      if (msg.type === 'value' && msg.widgetId === widgetId) {
        if (kind === 'page') {
          setTree(msg.value ?? undefined)
        } else {
          setItem(msg.value ?? undefined)
        }
      }
    })
    return unsub
  }, [widgetId, kind])

  const ctx: RenderInnerCtx = {
    config,
    item,
    tree,
    data,
    emit,
    appendOutcome,
  }

  // ── display=ui: widget only, no chrome. Modest padding for breathing
  //    room; per-kind max-width keeps line lengths readable for narrow-
  //    column content (form / view / wizard) while letting wide content
  //    (table / page / proto) use the full host width. ──
  if (flags.display === 'ui') {
    const wantsConstrainedWidth = kind === 'form' || kind === 'view' || kind === 'wizard'
    return (
      <Box
        data-testid={`${kind}.root`}
        style={{
          width: '100%',
          padding: 16,
          boxSizing: 'border-box',
        }}
      >
        <Box
          style={{
            maxWidth: wantsConstrainedWidth ? 880 : '100%',
            marginInline: 'auto',
          }}
        >
          {renderInner(ctx)}
        </Box>
      </Box>
    )
  }

  // ── display=schema: schema panel only, full-bleed. ──
  if (flags.display === 'schema') {
    return (
      <Box style={{ padding: 0 }}>
        <Code
          block
          data-testid="case.schema"
          style={{ fontSize: 11, padding: 12, margin: 0, borderRadius: 0, minHeight: '100vh' }}
        >
          {JSON.stringify(config.schema, null, 2)}
        </Code>
      </Box>
    )
  }

  // ── display=full (legacy debug surface): full chrome with optional panes. ──
  const hasRightPane = flags.showSchema || flags.showState
  const layoutCols = hasRightPane ? '2fr 1fr' : '1fr'

  return (
    <Box
      style={{
        display: 'grid',
        gridTemplateColumns: layoutCols,
        gap: 24,
        alignItems: 'start',
        padding: 24,
        background: 'var(--mantine-color-gray-0)',
        minHeight: '100vh',
      }}
    >
      <Card withBorder shadow="sm" padding="lg" radius="md">
        {config.pageTitle && (
          <Title order={3} mb={4}>
            {config.pageTitle}
          </Title>
        )}
        {config.description && (
          <Text size="sm" c="dimmed" mb="md">
            {config.description}
          </Text>
        )}
        <Box data-testid={`${kind}.root`}>{renderInner(ctx)}</Box>
        {flags.showEvents && <OutcomePanel events={outcomeEvents} />}
      </Card>

      {hasRightPane && (
        <Stack gap="md">
          {flags.showSchema && <SchemaSourcePane config={config} />}
          {flags.showState && <ArtifactStatePane item={item} tree={tree} />}
        </Stack>
      )}
    </Box>
  )
}

// =================================================================================================
// Cannot-render fallback (used by per-kind renderInner when build fails)
// =================================================================================================

export function CannotRender({ kind, message }: { kind: string; message: string }): ReactNode {
  return (
    <Notification color="yellow" withCloseButton={false} title={`Cannot render ${kind}`}>
      {message}
    </Notification>
  )
}
