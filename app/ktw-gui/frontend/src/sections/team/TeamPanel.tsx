// SPDX-License-Identifier: Apache-2.0
/**
 * TeamPanel — per-session run view with TWO LENSES:
 *
 *  - TASKS: the delegation task tree (React Flow graph of task cards; click
 *    a card → detail panel with input / output / run ID / resources +
 *    journal drill). Built from team.child_* events live, or cold from the
 *    session journal via GetSessionTaskTree.
 *  - TEAM: the collaboration narrative (team created / members / messages /
 *    state changes) built from team.team_event — the team as a first-class
 *    entity, not a task view.
 *
 * State is partitioned per root run (see teamSlice): a new run never
 * inherits the previous run's nodes; older runs stay selectable.
 */
import { useEffect } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  ReactFlowProvider,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { ActionIcon, Badge, Center, Group, SegmentedControl, Select, Stack, Text, Tooltip } from '@mantine/core'
import { IconRefresh, IconBolt, IconSitemap } from '@tabler/icons-react'
import { useState } from 'react'
import { AgentNode } from './AgentNode'
import { useTeamLiveSource, shortRunID } from './useTeamLiveSource'
import { useTeamStore, selectViewScope, selectRunList, type ColdTaskTree } from '../../store/teamSlice'
import { useAppStore } from '../../store'
import { WorkflowStatusBar } from './WorkflowStatusBar'
import { MailboxPanel } from './MailboxPanel'
import { TaskDetailPanel } from './TaskDetailPanel'
import { EdgeDetailPanel } from './EdgeDetailPanel'
import { TeamNarrativePanel } from './TeamNarrativePanel'
import { LedgerPanel } from './LedgerPanel'
import { GetSessionTaskTree } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

const nodeTypes = { agentNode: AgentNode }

function TeamFlowInner({ coldSessionId }: { coldSessionId?: string }) {
  const t = useT('team')
  const [lens, setLens] = useState<string>('tasks')
  const { nodes, edges, reset } = useTeamLiveSource()
  const scope = useTeamStore(selectViewScope)
  const runList = useTeamStore(selectRunList)
  const viewRootRunId = useTeamStore((s) => s.viewRootRunId ?? s.activeRootRunId)
  const selectRun = useTeamStore((s) => s.selectRun)
  const selectedTeammate = useTeamStore((s) => s.selectedTeammate)
  const selectTeammate = useTeamStore((s) => s.selectTeammate)
  const selectedTaskRunId = useTeamStore((s) => s.selectedTaskRunId)
  const selectTask = useTeamStore((s) => s.selectTask)
  const selectedEdgeKey = useTeamStore((s) => s.selectedEdgeKey)
  const selectEdge = useTeamStore((s) => s.selectEdge)

  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const runningSessionId = useAppStore((s) => s.runningSessionId)

  // The run selector must list ONLY the runs of the CURRENT session — the
  // global team store also holds scopes left by previously-viewed sessions
  // (e.g. a cold "uc-130-task (journal)"), which must not leak into this
  // session's selector. Filter by the effective session id (cold view's
  // sessionId, else the live running session).
  const effectiveSessionId = coldSessionId ?? runningSessionId
  const scopedRunList = effectiveSessionId
    ? runList.filter((r) => r.sessionId === effectiveSessionId || r.rootRunId === `session:${effectiveSessionId}`)
    : runList

  // Cold viewer (finished SessionViewer): coldSessionId is the SOLE hydration
  // authority. Re-fetch + re-point on every sessionId change so switching
  // sessions never shows a previous session's tree (no manual refresh). This
  // path replaces the live runningSessionId reconstruction below — they must
  // not both fire, or the live session's tree leaks into the cold view.
  useEffect(() => {
    if (!coldSessionId || !displayedUniverseName) return
    let cancelled = false
    GetSessionTaskTree(displayedUniverseName, coldSessionId)
      .then((res) => {
        if (cancelled || !res?.tree) return
        useTeamStore.getState().hydrateFromTaskTree(coldSessionId, res.tree as unknown as ColdTaskTree)
        useTeamStore.getState().selectRun(`session:${coldSessionId}`)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [coldSessionId, displayedUniverseName])

  // Live mode: point the view at THIS running session's scope (else clear the
  // selection to follow the active run). `viewRootRunId` is a sticky global —
  // without this, a selection left by a prior COLD session view keeps the live
  // Teams tab showing that old session's tree until a manual refresh.
  useEffect(() => {
    if (coldSessionId || !runningSessionId) return
    const st = useTeamStore.getState()
    let target: string | null = null
    for (const [rootId, sc] of st.runs) {
      if (sc.sessionId === runningSessionId) { target = rootId; break }
    }
    st.selectRun(target)
  }, [coldSessionId, runningSessionId])

  // Live reconstruction: when the LIVE panel opens with no data for the current
  // running session (reopened GUI / reconnected session), rebuild from the
  // journal. Disabled in cold-viewer mode (coldSessionId drives hydration).
  const scopeEmpty = !scope || (scope.children.size === 0 && scope.teamEvents.length === 0)
  useEffect(() => {
    if (coldSessionId) return
    if (!scopeEmpty || !displayedUniverseName || !runningSessionId) return
    let cancelled = false
    GetSessionTaskTree(displayedUniverseName, runningSessionId)
      .then((res) => {
        if (cancelled || !res?.tree) return
        useTeamStore.getState().hydrateFromTaskTree(runningSessionId, res.tree as unknown as ColdTaskTree)
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [coldSessionId, scopeEmpty, displayedUniverseName, runningSessionId])

  const doneCount = nodes.filter(n => n.data.status === 'done').length
  const runningCount = nodes.filter(n => n.data.status === 'running').length
  const calls = scope?.calls ?? []
  // Lenses present themselves only when they have content: Team for team runs
  // (create-team / spawn-teammate), Ledger for Magentic shared-state runs.
  // Tasks is always available. The toggle hides entirely when only Tasks fits.
  const hasTeam = (scope?.teamEvents.length ?? 0) > 0
  const hasLedger = (scope?.ledger.length ?? 0) > 0
  const lensOptions = [
    { label: t('panel.lens.tasks'), value: 'tasks' },
    ...(hasTeam ? [{ label: t('panel.lens.team'), value: 'team' }] : []),
    ...(hasLedger ? [{ label: t('panel.lens.ledger'), value: 'ledger' }] : []),
  ]
  const available = new Set(lensOptions.map((o) => o.value))
  const activeLens = available.has(lens) ? lens : 'tasks'

  return (
    <Stack gap={0} style={{ flex: 1, height: '100%' }}>
      <WorkflowStatusBar />

      {/* Toolbar */}
      <Group
        gap="xs"
        px="sm"
        py={6}
        style={{
          borderBottom: '1px solid var(--mantine-color-dark-4)',
          flexShrink: 0,
        }}
      >
        {lensOptions.length > 1 && (
          <SegmentedControl
            size="xs"
            value={activeLens}
            onChange={setLens}
            data={lensOptions}
          />
        )}

        {/* Run selector — one tree per top-level run; a new run never merges
            into the previous one. Hidden until there is more than one run. */}
        {scopedRunList.length > 1 && (
          <Select
            size="xs"
            w={190}
            value={viewRootRunId}
            onChange={(v) => selectRun(v)}
            data={scopedRunList.map((r) => ({
              value: r.rootRunId,
              label: `${r.workflow?.name || shortRunID(r.rootRunId)}${r.cold ? ` (${t('panel.journal')})` : ''}`,
            }))}
            allowDeselect={false}
          />
        )}

        <ActionIcon size="sm" variant="subtle" color="gray" onClick={reset} title={t('panel.clear')}>
          <IconRefresh size={14} />
        </ActionIcon>

        <Badge size="xs" color="teal" variant="light">{scope?.cold ? t('panel.journal') : t('panel.live')}</Badge>

        <div style={{ flex: 1 }} />

        {calls.length > 0 && (
          <Tooltip
            withArrow
            label={calls.map((c) => t('panel.callTip', { node: c.node, uri: c.uri, outcome: c.outcome, durationMs: c.durationMs, attempt: c.attempt })).join('\n')}
            multiline
          >
            <Badge size="xs" color="violet" variant="light" leftSection={<IconBolt size={10} />}>
              {t('panel.calls', { count: calls.length })}
            </Badge>
          </Tooltip>
        )}
        <Badge size="xs" color="blue" variant="light">{t('panel.running', { count: runningCount })}</Badge>
        <Badge size="xs" color="green" variant="light">{t('panel.done', { count: doneCount })}</Badge>
        <Badge size="xs" color="gray" variant="light">{t('panel.tasks', { count: nodes.length })}</Badge>
      </Group>

      {/* Body: graph or narrative + (optional) side panel */}
      <Group gap={0} style={{ flex: 1, alignItems: 'stretch', overflow: 'hidden' }} wrap="nowrap">
        <div style={{ flex: 1, overflow: 'hidden' }}>
          {activeLens === 'tasks' ? (
            nodes.length === 0 ? (
              <Center style={{ height: '100%' }}>
                <Stack align="center" gap="sm" style={{ opacity: 0.4 }}>
                  <IconSitemap size={40} />
                  <Text size="sm" c="dimmed">{t('panel.empty')}</Text>
                  <Text size="xs" c="dimmed">{t('panel.emptyHint')}</Text>
                </Stack>
              </Center>
            ) : (
              <ReactFlow
                nodes={nodes as any}
                edges={edges as any}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.2 }}
                minZoom={0.3}
                maxZoom={2}
                proOptions={{ hideAttribution: true }}
                style={{ background: 'var(--mantine-color-dark-8)' }}
                nodesDraggable={false}
                nodesConnectable={false}
                elementsSelectable={true}
                onNodeClick={(_e, node) => {
                  // Open the task detail panel keyed by runId (toggles off on re-click).
                  selectTask(node.id === selectedTaskRunId ? null : node.id)
                }}
                onEdgeClick={(_e, edge) => {
                  // Click a LINK → show what it carried: a delegation edge shows
                  // input-down / output-up; a comm edge lists the messages.
                  selectEdge(edge.id === selectedEdgeKey ? null : edge.id)
                }}
              >
                <Background color="var(--mantine-color-dark-5)" gap={20} size={1} />
                <Controls
                  showInteractive={false}
                  style={{ background: 'var(--mantine-color-dark-6)', borderColor: 'var(--mantine-color-dark-4)' }}
                />
              </ReactFlow>
            )
          ) : activeLens === 'team' ? (
            <TeamNarrativePanel />
          ) : (
            <LedgerPanel />
          )}
        </div>

        {/* Side panel: task detail (node click), edge detail (link click),
            or teammate mailbox. */}
        {scope && selectedTaskRunId && activeLens === 'tasks' && (
          <div style={{
            width: 340,
            borderLeft: '1px solid var(--mantine-color-dark-4)',
            background: 'var(--mantine-color-dark-7)',
            overflowY: 'auto',
            flexShrink: 0,
          }}>
            <TaskDetailPanel scope={scope} runId={selectedTaskRunId} onClose={() => selectTask(null)} />
          </div>
        )}
        {scope && selectedEdgeKey && activeLens === 'tasks' && (
          <div style={{
            width: 340,
            borderLeft: '1px solid var(--mantine-color-dark-4)',
            background: 'var(--mantine-color-dark-7)',
            overflowY: 'auto',
            flexShrink: 0,
          }}>
            <EdgeDetailPanel scope={scope} edgeKey={selectedEdgeKey} onClose={() => selectEdge(null)} />
          </div>
        )}
        {selectedTeammate && (
          <div style={{
            width: 320,
            borderLeft: '1px solid var(--mantine-color-dark-4)',
            background: 'var(--mantine-color-dark-7)',
            overflowY: 'auto',
            flexShrink: 0,
          }}>
            <MailboxPanel teammate={selectedTeammate} onClose={() => selectTeammate(null)} />
          </div>
        )}
      </Group>
    </Stack>
  )
}

export function TeamPanel({ coldSessionId }: { coldSessionId?: string } = {}) {
  return (
    <ReactFlowProvider>
      <TeamFlowInner coldSessionId={coldSessionId} />
    </ReactFlowProvider>
  )
}
