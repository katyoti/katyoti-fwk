// SPDX-License-Identifier: Apache-2.0
/**
 * useTeamLiveSource — returns React Flow nodes/edges derived from the DISPLAYED
 * run scope (live team.* events, or cold-hydrated from the session journal).
 * No data → empty graph (the panel renders its own empty state).
 */
import { useTeamStore, selectViewScope, type ChildInfo } from '../../store/teamSlice'
import type { AgentNodeData, AgentStatus } from './teamNodeTypes'

// React Flow's Node generic requires Record<string, unknown> for the data slot;
// AgentNodeData is structurally compatible but lacks an index signature, so we
// type the hook return loosely and the panel renders via `as any`.
type LooseNode = { id: string; type: string; data: AgentNodeData; position: { x: number; y: number } }
type LooseEdge = { id: string; source: string; target: string; animated?: boolean; label?: string; data?: unknown; style?: Record<string, unknown> }

interface LiveSource {
  nodes: LooseNode[]
  edges: LooseEdge[]
  reset: () => void
}

export function useTeamLiveSource(): LiveSource {
  const scope = useTeamStore(selectViewScope)

  const empty: LiveSource = { nodes: [], edges: [], reset: () => useTeamStore.getState().reset() }
  if (!scope || scope.children.size === 0) {
    return empty
  }

  const children = scope.children
  const edgesRaw = scope.edges

  // The workflow root (coordinator) is NOT a child — it lives in
  // scope.workflow. Render it as the top node so the tree has a single apex
  // and the root→team edges (which the children carry as parentRunId) draw.
  const rootRunId = scope.workflow?.runId
  const hasRoot = !!rootRunId && [...children.values()].some((c) => c.parentRunId === rootRunId)

  // Valid graph node ids = every child, plus the workflow root (when shown).
  const nodeIds = new Set<string>(children.keys())
  if (hasRoot) nodeIds.add(rootRunId!)

  // Build nodes from children. Layout: simple level-based grid — group by
  // parentRunId, lay siblings out horizontally per depth level. Sufficient
  // for a few-children-per-parent topology; can swap to dagre/elk later.
  const nodes: LooseNode[] = []
  const edges: LooseEdge[] = []

  // Compute depth per node. The root is depth 0; a child whose parent is the
  // root (or whose parent isn't a node — a forest root) starts the next level.
  const depthByRunId = new Map<string, number>()
  if (hasRoot) depthByRunId.set(rootRunId!, 0)
  const rootChildren: ChildInfo[] = []
  for (const c of children.values()) {
    const parentIsRoot = hasRoot && c.parentRunId === rootRunId
    const parentMissing = !c.parentRunId || !children.has(c.parentRunId)
    if (parentIsRoot || parentMissing) {
      depthByRunId.set(c.runId, hasRoot ? 1 : 0)
      rootChildren.push(c)
    }
  }
  // BFS to compute depth for the rest.
  let frontier = rootChildren
  while (frontier.length > 0) {
    const next: ChildInfo[] = []
    for (const parent of frontier) {
      const parentDepth = depthByRunId.get(parent.runId) ?? 0
      for (const c of children.values()) {
        if (c.parentRunId === parent.runId && !depthByRunId.has(c.runId)) {
          depthByRunId.set(c.runId, parentDepth + 1)
          next.push(c)
        }
      }
    }
    frontier = next
  }

  // Group children by depth for x-positioning.
  const byDepth = new Map<number, ChildInfo[]>()
  for (const c of children.values()) {
    const d = depthByRunId.get(c.runId) ?? 0
    if (!byDepth.has(d)) byDepth.set(d, [])
    byDepth.get(d)!.push(c)
  }

  const Y_STEP = 180
  const X_STEP = 240
  if (hasRoot) {
    const wf = scope.workflow!
    nodes.push({
      id: rootRunId!,
      type: 'agentNode',
      position: { x: 0, y: 0 },
      data: workflowToNodeData(wf),
    })
  }
  for (const [depth, siblings] of byDepth.entries()) {
    siblings.forEach((c, i) => {
      const offset = (siblings.length - 1) / 2
      nodes.push({
        id: c.runId,
        type: 'agentNode',
        position: { x: (i - offset) * X_STEP, y: depth * Y_STEP },
        data: childToNodeData(c),
      })
    })
  }

  // Edges: parent → child. The LINK carries the COMMS RELATIONSHIP (the
  // parent's delegation intent — `commsMode`, e.g. agent-as-human). The child's
  // resolved SPAWN mode rides the NODE, not the link. Label priority:
  //   1. commsMode (the intent the parent expressed) → friendly label,
  //   2. else a non-default spawn mode (background / fork / coordinator / …),
  //   3. else nothing — a plain delegation arrow (a bare `worker` is noise).
  // Both modes are kept in `data` for the EdgeDetailPanel ("max info").
  for (const e of edgesRaw) {
    if (!nodeIds.has(e.parentRunId) || !nodeIds.has(e.childRunId)) continue
    const child = children.get(e.childRunId)!
    const mode = child.mode || 'worker'
    const comms = child.commsMode || ''
    const label = comms
      ? commsLabel(comms)
      : mode === 'worker'
        ? undefined
        : mode
    edges.push({
      id: `${e.parentRunId}->${e.childRunId}`,
      source: e.parentRunId,
      target: e.childRunId,
      animated: child.state === 'running',
      label,
      data: { mode, commsMode: comms, active: child.state === 'running' },
    })
  }

  // Comm edges: direct agent-to-agent messages (a lead-to-lead channel). A
  // `send-message` fires a team.message captured in scope.mailboxes (keyed by
  // recipient name); we draw a DISTINCT dashed link between the two run nodes,
  // tagged with the message count. Clicking it lists what the channel carried.
  const commCount = new Map<string, number>()
  for (const msgs of scope.mailboxes.values()) {
    for (const m of msgs) {
      const fromRun = scope.childrenByName.get(m.from)
      const toRun = scope.childrenByName.get(m.to)
      if (!fromRun || !toRun || !nodeIds.has(fromRun) || !nodeIds.has(toRun)) continue
      const key = `${fromRun}->${toRun}`
      commCount.set(key, (commCount.get(key) ?? 0) + 1)
    }
  }
  for (const [key, count] of commCount.entries()) {
    const [fromRun, toRun] = key.split('->')
    edges.push({
      id: `comm:${key}`,
      source: fromRun,
      target: toRun,
      animated: true,
      label: `💬 ${count}`,
      data: { kind: 'comm', count },
      style: { stroke: '#b197fc', strokeDasharray: '6 3', strokeWidth: 2 },
    })
  }

  return {
    nodes,
    edges,
    reset: () => useTeamStore.getState().reset(),
  }
}

const stateToStatus = (state: ChildInfo['state']): AgentStatus => {
  switch (state) {
    case 'running': return 'running'
    case 'done':    return 'done'
    case 'failed':  return 'error'
  }
}

const childToNodeData = (c: ChildInfo): AgentNodeData => ({
  // Top-level agent runs don't carry a Name in agent.session_start, so fall
  // back to a short form of the runID for a non-empty card label. Sub-agents
  // (delegated/spawned) DO have a name and use it directly.
  label: c.name || shortRunID(c.runId),
  role: roleFromMode(c.mode, c.kind),
  kind: c.kind || 'agent',
  mode: c.mode || 'worker',
  taskName: c.kind,
  status: stateToStatus(c.state),
  capability: '',  // not surfaced via team events; left blank for live cards
  turns: 0,
  summary: c.error ?? summaryFromOutput(c.output),
  toolCount: 0,
  promptCount: 0,
  sessionId: c.runId,
})

const workflowToNodeData = (wf: { runId: string; name: string; state: string; output?: unknown; error?: string }): AgentNodeData => ({
  label: wf.name || 'coordinator',
  role: 'coordinator',
  kind: 'workflow',
  mode: 'coordinator',
  taskName: 'workflow',
  status: wf.state === 'done' ? 'done' : wf.state === 'failed' ? 'error' : 'running',
  capability: '',
  turns: 0,
  summary: wf.error ?? summaryFromOutput(wf.output),
  toolCount: 0,
  promptCount: 0,
  sessionId: wf.runId,
})

export const shortRunID = (runId: string): string => {
  // run-deadbeef12345678 → run-deadbe… (8 chars after prefix)
  if (runId.startsWith('run-') && runId.length > 12) {
    return runId.slice(0, 12) + '…'
  }
  if (runId.startsWith('session:')) {
    return 'session'
  }
  return runId
}

const roleFromMode = (mode: string, kind: string): string => {
  // The REAL delegation mode drives the role; kind only as fallback.
  if (mode === 'coordinator') return 'coordinator'
  if (mode === 'team-leader') return 'team-lead'
  if (kind === 'workflow') return 'coordinator'
  if (kind === 'ledger') return 'team-lead'
  return 'worker'
}

// Friendly label for the comms relationship printed on a delegation LINK. Maps
// the raw delegate `mode` arg / resolved UserComms input to a human phrase; the
// wire value never leaks into the UI. Unknown values pass through normalized.
export const commsLabel = (comms: string): string => {
  switch (comms) {
    case 'agent_as_human':
    case 'agent-as-human':
    case 'via_agent':
      return 'agent-as-human'
    case 'bubble_to_parent':
      return '↑ asks parent'
    case 'via_leader':
      return 'via leader'
    case 'inbox':
      return 'inbox'
    case 'direct':
      return 'direct'
    case 'none':
      return 'no user channel'
    default:
      return comms.replace(/_/g, ' ')
  }
}

const summaryFromOutput = (output: unknown): string => {
  if (output == null) return ''
  if (typeof output === 'string') return output
  if (typeof output === 'object') {
    const o = output as Record<string, unknown>
    if (typeof o.summary === 'string') return o.summary
    if (o.dry_run === true) return `DRY_RUN (${o.child_kind ?? '?'})`
  }
  return ''
}
