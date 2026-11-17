// SPDX-License-Identifier: Apache-2.0
/**
 * DisplaySwitchesPanel — the debugger's « Interrupteurs d'affichage » tab:
 * ONE read-only table, one row per increment display switch
 * - Variable | Incrément (a link) | Ce qu'il change |
 * État as this GUI started - then the dev front-end's VITE_KTW_DISPLAY_SWITCHES.
 * The switches are the features CWM tags display-switch (cwm object-search),
 * each one's increment the increment taking it (cwm edge-list, takes); the
 * variable and its state come from the backend (DisplaySwitchStates, read from
 * this process's environment - the hub's packs read the same variable from the
 * same case environment). No toggling: a switch is turned on only by its
 * variable.
 */
import { useEffect, useState } from 'react'
import { Alert, Anchor, Badge, Box, Code, Loader, ScrollArea, Table, Text, Title } from '@mantine/core'

import { useAppStore } from '../../store'
import { useT, type TFn } from '../../i18n'
import { DisplaySwitchStates, WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { openObjectTab } from '../workspace/openObjectTab'
import {
  DEV_SWITCHES_VARIABLE, incrementLabel, incrementOf, switchRows,
  type SwitchFeature, type SwitchRow, type SwitchState,
} from './displaySwitchRows'

const SEARCH = 'ktw-agents:cwm/object-search'
const EDGES = 'ktw-agents:cwm/edge-list'

/** The dev front-end's list, read in a dev build only (devSwitch.ts). */
function devList(): { enabled: boolean; value: string } {
  const env = (import.meta as { env?: Record<string, unknown> }).env
  return { enabled: !!env?.DEV, value: String(env?.VITE_KTW_DISPLAY_SWITCHES ?? '') }
}

async function run<T>(universe: string, uri: string, input: unknown): Promise<T> {
  const r = await WorkspaceRunAction(universe, uri, JSON.stringify(input))
  if (r.error || !r.value) throw new Error(r.error || 'No answer from ' + uri)
  return JSON.parse(r.value) as T
}

/** A feature as cwm object-search answers it: its lifecycle says whether the switch is built. */
interface FoundFeature { id: string; name?: string; capabilities?: { lifecycle?: { state?: string } } }

/** Reads the switches: their features, the increment of each, their states. */
export async function readSwitchRows(universe: string): Promise<SwitchRow[]> {
  const found = await run<{ objects?: FoundFeature[] }>(universe, SEARCH, { capabilities: ['display-switch'], limit: 500 })
  const features: SwitchFeature[] = (found.objects ?? []).map((o) => ({ id: o.id, name: o.name, state: o.capabilities?.lifecycle?.state }))
  const incrementByFeature: Record<string, string> = {}
  await Promise.all(features.map(async (f) => {
    const e = await run<{ edges?: { from: string }[] }>(universe, EDGES, { object_id: f.id, kind: 'takes', direction: 'in' })
    incrementByFeature[f.id] = incrementOf((e.edges ?? []).map((x) => x.from))
  }))
  const increments = [...new Set(Object.values(incrementByFeature).filter(Boolean))]
  const states: SwitchState[] = increments.length ? (await DisplaySwitchStates(increments)) ?? [] : []
  return switchRows(features, incrementByFeature, states, devList())
}

/**
 * Column widths: Variable, Incrément and État never wrap and
 * take the width of their longest value (width 1% shrinks a column to its
 * content in an auto-layout table); « Ce qu'il change » takes the rest. The
 * state badge never clips its label: Mantine's Badge hides its overflow, which
 * lets a squeezed last column cut « éteint » to « é… » - its overflow is shown
 * so the cell's minimum width is the whole label.
 */
const fitColumn = { whiteSpace: 'nowrap', width: '1%' } as const
/**
 * The dev row's list of switches wraps inside the État cell:
 * only the badge keeps the cell's nowrap, so a long VITE_KTW_DISPLAY_SWITCHES
 * never widens the column and squeezes « Ce qu'il change ».
 */
const listWraps = { whiteSpace: 'normal' } as const
const badgeWhole = { root: { overflow: 'visible', maxWidth: 'none' }, label: { overflow: 'visible', textOverflow: 'clip' } } as const

/** The table itself, from its rows. */
export function DisplaySwitchesTable({ rows, t, onOpen }: { rows: SwitchRow[]; t: TFn; onOpen: (id: string) => void }) {
  return (
    <Table striped withTableBorder data-element="display-switches-table">
      <Table.Thead>
        <Table.Tr>
          <Table.Th style={fitColumn}>{t('displaySwitches.variable')}</Table.Th>
          <Table.Th style={fitColumn}>{t('displaySwitches.increment')}</Table.Th>
          <Table.Th>{t('displaySwitches.changes')}</Table.Th>
          <Table.Th style={fitColumn}>{t('displaySwitches.state')}</Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {rows.map((r) => (
          <Table.Tr key={r.dev ? DEV_SWITCHES_VARIABLE : r.feature} data-element={`display-switch-${r.variable || r.feature}`}>
            <Table.Td style={fitColumn}><Code>{r.variable || '—'}</Code></Table.Td>
            <Table.Td style={fitColumn}>
              {r.dev ? (
                <Text size="sm" c="dimmed">{t('displaySwitches.devGui')}</Text>
              ) : r.increment ? (
                <Anchor size="sm" component="button" onClick={() => onOpen(r.increment)} data-element={`display-switch-open-${r.increment}`}>
                  {incrementLabel(r.increment)}
                </Anchor>
              ) : (
                <Text size="sm" c="dimmed">{t('displaySwitches.noIncrement')}</Text>
              )}
            </Table.Td>
            <Table.Td>
              <Text size="sm" title={r.dev ? undefined : r.name}>{r.dev ? t('displaySwitches.devLine') : r.changes}</Text>
            </Table.Td>
            <Table.Td style={fitColumn}>
              <Badge color={!r.built ? 'yellow' : r.on ? 'green' : 'gray'} variant="light" styles={badgeWhole} data-element={`display-switch-state-${r.variable || r.feature}`}>
                {t(!r.built ? 'displaySwitches.notBuilt' : r.on ? 'displaySwitches.on' : 'displaySwitches.off')}
              </Badge>
              {r.dev && r.on && <Text size="xs" c="dimmed" style={listWraps} data-element="display-switch-dev-listed">{r.listed?.join(', ')}</Text>}
            </Table.Td>
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  )
}

export function DisplaySwitchesPanel() {
  const t = useT('shell')
  const universeName = useAppStore((s) => s.displayedUniverseName)
  const [rows, setRows] = useState<SwitchRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!universeName) return
    let cancelled = false
    setRows(null)
    setError(null)
    readSwitchRows(universeName).then(
      (r) => { if (!cancelled) setRows(r) },
      (e) => { if (!cancelled) setError(String(e)) },
    )
    return () => { cancelled = true }
  }, [universeName])

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md" data-element="display-switches">
        <Title order={4}>{t('displaySwitches.title')}</Title>
        <Text size="sm" c="dimmed" mb="sm">{t('displaySwitches.intro')}</Text>
        {error && <Alert color="red">{error}</Alert>}
        {!error && rows === null && <Loader size="sm" aria-label={t('displaySwitches.loading')} />}
        {rows && rows.length === 1 && <Text size="sm" c="dimmed" mb="xs">{t('displaySwitches.none')}</Text>}
        {rows && (
          <DisplaySwitchesTable rows={rows} t={t}
            onOpen={(id) => universeName && openObjectTab(universeName, { id })} />
        )}
      </Box>
    </ScrollArea>
  )
}
