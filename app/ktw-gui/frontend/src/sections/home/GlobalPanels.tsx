// SPDX-License-Identifier: Apache-2.0
/**
 * GlobalPanels — the declared panels marked `global` (object-pane
 * GlobalPanels), as tabs of the Global strip. There is no object: each panel
 * sits in a shell with a TARGET selector (every module, or one module) and a
 * refresh; a source panel is called with `{target}` — the same declaration a
 * module's pane calls with `{id}` — so one panel serves both levels.
 */

import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { ActionIcon, Box, Group, Select, Stack, Text, ThemeIcon, Title, Tooltip } from '@mantine/core'
import { IconLayoutDashboard, IconRefresh, IconTarget } from '@tabler/icons-react'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { WorkspaceObjectsTree } from '../../../wailsjs/go/main/App'
import { runMethod } from '../../components/compositions/hostServices'
import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { RESTART_BACKOFF_MS, useHostLoad, type HostLoad, type PartLoad } from '../../hooks/useHostLoad'
import { RenderDefView, SourcePanel } from '../workspace/RenderDefView'
import { pageIdentity } from '../workspace/resolvePanel'
import { recall, remember } from '../workspace/panelMemory'
import type { StripEntry } from '../workspace/VerticalTabBody'
import type { RenderDef } from '../../generated/render-def'
import { moduleOfId, type PanelRefresh } from '../../hooks/usePanelRefresh'
import { useT } from '../../i18n'
import { contribURI, useCT } from '../../i18n/contrib'
import { EmptyInvite } from '../workspace/EmptyInvite'
import { visibleObjects } from '../workspace/systemModules'
import { useDisplayLevel } from '../../hooks/useUniverseGui'
import type { PanelEmpty } from '../workspace/emptyState'

/** One global panel, as GlobalPanels answers it. */
export interface GlobalPanel {
  id: string
  name: string
  description?: string
  icon?: { tabler?: string }
  order: number
  kind: 'source' | 'content'
  source?: string
  content?: RenderDef
  /** The method answering the targets it offers ({targets: [{value, label, group}]}). */
  targets?: string
  /** Which object changes re-read it while shown (usePanelRefresh). */
  refresh?: PanelRefresh
  /** Its declared input, under the target its source is called with. */
  input?: Record<string, unknown>
  /** What it shows when its source answers nothing: a sentence and one action. */
  empty?: PanelEmpty
}

type Choice = { value: string; label: string }
type Options = Array<Choice | { group: string; items: Choice[] }>

const GLOBAL_PANELS = 'ktw-agents:object-pane/global-panels'

/** The load of the Global strip's declared panels. A failure is asked again
 *  over a hub restart's window (a deploy restarts the hub under an open GUI),
 *  then shown as the error card with « Réessayer » - never as no panels. */
export function globalPanelsPart(universeName: string): PartLoad<GlobalPanel[]> {
  return {
    universe: universeName, key: 'global-panels', retryErrors: true, backoff: RESTART_BACKOFF_MS,
    load: async (signal) => ((await runMethod(universeName, GLOBAL_PANELS, '{}', signal)) as { panels?: GlobalPanel[] } | null)?.panels ?? [],
  }
}

/** The Global strip's declared panels, re-read when the universe changes,
 *  through the host's one load. A universe with no object pane
 *  has none; a load that failed keeps its error and its « Réessayer »
 *  (`part`), drawn by GlobalPanelsFailed. */
export function useGlobalPanels(universeName: string | null | undefined, revision: unknown): { panels: GlobalPanel[]; part: HostLoad<GlobalPanel[]> } {
  const part = useHostLoad<GlobalPanel[]>({ ...globalPanelsPart(universeName ?? ''), universe: universeName, revision })
  return { panels: part.data ?? NO_PANELS, part }
}

/** GlobalPanelsFailed is the error card of a failed panels load, with
 *  « Réessayer », drawn where the panels' entries would be; null
 *  while the load answers. */
export function GlobalPanelsFailed({ part }: { part: Pick<HostLoad<unknown>, 'error' | 'retry'> }): ReactNode {
  if (!part.error) return null
  return <SectionFailed id="global-panels" message={part.error} onRetry={part.retry} />
}

const NO_PANELS: GlobalPanel[] = []

/** The strip entries of the global panels: their ids are the panels'. */
export function globalPanelEntries(panels: GlobalPanel[]): StripEntry[] {
  return panels.map((p) => {
    const Icon = tablerIcon(p.icon?.tabler, IconLayoutDashboard)
    return { id: p.id, label: p.name, accent: 'indigo', icon: <Icon size={14} /> }
  })
}

const ALL = '__all__'

/** GlobalPanelView — one global panel in its shell: the target, a refresh,
 *  then what the panel shows. */
export function GlobalPanelView({ panel, universeName }: { panel: GlobalPanel; universeName: string }) {
  const t = useT('home')
  // Its texts in the person's language, from the pack's translations.
  const ct = useCT()
  const panelURI = contribURI('object-panel', panel.id.replace(/^(home|panel):/, ''))
  // Its target and its page's query outlive a switch to another tab: back
  // from an object opened here, the panel is as the person left it.
  const memo = `global|${universeName}|${panel.id}`
  const [target, setTarget] = useState<string>(() => recall(memo, 'target', ALL))
  useEffect(() => { remember(memo, 'target', target) }, [memo, target])
  const queries = recall<Record<string, Record<string, unknown>>>(memo, 'queries', {})
  const keepQuery = useCallback((q: Record<string, unknown>) => {
    remember(memo, 'queries', { ...recall<Record<string, Record<string, unknown>>>(memo, 'queries', {}), [target]: q })
  }, [memo, target])
  const [revision, setRevision] = useState(0)
  const level = useDisplayLevel()

  // The panel's own targets (modules, releases…), grouped; else - no targets
  // method, or it failed - every module. Both read through the host's one load.
  const targets = useHostLoad<Options>({
    universe: universeName, key: `targets|${panel.targets ?? ''}`, enabled: !!panel.targets,
    load: async (signal) => {
      const out = (await runMethod(universeName, panel.targets!, '{}', signal)) as { targets?: Array<{ value: string; label: string; group?: string }> } | null
      const top: Choice[] = []
      const groups = new Map<string, Choice[]>()
      for (const t of out?.targets ?? []) {
        const c = { value: t.value === '' ? ALL : t.value, label: t.label }
        if (!t.group) top.push(c)
        else groups.set(t.group, [...(groups.get(t.group) ?? []), c])
      }
      return [...top, ...[...groups].map(([group, items]) => ({ group, items }))]
    },
  })
  const declared = targets.data ?? null

  // The modules offered when the panel declares no targets: a failed read is
  // the error card with « Réessayer », never an empty module list.
  const objects = useHostLoad<Awaited<ReturnType<typeof WorkspaceObjectsTree>>>({
    universe: universeName, key: 'objects-tree', retryErrors: true,
    load: async () => (await WorkspaceObjectsTree(universeName)) ?? [],
  })
  const modules = useMemo(() => visibleObjects(objects.data ?? [], level)
    .filter((o) => o.level === 'module') // its type's level capability, never a type name
    .map((o) => ({ value: o.id, label: o.title || o.id }))
    .sort((a, b) => a.label.localeCompare(b.label)), [objects.data, level])

  const options: Options = useMemo(() => declared ?? [{ value: ALL, label: t('globalPanel.allModules') }, ...modules], [declared, modules, t])
  const args = useMemo(() => ({ target: target === ALL ? '' : target }), [target])
  const refresh = useCallback(() => setRevision((n) => n + 1), [])
  const PanelIcon = tablerIcon(panel.icon?.tabler, IconLayoutDashboard)

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group justify="space-between" px="lg" py="md" wrap="nowrap" style={{
        borderBottom: '1px solid var(--mantine-color-default-border)',
        background: 'linear-gradient(110deg, var(--mantine-color-violet-light) 0%, var(--mantine-color-blue-light) 45%, transparent 85%)',
      }}>
        <Group gap="md" wrap="nowrap" style={{ minWidth: 0 }}>
          <ThemeIcon size={46} radius="md" variant="gradient" gradient={{ from: 'violet', to: 'blue', deg: 135 }}>
            <PanelIcon size={26} />
          </ThemeIcon>
          <Box style={{ minWidth: 0 }}>
            <Title order={3} fw={700} style={{ lineHeight: 1.15 }}>{panel.name}</Title>
            {panel.description && <Text size="sm" c="dimmed" truncate>{panel.description}</Text>}
          </Box>
        </Group>
        <Group gap="xs" wrap="nowrap">
          <Select size="sm" w={280} aria-label={t('globalPanel.target')} data={options} value={target}
            leftSection={<IconTarget size={16} />}
            onChange={(v) => setTarget(v ?? ALL)} allowDeselect={false} searchable />
          <Tooltip label={t('globalPanel.refresh')} withArrow>
            <ActionIcon variant="light" size="lg" aria-label={t('globalPanel.refresh')} onClick={refresh}><IconRefresh size={18} /></ActionIcon>
          </Tooltip>
        </Group>
      </Group>
      {objects.error && !declared && (
        <Box px="lg" pt="sm"><SectionFailed id="global-panel-modules" message={objects.error} onRetry={objects.retry} /></Box>
      )}
      <Box style={{ flex: 1, minHeight: 0 }}>
        {panel.kind === 'source' && panel.source ? (
          <SourcePanel key={target} source={panel.source} content={panel.content} args={args} input={panel.input}
            initialQuery={queries[target]} onQueried={keepQuery}
            revision={revision} refresh={panel.refresh} module={target === ALL ? '' : moduleOfId(target)}
            universeName={universeName} objectId="" objectName={panel.name}
            empty={panel.empty ? (
              <EmptyInvite empty={panel.empty} universeName={universeName} onDone={refresh}
                text={ct(panelURI, 'empty.text', panel.empty.text)}
                label={panel.empty.label ? ct(panelURI, 'empty.label', panel.empty.label) : undefined} />
            ) : undefined} />
        ) : panel.content ? (
          <RenderDefView key={pageIdentity(universeName, '', undefined, panel.id)} panel={panel.id}
            def={panel.content} universeName={universeName} objectId="" objectName={panel.name} />
        ) : (
          <Text size="sm" c="dimmed" p="md">{t('globalPanel.nothing')}</Text>
        )}
      </Box>
    </Stack>
  )
}
