// SPDX-License-Identifier: Apache-2.0
/**
 * DebugRoot — body of the `debug` tab. Hosts:
 *
 *   • "UI Components" — DebugTreePane + per-test demo pages
 *     (iframe-widget / ui-form / ui-view / ui-table / etc.).
 *   • "KTSP Browser"   — the in-universe slot inspector + invoker,
 *     equivalent to `ktw-utils.exe inspect` but rendered in-app.
 *   • Resources / Tasks / Skills / Types / Bus-RPC / Workspace.
 *
 * The selected section and demo page live in the tab's DebugTabPayload,
 * so leaving the tab and coming back restores the view.
 */

import { Box, Tabs } from '@mantine/core'
import { Split } from '@gfazioli/mantine-split-pane'
import {
  IconActivity,
  IconBrain,
  IconBuildingWarehouse,
  IconChecklist,
  IconLayoutGrid,
  IconPackage,
  IconShape,
  IconWand,
  IconBolt,
  IconToggleRight,
} from '@tabler/icons-react'

import type { DebugTabPayload } from '../../store/tabsSlice'
import { DebugTreePane } from './DebugTreePane'
import { DebugTestHost } from './DebugTestHost'
import { KTSPBrowser } from './KTSPBrowser'
import { TypeBrowserPanel } from './TypeBrowserPanel'
import { ContributionBrowserPanel } from './ContributionBrowserPanel'
import { DebugBusTracePanel } from './DebugBusTracePanel'
import { DebugWorkspaceStatePanel } from './DebugWorkspaceStatePanel'
import { EventsPanel } from './EventsPanel'
import { DisplaySwitchesPanel } from './DisplaySwitchesPanel'
import { useT } from '../../i18n'

type DebugTab =
  | 'ui-components'
  | 'ktsp-browser'
  | 'contributions'
  | 'type-browser'
  | 'bus-trace'        // cwm-v3 UX restructure 2026-06-06: was a top-level tab; merged here.
  | 'tasks-inventory'  // cwm-v3 UX restructure 2026-06-06: was a top-level tab; merged here.
  | 'workspace-state'  // cwm-v3 UX restructure 2026-06-06: was a top-level tab; merged here.
  | 'skills'           // cwm-v3 UX restructure 2026-06-06: moved here from the Quick Actions card grid.
  | 'events'           // the declared event types and their subscribers
  | 'display-switches' // every increment display switch, on or off as the GUI started

interface DebugRootProps {
  payload:  DebugTabPayload
  onChange: (payload: DebugTabPayload) => void
}

export function DebugRoot({ payload, onChange }: DebugRootProps) {
  const tab        = (payload.section as DebugTab | undefined) ?? 'ui-components'
  const activeTest = payload.test ?? null
  const t          = useT('shell')

  return (
    <Box style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Tabs
        value={tab}
        onChange={(v) => v && onChange({ ...payload, section: v })}
        variant="outline"
        styles={{
          tab: { fontSize: 13 },
        }}
      >
        <Tabs.List>
          <Tabs.Tab value="ui-components" leftSection={<IconLayoutGrid size={14} />}>
            UI Components
          </Tabs.Tab>
          <Tabs.Tab value="ktsp-browser" leftSection={<IconBrain size={14} />}>
            KTSP Browser
          </Tabs.Tab>
          <Tabs.Tab value="contributions" leftSection={<IconPackage size={14} />}>
            Resources
          </Tabs.Tab>
          {/* cwm-v3 UX restructure 2026-06-07: Tasks + Skills placed
              right after Contributions so the catalog-like surfaces
              cluster visually (Contributions / Tasks / Skills /
              Types are all "list of things in the catalog"). */}
          <Tabs.Tab value="tasks-inventory" leftSection={<IconChecklist size={14} />}>
            Tasks
          </Tabs.Tab>
          <Tabs.Tab value="skills" leftSection={<IconWand size={14} />}>
            Skills
          </Tabs.Tab>
          <Tabs.Tab value="type-browser" leftSection={<IconShape size={14} />}>
            Types
          </Tabs.Tab>
          <Tabs.Tab value="events" leftSection={<IconBolt size={14} />}>
            Events
          </Tabs.Tab>
          <Tabs.Tab value="display-switches" leftSection={<IconToggleRight size={14} />} data-element="debug-tab-display-switches">
            {t('displaySwitches.tab')}
          </Tabs.Tab>
          <Tabs.Tab value="bus-trace" leftSection={<IconActivity size={14} />}>
            Bus / RPC
          </Tabs.Tab>
          <Tabs.Tab value="workspace-state" leftSection={<IconBuildingWarehouse size={14} />}>
            Workspace
          </Tabs.Tab>
        </Tabs.List>
      </Tabs>

      <Box style={{ flex: 1, minHeight: 0 }}>
        {tab === 'ui-components' && (
          <Split style={{ height: '100%' }}>
            <Split.Pane initialWidth={240} minWidth={180} maxWidth={360}>
              <DebugTreePane
                activeTest={activeTest}
                onSelect={(test) => onChange({ ...payload, test })}
              />
            </Split.Pane>
            <Split.Resizer />
            <Split.Pane grow>
              <DebugTestHost test={activeTest} />
            </Split.Pane>
          </Split>
        )}

        {tab === 'ktsp-browser' && <KTSPBrowser />}
        {tab === 'contributions' && <ContributionBrowserPanel />}
        {tab === 'type-browser' && <TypeBrowserPanel />}
        {tab === 'bus-trace' && <DebugBusTracePanel />}
        {/* Tasks + Skills are the catalog's `task` / `skill` contributions —
            same tree + metadata/body detail as Resources/Types, scoped to
            one type. Replaces the old card-grid panels (which decoded the
            now-body-free index → 0 rows). */}
        {tab === 'tasks-inventory' && <ContributionBrowserPanel typeFilter="task" />}
        {tab === 'workspace-state' && <DebugWorkspaceStatePanel />}
        {tab === 'skills' && <ContributionBrowserPanel typeFilter="skill" />}
        {tab === 'events' && <EventsPanel />}
        {tab === 'display-switches' && <DisplaySwitchesPanel />}
      </Box>
    </Box>
  )
}
