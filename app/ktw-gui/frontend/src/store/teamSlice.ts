// SPDX-License-Identifier: Apache-2.0
/**
 * Team store — per-run session state derived from terminal-protocol team.*
 * events bridged from the task engine.
 *
 * State is PARTITIONED BY ROOT RUN: every event is routed to the RunScope
 * owning its runId (via runIndex), so two consecutive runs in one session
 * render as two distinct trees and never accumulate into one graph. The
 * panel displays one scope at a time (viewRootRunId, defaulting to the
 * most recently started run).
 *
 * Two lenses read from a scope:
 *  - TASK TREE: children + edges (+ calls) — what ran, with which mode,
 *    which input/output (input/output enriched by cold hydration from
 *    GetSessionTaskTree when the journal has them).
 *  - TEAM VIEW: teamEvents narrative (created / member-joined / message /
 *    state-changed / completed) + derived member states.
 *
 * Wired by `useAgentEvents` which calls the action functions on each
 * `team.*` event from Wails IPC.
 */

import { create } from 'zustand'

export type WorkflowState = 'running' | 'done' | 'failed'
export type ChildState = 'running' | 'done' | 'failed'

export interface WorkflowInfo {
  runId: string
  name: string
  state: WorkflowState
  startedAt: number  // epoch ms
  endedAt?: number   // set on done/failed
  output?: unknown
  error?: string
}

export interface ChildInfo {
  runId: string
  name: string
  kind: string         // original kind (agent, task, workflow, simulator, ...)
  mode: string         // resolved SPAWN mode (worker / background / fork / ...) — from the event
  commsMode?: string   // parent's delegation INTENT (delegate `mode` arg, e.g. agent_as_human) — the comms relationship; rendered on the LINK
  parentRunId: string  // empty for root children
  state: ChildState
  startedAt: number
  endedAt?: number
  input?: Record<string, unknown>  // cold-hydrated from the session journal
  output?: unknown
  error?: string
}

export interface TeamMessage {
  from: string
  to: string         // empty if group broadcast
  team: string       // empty if direct message
  message: string
  summary: string
  ts: number
}

export interface TeamEventEntry {
  action: string     // created | member-joined | member-left | member-state-changed | message | item-shared | state-changed | completed | failed
  teamId: string
  teamName: string
  state: string      // team FSM state (forming/active/done/failed) or member state on member-* actions
  memberId: string
  runId: string      // acting run (correlation)
  detail?: Record<string, unknown>
  ts: number
}

export interface CallEntry {
  node: string
  uri: string
  runId: string      // owning workflow run (correlation)
  durationMs: number
  attempt: number
  outcome: string    // success | failure | timeout
  error?: string
  ts: number
}

export interface ParentChildEdge {
  parentRunId: string
  childRunId: string
}

/** One shared-state ledger event (Magentic pattern): a snapshot of the full
 *  state at a version, or the terminal completion. */
export interface LedgerSnapshot {
  state: Record<string, unknown>
  version: number
  ts: number
  complete?: boolean
  reason?: string
}

/** All state for one root run — the unit of display. */
export interface RunScope {
  rootRunId: string
  sessionId: string                          // session that produced it (when known)
  workflow?: WorkflowInfo
  children: Map<string, ChildInfo>           // keyed by runId
  childrenByName: Map<string, string>        // name → most-recent child runId
  edges: ParentChildEdge[]
  teamEvents: TeamEventEntry[]
  calls: CallEntry[]
  mailboxes: Map<string, TeamMessage[]>      // keyed by recipient name
  groupMessages: Map<string, TeamMessage[]>  // keyed by team name
  ledger: LedgerSnapshot[]                   // shared-state ledger timeline (Magentic)
  cold: boolean                              // true when hydrated from the journal (not live)
}

const newScope = (rootRunId: string, sessionId: string): RunScope => ({
  rootRunId,
  sessionId,
  children: new Map(),
  childrenByName: new Map(),
  edges: [],
  teamEvents: [],
  calls: [],
  mailboxes: new Map(),
  groupMessages: new Map(),
  ledger: [],
  cold: false,
})

interface TeamStore {
  runs: Map<string, RunScope>     // keyed by rootRunId
  runIndex: Map<string, string>   // any runId → rootRunId
  activeRootRunId: string | null  // latest started root run (live routing fallback)
  viewRootRunId: string | null    // user-selected run to display (null = follow active)
  selectedTeammate: string | null
  selectedTaskRunId: string | null
  selectedEdgeKey: string | null  // `${parentRunId}->${childRunId}` of the clicked link, or null
  currentSessionId: string        // bound by the session.created event; stamped onto new scopes

  // Lifecycle handlers — called by useAgentEvents on incoming team.* events.
  onWorkflowStarted: (e: { runId: string; parentRunId?: string; name: string; ts: number }) => void
  onWorkflowDone: (e: { runId: string; output?: unknown; ts: number }) => void
  onWorkflowFailed: (e: { runId: string; error?: string; ts: number }) => void
  onChildStarted: (e: { runId: string; parentRunId: string; name: string; kind: string; mode: string; ts: number }) => void
  onChildDone: (e: { runId: string; name: string; output?: unknown; ts: number }) => void
  onChildFailed: (e: { runId: string; name: string; error?: string; ts: number }) => void
  onMessage: (e: TeamMessage) => void
  onTeamEvent: (e: TeamEventEntry) => void
  onCallCompleted: (e: CallEntry) => void
  onLedger: (e: { runId?: string; state: Record<string, unknown>; version: number; complete?: boolean; reason?: string; ts: number }) => void

  // Session binding + cold reconstruction.
  bindSession: (sessionId: string) => void
  hydrateFromTaskTree: (sessionId: string, tree: ColdTaskTree) => void

  // UI helpers.
  selectTeammate: (name: string | null) => void
  selectTask: (runId: string | null) => void
  selectEdge: (key: string | null) => void
  selectRun: (rootRunId: string | null) => void
  reset: () => void
}

/** Shape of the GetSessionTaskTree Wails payload (cold reconstruction). */
export interface ColdTaskTree {
  sessionId: string
  model?: string
  totalTokens?: number
  durationMs?: number
  tasks?: Array<{
    runId: string
    parentRunId?: string
    name: string
    kind?: string
    mode?: string
    commsMode?: string
    status?: string
    input?: Record<string, unknown>
    output?: Record<string, unknown>
    ts?: string
  }>
  teamEvents?: Array<{
    action: string
    teamId?: string
    teamName?: string
    state?: string
    memberId?: string
    runId?: string
    detail?: Record<string, unknown>
    ts?: string
  }>
  messages?: Array<{
    from: string
    to: string
    team?: string
    text: string
    summary?: string
    ts?: string
  }>
  calls?: Array<{
    node: string
    uri: string
    durationMs?: number
    attempt?: number
    outcome?: string
    error?: string
    runId?: string
    ts?: string
  }>
  ledger?: Array<{
    kind: string
    state?: Record<string, unknown>
    version?: number
    reason?: string
    runId?: string
    ts?: string
  }>
}

// ── Routing helpers (pure) ──────────────────────────────────────────────────

/** Resolve the root run owning `runId` (falling back through parent, then the
 *  active run, then creating a fresh scope rooted at `runId` itself). */
function resolveRoot(
  s: Pick<TeamStore, 'runs' | 'runIndex' | 'activeRootRunId'>,
  runId: string,
  parentRunId?: string,
): string | null {
  if (s.runIndex.has(runId)) return s.runIndex.get(runId)!
  if (parentRunId && s.runIndex.has(parentRunId)) return s.runIndex.get(parentRunId)!
  return s.activeRootRunId
}

/** Clone-on-write a scope out of the runs map. */
function scopeFor(runs: Map<string, RunScope>, rootId: string): { runs: Map<string, RunScope>; scope: RunScope } {
  const next = new Map(runs)
  const prev = next.get(rootId)
  const scope: RunScope = prev
    ? { ...prev, children: new Map(prev.children), childrenByName: new Map(prev.childrenByName), edges: [...prev.edges], teamEvents: [...prev.teamEvents], calls: [...prev.calls], mailboxes: new Map(prev.mailboxes), groupMessages: new Map(prev.groupMessages), ledger: [...prev.ledger] }
    : newScope(rootId, '')
  next.set(rootId, scope)
  return { runs: next, scope }
}

export const useTeamStore = create<TeamStore>((set) => ({
  runs: new Map(),
  runIndex: new Map(),
  activeRootRunId: null,
  viewRootRunId: null,
  selectedTeammate: null,
  selectedTaskRunId: null,
  selectedEdgeKey: null,
  currentSessionId: '',

  onWorkflowStarted: (e) =>
    set((s) => {
      // A NESTED workflow (a `type: workflow` step inside another workflow, e.g.
      // team-a/team-b under a coordinator) carries a parentRunId — keep it in the
      // parent's tree instead of forking a second root scope (which shattered the
      // Team graph into one fragment per workflow level and polluted the run
      // selector with seq_N entries). It's also registered as a child via
      // team.child_started. Routing it to the parent here is order-independent.
      if (e.parentRunId || s.runIndex.has(e.runId)) {
        // Make sure it's indexed to a root so later child events route correctly.
        const rootId = resolveRoot(s, e.runId, e.parentRunId)
        if (rootId && !s.runIndex.has(e.runId)) {
          const runIndex = new Map(s.runIndex)
          runIndex.set(e.runId, rootId)
          return { runIndex }
        }
        return {}
      }
      // A new TOP-LEVEL run: fresh scope, becomes active. Previous runs stay in
      // the map (selectable) but never merge into this one.
      const runs = new Map(s.runs)
      const scope = newScope(e.runId, s.currentSessionId)
      scope.workflow = { runId: e.runId, name: e.name, state: 'running', startedAt: e.ts }
      runs.set(e.runId, scope)
      const runIndex = new Map(s.runIndex)
      runIndex.set(e.runId, e.runId)
      return { runs, runIndex, activeRootRunId: e.runId, selectedTaskRunId: null }
    }),

  onWorkflowDone: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId)
      if (!rootId) return {}
      // Only the TRUE root's lifecycle drives the workflow banner; a nested
      // workflow's completion is reflected by its child node (child.done).
      if (rootId !== e.runId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      const wf = scope.workflow
      scope.workflow = {
        runId: e.runId,
        name: wf?.name ?? '',
        startedAt: wf?.startedAt ?? e.ts,
        state: 'done',
        endedAt: e.ts,
        output: e.output,
      }
      return { runs }
    }),

  onWorkflowFailed: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId)
      if (!rootId) return {}
      if (rootId !== e.runId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      const wf = scope.workflow
      scope.workflow = {
        runId: e.runId,
        name: wf?.name ?? '',
        startedAt: wf?.startedAt ?? e.ts,
        state: 'failed',
        endedAt: e.ts,
        error: e.error,
      }
      return { runs }
    }),

  onChildStarted: (e) =>
    set((s) => {
      // Route to the run owning the parent; a parentless child with no
      // active run roots its own scope (top-level agent run).
      let rootId = resolveRoot(s, e.runId, e.parentRunId)
      let activeRootRunId = s.activeRootRunId
      if (!rootId) {
        rootId = e.runId
        activeRootRunId = e.runId
      }
      const { runs, scope } = scopeFor(s.runs, rootId)
      if (!runs.has(rootId)) runs.set(rootId, scope)
      scope.children.set(e.runId, {
        runId: e.runId,
        name: e.name,
        kind: e.kind,
        mode: e.mode,
        parentRunId: e.parentRunId,
        state: 'running',
        startedAt: e.ts,
      })
      scope.childrenByName.set(e.name, e.runId)
      if (e.parentRunId) {
        scope.edges.push({ parentRunId: e.parentRunId, childRunId: e.runId })
      }
      // Ensure mailbox exists for this child (so the mailbox panel can render
      // an empty state when clicked before any message arrives).
      if (!scope.mailboxes.has(e.name)) scope.mailboxes.set(e.name, [])
      const runIndex = new Map(s.runIndex)
      runIndex.set(e.runId, rootId)
      return { runs, runIndex, activeRootRunId }
    }),

  onChildDone: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId)
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      const existing = scope.children.get(e.runId)
      if (!existing) return {}
      scope.children.set(e.runId, { ...existing, state: 'done', endedAt: e.ts, output: e.output })
      return { runs }
    }),

  onChildFailed: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId)
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      const existing = scope.children.get(e.runId)
      if (!existing) return {}
      scope.children.set(e.runId, { ...existing, state: 'failed', endedAt: e.ts, error: e.error })
      return { runs }
    }),

  onMessage: (e) =>
    set((s) => {
      const rootId = s.activeRootRunId
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      if (e.to) {
        const inbox = scope.mailboxes.get(e.to) ?? []
        scope.mailboxes.set(e.to, [...inbox, e])
      } else if (e.team) {
        const inbox = scope.groupMessages.get(e.team) ?? []
        scope.groupMessages.set(e.team, [...inbox, e])
      } else {
        return {}
      }
      return { runs }
    }),

  onTeamEvent: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId) ?? s.activeRootRunId
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      scope.teamEvents.push(e)
      return { runs }
    }),

  onCallCompleted: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId) ?? s.activeRootRunId
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      scope.calls.push(e)
      return { runs }
    }),

  onLedger: (e) =>
    set((s) => {
      const rootId = resolveRoot(s, e.runId ?? '') ?? s.activeRootRunId
      if (!rootId) return {}
      const { runs, scope } = scopeFor(s.runs, rootId)
      scope.ledger.push({
        state: e.state ?? {},
        version: e.version ?? 0,
        ts: e.ts,
        complete: e.complete,
        reason: e.reason,
      })
      return { runs }
    }),

  bindSession: (sessionId) =>
    set(() => ({ currentSessionId: sessionId })),

  hydrateFromTaskTree: (sessionId, tree) =>
    set((s) => {
      // Cold reconstruction: build one scope per root task found in the
      // journal projection. Skip ONLY when a still-RUNNING live scope owns this
      // session — never clobber an active run with journal data. A finished or
      // stale live scope (workflow done/failed, or none) must YIELD to cold
      // reconstruction; otherwise opening a done session's Team tab shows the
      // empty live remnant until a manual refresh.
      for (const scope of s.runs.values()) {
        if (scope.sessionId === sessionId && !scope.cold && scope.workflow?.state === 'running') return {}
      }
      const runs = new Map(s.runs)
      const runIndex = new Map(s.runIndex)
      // Evict stale COLD scopes from OTHER sessions. The store is a global
      // singleton that is never reset on session switch, so without this every
      // cold session ever opened accumulates as a `session:<id>` root and the
      // run selector fills with indistinguishable "session (journal)" entries.
      // Live scopes are preserved (a concurrent live run stays selectable).
      for (const [rootId, scope] of [...runs.entries()]) {
        if (scope.cold && scope.sessionId !== sessionId) {
          runs.delete(rootId)
          for (const [rid, root] of [...runIndex.entries()]) {
            if (root === rootId) runIndex.delete(rid)
          }
        }
      }
      const tasks = tree.tasks ?? []
      const known = new Set(tasks.map((t) => t.runId).filter(Boolean))
      const rootId = `session:${sessionId}`
      const scope = newScope(rootId, sessionId)
      scope.cold = true
      // Synthetic workflow so the run selector labels the cold root by its task
      // name + start time (not the literal "session"), and sorts correctly.
      // The status bar is hidden for cold scopes, so this never renders as a
      // live "Workflow …" line.
      const rootTask = tasks.find((t) => !(t.parentRunId && known.has(t.parentRunId)))
      scope.workflow = {
        runId: rootId,
        name: rootTask?.name || sessionId,
        state: rootTask?.status === 'failed' ? 'failed' : 'done',
        startedAt: rootTask?.ts ? Date.parse(rootTask.ts) || 0 : 0,
      }
      for (const t of tasks) {
        const runId = t.runId || `pending:${t.name}`
        const status = t.status === 'completed' ? 'done' : t.status === 'failed' ? 'failed' : 'running'
        scope.children.set(runId, {
          runId,
          name: t.name,
          kind: t.kind ?? '',
          mode: t.mode ?? '',
          commsMode: t.commsMode ?? '',
          parentRunId: t.parentRunId && known.has(t.parentRunId) ? t.parentRunId : '',
          state: status as ChildState,
          startedAt: t.ts ? Date.parse(t.ts) || 0 : 0,
          input: t.input,
          output: t.output,
        })
        scope.childrenByName.set(t.name, runId)
        if (t.parentRunId && known.has(t.parentRunId)) {
          scope.edges.push({ parentRunId: t.parentRunId, childRunId: runId })
        }
        runIndex.set(runId, rootId)
      }
      for (const te of tree.teamEvents ?? []) {
        scope.teamEvents.push({
          action: te.action,
          teamId: te.teamId ?? '',
          teamName: te.teamName ?? '',
          state: te.state ?? '',
          memberId: te.memberId ?? '',
          runId: te.runId ?? '',
          detail: te.detail,
          ts: te.ts ? Date.parse(te.ts) || 0 : 0,
        })
      }
      // Comms: rebuild the mailbox / group-message maps the live onMessage
      // path fills, so the cold Team view shows the 💬 comm edges + mailboxes.
      for (const msg of tree.messages ?? []) {
        const m: TeamMessage = {
          from: msg.from, to: msg.to, team: msg.team ?? '', message: msg.text,
          summary: msg.summary ?? '', ts: msg.ts ? Date.parse(msg.ts) || 0 : 0,
        }
        if (msg.to && msg.to !== '*') {
          scope.mailboxes.set(msg.to, [...(scope.mailboxes.get(msg.to) ?? []), m])
        } else if (msg.team) {
          scope.groupMessages.set(msg.team, [...(scope.groupMessages.get(msg.team) ?? []), m])
        }
      }
      // Calls: rebuild the call observability list (the violet calls badge).
      for (const c of tree.calls ?? []) {
        scope.calls.push({
          node: c.node, uri: c.uri, runId: c.runId ?? '', durationMs: c.durationMs ?? 0,
          attempt: c.attempt ?? 0, outcome: c.outcome ?? '', error: c.error,
          ts: c.ts ? Date.parse(c.ts) || 0 : 0,
        })
      }
      // Ledger: rebuild the shared-state timeline for the Ledger lens.
      for (const l of tree.ledger ?? []) {
        scope.ledger.push({
          state: l.state ?? {}, version: l.version ?? 0,
          ts: l.ts ? Date.parse(l.ts) || 0 : 0,
          complete: l.kind === 'complete', reason: l.reason,
        })
      }
      runs.set(rootId, scope)
      runIndex.set(rootId, rootId)
      return {
        runs,
        runIndex,
        activeRootRunId: s.activeRootRunId ?? rootId,
        viewRootRunId: s.viewRootRunId ?? rootId,
      }
    }),

  selectTeammate: (name) => set((s) => ({ selectedTeammate: name, selectedTaskRunId: name ? null : s.selectedTaskRunId, selectedEdgeKey: name ? null : s.selectedEdgeKey })),
  selectTask: (runId) => set((s) => ({ selectedTaskRunId: runId, selectedTeammate: runId ? null : s.selectedTeammate, selectedEdgeKey: runId ? null : s.selectedEdgeKey })),
  selectEdge: (key) => set((s) => ({ selectedEdgeKey: key, selectedTaskRunId: key ? null : s.selectedTaskRunId, selectedTeammate: key ? null : s.selectedTeammate })),
  selectRun: (rootRunId) => set({ viewRootRunId: rootRunId, selectedTaskRunId: null, selectedTeammate: null, selectedEdgeKey: null }),

  reset: () =>
    set({
      runs: new Map(),
      runIndex: new Map(),
      activeRootRunId: null,
      viewRootRunId: null,
      selectedTeammate: null,
      selectedTaskRunId: null,
      selectedEdgeKey: null,
    }),
}))

// ── Selectors ──────────────────────────────────────────────────────────────

/** The scope the panel displays: the user-selected run, else the active one. */
export const selectViewScope = (s: TeamStore): RunScope | undefined => {
  const id = s.viewRootRunId ?? s.activeRootRunId
  return id ? s.runs.get(id) : undefined
}

/** All known runs, latest first — for the run selector. */
export const selectRunList = (s: TeamStore): RunScope[] => {
  const list = [...s.runs.values()]
  list.sort((a, b) => (b.workflow?.startedAt ?? 0) - (a.workflow?.startedAt ?? 0))
  return list
}

/** The displayed scope's workflow (status bar). */
export const selectViewWorkflow = (s: TeamStore): WorkflowInfo | undefined =>
  selectViewScope(s)?.workflow

/** Number of children substituted in dry-run mode (status bar badge). */
export const selectSubstitutedCount = (s: TeamStore): number => {
  const scope = selectViewScope(s)
  if (!scope) return 0
  let n = 0
  for (const c of scope.children.values()) {
    const out = c.output as { dry_run?: boolean } | undefined
    if (out?.dry_run) n++
  }
  return n
}

/** Member states derived from the team narrative (latest wins). */
export interface MemberView {
  memberId: string
  state: string       // working | done | left
  joinedAt: number
}

export interface TeamView {
  teamId: string
  teamName: string
  state: string       // forming | active | done | failed
  members: MemberView[]
}

export const selectTeamView = (s: TeamStore): TeamView | undefined => {
  const scope = selectViewScope(s)
  if (!scope || scope.teamEvents.length === 0) return undefined
  const view: TeamView = { teamId: '', teamName: '', state: '', members: [] }
  const members = new Map<string, MemberView>()
  for (const e of scope.teamEvents) {
    if (e.teamId) view.teamId = e.teamId
    if (e.teamName) view.teamName = e.teamName
    switch (e.action) {
      case 'created':
        view.state = e.state || 'forming'
        break
      case 'state-changed':
      case 'completed':
      case 'failed':
        if (e.state) view.state = e.state
        break
      case 'member-joined':
        members.set(e.memberId, { memberId: e.memberId, state: 'working', joinedAt: e.ts })
        break
      case 'member-left':
        if (members.has(e.memberId)) members.get(e.memberId)!.state = 'left'
        break
      case 'member-state-changed':
        if (members.has(e.memberId)) members.get(e.memberId)!.state = e.state || 'working'
        else members.set(e.memberId, { memberId: e.memberId, state: e.state || 'working', joinedAt: e.ts })
        break
    }
  }
  view.members = [...members.values()]
  return view
}
