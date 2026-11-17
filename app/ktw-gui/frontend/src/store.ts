// SPDX-License-Identifier: Apache-2.0
import { create } from 'zustand'
import type { MsgItem, ToolMsg, SessionStatus, AskUserPayload, AskFormMsg } from './types/messages'
import type { AppSettings, SettingsSection } from './types/settings'
import { DEFAULT_SETTINGS } from './types/settings'
import type { Project, SidebarTab } from './types/project'
import { MOCK_PROJECTS } from './types/project'
import { stampReceived } from './sections/agent/messageTime'
import type { Universe, MainPaneRoute } from './types/universe'

// callApp invokes a Wails App binding by name via the runtime object. Used
// for bindings whose signature changed (StopTask now takes a sessionId) or
// are new (SetFocusedSession) — the runtime-access pattern compiles before
// the wailsjs/go/main/App.d.ts regen on the next build (same approach as
// LiveSessionList / IconRibbon).
function callApp(method: string, ...args: unknown[]): void {
  const fn = (window as unknown as { go?: { main?: { App?: Record<string, (...a: unknown[]) => unknown> } } })
    .go?.main?.App?.[method]
  if (typeof fn === 'function') { void fn(...args) }
}

// Cumulative session telemetry shown in the status bar.
export interface SessionStats {
  model?: string
  models?: string[]
  provider?: string
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  costUSD: number
  costKnown: boolean
  currentInputTokens: number
  contextWindow?: number
  turns: number
  lastDurationMs?: number
  lastTtftMs?: number
}

// Document state (LLM-generated documents opened as tabs).
export interface DocumentState {
  id: string
  title: string
  /** Picks the display engine (render media types). */
  mediaType: string
  content: string
  /** Set when the document is a streamed artifact: its content lives in the
   *  artifacts store under this id. */
  artifactId?: string
  status: 'streaming' | 'done'
  // The session (run) that produced this document. Parallel chats each show
  // only THEIR documents — without this, every chat tab shows every doc.
  sessionId?: string
}

/** Per-session conversation state. The `sessions` map is the source of
 *  truth for parallel chats; the top-level `messages`/`activePrompt`/`stats`/
 *  `sessionStatus` fields MIRROR whichever session is currently focused
 *  (`runningSessionId`) so existing chat components keep reading the global
 *  fields unchanged. Events route to a session bucket by their `session`
 *  id; only the focused session's writes also update the mirror. */
export interface SessionState {
  messages: MsgItem[]
  activePrompt: AskUserPayload | null
  stats: SessionStats | null
  status: SessionStatus
  taskId: string | null   // the run/task id
  name: string            // task name (for the tab label / focus restore)
  // Set when the task completed with a summary-only result (a `complete`
  // call whose only field is `summary`). The summary is shown as the final
  // chat message instead of opening a raw-JSON Output tab.
  summary: string | null
}

interface AppStore {
  // ── messages ─────────────────────────────────────────────────────────
  messages: MsgItem[]
  /** Per-session buckets (parallel chats). Keyed by journal session id. */
  sessions: Record<string, SessionState>
  /** Create an empty bucket for a session if absent (idempotent). */
  ensureSession: (sessionId: string, name?: string) => void
  /** Make `sessionId` the focused session: point runningSessionId/Task at
   *  it and mirror its bucket into the top-level fields. */
  focusSession: (sessionId: string) => void
  /** Drop a finished session's bucket (called on terminal cleanup). */
  removeSession: (sessionId: string) => void
  // Write actions take an optional `sid` — the session the event belongs
  // to (event.session). Omitted → the focused session. Writes the bucket
  // and, when sid is the focused session, the top-level mirror.
  addMessage: (msg: MsgItem, sid?: string, sourceRun?: string) => void
  /** Record a summary-only task result for the session. */
  setSessionSummary: (summary: string, sid?: string) => void
  appendTextDelta: (id: string, delta: string, sid?: string) => void
  finalizeText: (id: string, sid?: string) => void
  /** Replace a text message's content with the authoritative final
   *  text AND mark it done. Used on `msg.new` (persisted message) to
   *  reconcile the streamed deltas with the source-of-truth text —
   *  fixes truncation when the delta stream ended early (tool flush
   *  mid-message / dropped tail). Idempotent when deltas were complete. */
  setTextFinal: (id: string, text: string, sid?: string, sourceRun?: string, at?: string) => void
  updateTool: (id: string, update: Partial<ToolMsg>, sid?: string) => void
  /** Persist an ask-form card's resolved state (submitted value / cancelled)
   *  so the form↔view transition survives a tab switch / re-mount. */
  patchAskForm: (id: string, patch: Partial<AskFormMsg>, sid?: string) => void
  clearMessages: () => void

  // ── documents (LLM-generated tabs) ───────────────────────────────────
  documents: Map<string, DocumentState>
  openDocument: (doc: DocumentState) => void
  appendDocumentDelta: (id: string, delta: string) => void
  finalizeDocument: (id: string) => void
  /** Replaces a document's content: the user saved an edit in its tab. */
  setDocumentContent: (id: string, content: string) => void
  closeDocument: (id: string) => void
  clearDocuments: () => void

  // ── prompt ────────────────────────────────────────────────────────────
  activePrompt: AskUserPayload | null
  setPrompt: (p: AskUserPayload | null, sid?: string) => void

  // ── session telemetry (status bar: model + cumulative tokens) ────────
  stats: SessionStats | null
  setStats: (s: SessionStats, sid?: string) => void

  // ── session ───────────────────────────────────────────────────────────
  sessionStatus: SessionStatus
  activeTask: string | null
  /** Non-null while a task is running. Drives MainArea → TaskMode routing. */
  runningTaskId: string | null
  /** Server-issued session id of the currently-running task (e.g.
   *  "2026-04-14T13-29-33_hello-world"). Set by the `session.created`
   *  bridge event; consumed by panels that fetch journal/output
   *  (ChatPanel's Output tab, post-run reports). Null when no task is
   *  active. */
  runningSessionId: string | null
  startSession: (task: string) => void
  setSessionStatus: (status: SessionStatus, sid?: string) => void
  setRunningSessionId: (id: string | null) => void
  /** User-initiated dismissal of a finished task. Clears runningTaskId so MainArea
   *  routes back to NodeView. Distinct from setSessionStatus: status alone no longer
   *  auto-clears the task, the user must explicitly click "Done". */
  dismissRunningTask: () => void

  // ── pending task (gallery click → RunTaskPanel form) ─────────────────
  // Set when the user clicks a task card in NodeView; cleared when the
  // user hits Run (→ startSession) or Cancel (→ closeRunTask). pendingTask
  // !== null is the gate for MainArea → run-task mode.
  pendingTask: {
    /** The run identifier passed to RunTaskInUniverse (uri || bare name). */
    name: string
    description: string
    /** Display label (TASK.md `title:`, falls back to bare name). */
    title?: string
    /** Provider that hosts the task. */
    provider?: string
    /** Bare task name (the `run <name>` argument). */
    taskName?: string
    /** Canonical task URI (provider:name). */
    uri?: string
    inputSchemaURI?: string
    iconName?: string
    /** The object an action runs on: the chat starts with it as context. */
    targetId?: string
    /** Values the form opens with (a create task's `parent`). */
    initial?: Record<string, unknown>
    /** The task's tags: `no-initial-input` starts the run at once, with no
     *  initial-message box. */
    tags?: string[]
  } | null
  openRunTask: (task: {
    name: string
    description: string
    title?: string
    provider?: string
    taskName?: string
    uri?: string
    /** Canonical schema URI for the task's input shape, when the task
     *  shipped an `input-schema:` block (per ms-schema-driven-ui US-S1
     *  D1''). When set, RunTaskPanel renders the schema-driven
     *  FormWidgetPanel instead of the legacy free-form textarea. */
    inputSchemaURI?: string
    /** Optional icon name for the FormWidgetPanel header card. */
    iconName?: string
    /** The object an action runs on (an object's interactive action). */
    targetId?: string
    /** Values the form opens with (a create task's `parent`). */
    initial?: Record<string, unknown>
    /** The task's tags (`no-initial-input`: no initial-message box). */
    tags?: string[]
  }) => void
  closeRunTask: () => void

  // ── entry form (« + Nouveau » → a create task's form in a dialog) ────
  // A create task opened from « + Nouveau » shows its form in a dialog over
  // the object (TaskEntryDialog), not in the main area: the task as
  // pendingTask holds it, with the line its header shows.
  entryForm: (NonNullable<AppStore['pendingTask']> & {
    /** The dialog header's line (« + Nouveau › Étude · sous « X » »). */
    trail: string
  }) | null
  openEntryForm: (form: NonNullable<AppStore['entryForm']>) => void
  closeEntryForm: () => void

  // ── context tree navigation (within active project) ──────────────────
  /** Selected node within the active project's context tree. null = ProjectHome. */
  activeNodeId: string | null
  /** Display name of the active node, kept in sync by NodeTab. Used to
   *  title chat tabs as "<node name> : <action>" (null = global scope). */
  activeNodeName: string | null
  selectNode: (nodeId: string) => void
  setActiveNodeName: (name: string | null) => void
  clearNode:  () => void

  // ── workspace tree refresh (US-1: post-write re-fetch trigger) ───────
  /** Monotonic counter; components include it in their fetch dependency
   *  list. Bump after any write that may have changed the tree. */
  workspaceTreeRevision: number
  bumpWorkspaceTree:    () => void

  // ── transient toasts (artifact-creation visibility, sdd-ux-p0) ───────
  /** Short-lived feedback banners (e.g. "Created dossier X") shown when an
   *  agent task mutates the workspace. Rendered by <ToastStack>; each
   *  auto-dismisses. */
  toasts: { id: string; message: string; color?: string }[]
  pushToast:    (t: { message: string; color?: string }) => void
  dismissToast: (id: string) => void

  // (US-1 Phase 1+2 modal slots were removed when the create/edit flows
  // migrated to a `node-editor` tab in the right pane — see
  // sections/workspace/NodeEditorTab.tsx + tabsSlice.ts.)

  // ── workspace delete-node confirmation modal (US-1 Phase 3) ──────────
  /** When set, DeleteNodeConfirmModal opens for this target. Deletion
   *  cascades to all descendants + their items (substrate force=true). */
  deleteNodeTarget: { id: string; name: string; type: string } | null
  openDeleteNode:  (target: { id: string; name: string; type: string }) => void
  closeDeleteNode: () => void

  // ── cwm-v3 US-2: item delete confirmation ───────────────────────────
  // (Item create/edit live in ItemEditorTab — a full tab surface, not a
  //  modal. The old AddItemModal/EditItemModal flags were removed with
  //  the modals.)
  /** When set, DeleteItemConfirmModal opens for this item. */
  deleteItemTarget: { nodeId: string; itemId: string; title: string; type: string } | null
  openDeleteItem:  (target: { nodeId: string; itemId: string; title: string; type: string }) => void
  closeDeleteItem: () => void

  // ── projects ────────────────────────────────────────────────────────
  projects: Project[]
  activeProjectId: string | null
  sidebarTab: SidebarTab
  selectProject: (id: string) => void
  setSidebarTab: (tab: SidebarTab) => void

  // ── layout ────────────────────────────────────────────────────────────
  projectPaneOpen: boolean
  settingsOpen: boolean
  toggleProjectPane: () => void
  toggleSettings: () => void

  // ── app settings modal ────────────────────────────────────────────────
  appSettings: AppSettings
  settingsModalOpen: boolean
  settingsModalSection: SettingsSection
  openSettingsModal: (section?: SettingsSection) => void
  closeSettingsModal: () => void
  updateSettings: <K extends keyof AppSettings>(section: K, patch: Partial<AppSettings[K]>) => void

  // ── universes (Phase 2) ─────────────────────────────────────────────
  // Universes are the user's first-class context. The IconRibbon renders
  // one icon per universe; double-click activates+displays. Single click is
  // a no-op (UX safety against accidental switches).
  universes:                Universe[]
  universesLoaded:          boolean
  universesError:           string | null
  displayedUniverseName:    string | null    // null = welcome state
  mainPaneRoute:            MainPaneRoute    // welcome | universe | edit/<name> | delete/<name>
  creationModalOpen:        boolean
  /** Monotonic counter bumped on every successful `activateUniverse`,
   *  including re-activating the same universe. Universe-scoped panels
   *  (workspace tree, task gallery, home quick-actions) include this in
   *  their useEffect deps so they force a re-fetch on activation —
   *  closes the hub-catalog hydration race ("display nothing until I
   *  switch tabs and come back"). */
  universeRevision:         number

  setUniverses:             (us: Universe[]) => void
  setUniversesError:        (err: string | null) => void
  displayUniverse:          (name: string) => void
  setMainPaneRoute:         (route: MainPaneRoute) => void
  openCreationModal:        () => void
  closeCreationModal:       () => void
  /** Spawn a ktw-agents core for the universe, then display it.
   *  Idempotent if already active; refreshes busRunning and hubKeepsRunning on success. */
  activateUniverse:         (name: string) => Promise<void>
  /** Stop the universe's core. If currently displayed, falls back to
   *  welcome state. Refreshes busRunning. */
  deactivateUniverse:       (name: string) => Promise<void>
  /** « Redémarrer l'univers »: its hub and every member ended -
   *  even a hub this GUI found running -, then the universe activated again;
   *  answers whether it is running again. */
  restartUniverse:          (name: string) => Promise<boolean>
  /** Pull the active-universes list from Go and patch each universe's
   *  busRunning flag in-place. Called after activate/deactivate so the
   *  IconRibbon dot updates without a full UniverseList refetch. */
  refreshActiveUniverses:   () => Promise<void>

}

function emptySession(name?: string): SessionState {
  return { messages: [], activePrompt: null, stats: null, status: 'running', taskId: null, name: name ?? '', summary: null }
}

// applyToSession writes `patch` into the session bucket `sid` (default: the
// focused session) and, when sid IS the focused session, mirrors the bucket
// into the top-level fields the chat components read unchanged. Background
// (non-focused) sessions update their bucket only — so a parallel chat
// accumulates silently without disturbing the visible conversation.
function applyToSession(
  s: AppStore,
  sid: string | undefined,
  patch: (sess: SessionState) => SessionState,
): Partial<AppStore> {
  const target = sid || s.runningSessionId || ''
  if (!target) return {}
  const next = patch(s.sessions[target] ?? emptySession())
  const sessions = { ...s.sessions, [target]: next }
  if (target === s.runningSessionId) {
    return {
      sessions,
      messages: next.messages,
      activePrompt: next.activePrompt,
      stats: next.stats,
      sessionStatus: next.status,
    }
  }
  return { sessions }
}

export const useAppStore = create<AppStore>((set) => ({
  // ── messages ──────────────────────────────────────────────────────────
  messages: [],
  sessions: {},

  ensureSession: (sessionId, name) =>
    set((s) =>
      sessionId && !s.sessions[sessionId]
        ? { sessions: { ...s.sessions, [sessionId]: emptySession(name) } }
        : {}
    ),

  focusSession: (sessionId) => {
    // Tell the Go side which run the single-target commands (AnswerPrompt …)
    // should hit. No-op if the binding isn't present yet.
    callApp('SetFocusedSession', sessionId)
    set((s) => {
      const sess = s.sessions[sessionId] ?? emptySession()
      return {
        runningSessionId: sessionId,
        runningTaskId: sess.taskId ?? sessionId,
        activeTask: sess.name || s.activeTask,
        // mirror the bucket into the top-level (focused) view
        messages: sess.messages,
        activePrompt: sess.activePrompt,
        stats: sess.stats,
        sessionStatus: sess.status,
      }
    })
  },

  removeSession: (sessionId) =>
    set((s) => {
      if (!s.sessions[sessionId]) return {}
      const next = { ...s.sessions }
      delete next[sessionId]
      return { sessions: next }
    }),

  addMessage: (msg, sid, sourceRun) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: [...sess.messages, stampReceived(sourceRun ? { ...msg, sourceRun } : msg)],
    }))),

  setSessionSummary: (summary, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({ ...sess, summary }))),

  appendTextDelta: (id, delta, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: sess.messages.map((m) =>
        m.kind === 'text' && m.id === id ? { ...m, text: m.text + delta } : m),
    }))),

  finalizeText: (id, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: sess.messages.map((m) =>
        m.kind === 'text' && m.id === id ? { ...m, done: true } : m),
    }))),

  setTextFinal: (id, text, sid, sourceRun, at) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: sess.messages.map((m) =>
        m.kind === 'text' && m.id === id
          ? { ...m, text, done: true, ...(sourceRun ? { sourceRun } : {}), ...(at ? { at } : {}) }
          : m),
    }))),

  updateTool: (id, update, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: sess.messages.map((m) =>
        m.kind === 'tool' && m.id === id ? { ...m, ...update } : m),
    }))),

  patchAskForm: (id, patch, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      messages: sess.messages.map((m) =>
        m.kind === 'ask-form' && m.id === id ? { ...m, ...patch } : m),
    }))),

  clearMessages: () => set((s) => applyToSession(s, undefined, (sess) => ({ ...sess, messages: [] }))),

  // ── documents ────────────────────────────────────────────────────────
  documents: new Map(),

  openDocument: (doc) =>
    set((s) => {
      const next = new Map(s.documents)
      next.set(doc.id, doc)
      return { documents: next }
    }),

  appendDocumentDelta: (id, delta) =>
    set((s) => {
      const next = new Map(s.documents)
      const existing = next.get(id)
      if (existing) {
        next.set(id, { ...existing, content: existing.content + delta })
      }
      return { documents: next }
    }),

  finalizeDocument: (id) =>
    set((s) => {
      const next = new Map(s.documents)
      const existing = next.get(id)
      if (existing) {
        next.set(id, { ...existing, status: 'done' })
      }
      return { documents: next }
    }),

  setDocumentContent: (id, content) =>
    set((s) => {
      const existing = s.documents.get(id)
      if (!existing) return {}
      const next = new Map(s.documents)
      next.set(id, { ...existing, content })
      return { documents: next }
    }),

  closeDocument: (id) =>
    set((s) => {
      const next = new Map(s.documents)
      next.delete(id)
      return { documents: next }
    }),

  clearDocuments: () => set({ documents: new Map() }),

  // ── prompt ────────────────────────────────────────────────────────────
  activePrompt: null,
  setPrompt: (p, sid) => set((s) => applyToSession(s, sid, (sess) => ({ ...sess, activePrompt: p }))),

  // ── session telemetry ─────────────────────────────────────────────────
  stats: null,
  setStats: (st, sid) => set((s) => applyToSession(s, sid, (sess) => ({ ...sess, stats: st }))),

  // ── session ───────────────────────────────────────────────────────────
  sessionStatus: 'idle',
  activeTask: null,
  runningTaskId: null,
  runningSessionId: null,

  setRunningSessionId: (id) => set({ runningSessionId: id }),

  startSession: (task) =>
    set({
      sessionStatus: 'running',
      activeTask: task,
      runningTaskId: task,
      runningSessionId: null,
      messages: [],
      activePrompt: null,
      pendingTask: null,
    }),

  setSessionStatus: (status, sid) =>
    set((s) => applyToSession(s, sid, (sess) => ({
      ...sess,
      status,
      activePrompt: status !== 'running' ? null : sess.activePrompt,
      // Do NOT auto-clear runningTaskId on terminal status — the user must
      // explicitly dismiss via the "Done" button (see dismissRunningTask).
    }))),

  /** User-initiated dismissal of a finished task (replaces the auto-clear in setSessionStatus).
   *  Calls StopTask() so the Go side cancels its run context, closes the transport,
   *  exits bridgeEvents, and resets `a.running=false` — without this, a second task
   *  silently no-ops on the `if a.running { return }` guard (see task-rerun-hang). */
  dismissRunningTask: () => {
    const sid = useAppStore.getState().runningSessionId ?? ''
    // Stop exactly this session's run (parallel chats); empty → focused.
    callApp('StopTask', sid)
    set((s) => {
      const sessions = { ...s.sessions }
      if (sid) delete sessions[sid]
      return {
        sessions,
        runningTaskId: null, runningSessionId: null,
        sessionStatus: 'idle', activePrompt: null, messages: [],
      }
    })
  },

  // ── pending task (gallery click → RunTaskPanel form) ─────────────────
  pendingTask: null,
  openRunTask: (task) => set({ pendingTask: task }),
  closeRunTask: () => set({ pendingTask: null }),
  entryForm: null,
  openEntryForm: (form) => set({ entryForm: form }),
  closeEntryForm: () => set({ entryForm: null }),

  // ── context tree navigation ──────────────────────────────────────────
  activeNodeId: null,
  activeNodeName: null,
  selectNode: (nodeId) => set({ activeNodeId: nodeId }),
  setActiveNodeName: (name) => set({ activeNodeName: name }),
  clearNode:  () => set({ activeNodeId: null, activeNodeName: null }),

  // ── workspace tree refresh (US-1 Phase 1: monotonic revision) ────────
  workspaceTreeRevision: 0,
  bumpWorkspaceTree: () => set((s) => ({ workspaceTreeRevision: s.workspaceTreeRevision + 1 })),

  toasts: [],
  pushToast: ({ message, color }) =>
    set((s) => ({
      // id derived from the current count + message keeps it unique within a
      // burst without Date.now()/random (stable across re-renders).
      toasts: [...s.toasts, { id: `toast-${s.workspaceTreeRevision}-${s.toasts.length}-${message.length}`, message, color }],
    })),
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),

  // ── workspace delete-node confirmation modal (US-1 Phase 3) ──────────
  deleteNodeTarget: null,
  openDeleteNode:  (target) => set({ deleteNodeTarget: target }),
  closeDeleteNode: () => set({ deleteNodeTarget: null }),

  deleteItemTarget: null,
  openDeleteItem:  (target) => set({ deleteItemTarget: target }),
  closeDeleteItem: () => set({ deleteItemTarget: null }),


  // ── projects ─────────────────────────────────────────────────────────
  projects: MOCK_PROJECTS,
  activeProjectId: MOCK_PROJECTS[0].id,
  sidebarTab: 'objects' as SidebarTab,
  // Switching project resets context: clear node selection, pending task,
  // and any running task. Default sidebar tab back to the workspace tab.
  selectProject: (id) =>
    set({
      activeProjectId: id,
      activeNodeId: null,
      activeNodeName: null,
      pendingTask: null,
      runningTaskId: null,
      sidebarTab: 'objects',
    }),
  setSidebarTab: (tab) => set({ sidebarTab: tab }),

  // ── layout ────────────────────────────────────────────────────────────
  projectPaneOpen: true,
  settingsOpen: false,
  toggleProjectPane: () => set((s) => ({ projectPaneOpen: !s.projectPaneOpen })),
  toggleSettings: () => set((s) => ({ settingsOpen: !s.settingsOpen })),

  // ── app settings modal ────────────────────────────────────────────────
  appSettings: DEFAULT_SETTINGS,
  settingsModalOpen: false,
  settingsModalSection: 'general',

  openSettingsModal: (section = 'general') =>
    set({ settingsModalOpen: true, settingsModalSection: section }),

  closeSettingsModal: () =>
    set({ settingsModalOpen: false }),

  updateSettings: (section, patch) =>
    set((s) => ({
      appSettings: {
        ...s.appSettings,
        [section]: { ...s.appSettings[section], ...patch },
      },
    })),

  // ── universes ────────────────────────────────────────────────────────
  universes:             [],
  universesLoaded:       false,
  universesError:        null,
  displayedUniverseName: null,
  mainPaneRoute:         { kind: 'welcome' },
  creationModalOpen:     false,
  universeRevision:      0,

  setUniverses: (us) =>
    set({ universes: us, universesLoaded: true, universesError: null }),

  setUniversesError: (err) =>
    set({ universesError: err, universesLoaded: true }),

  displayUniverse: (name) => {
    try {
      window.localStorage.setItem('ktw.lastDisplayed', name)
    } catch {
      // storage may be unavailable in obscure environments; no-op
    }
    set({
      displayedUniverseName: name,
      mainPaneRoute:         { kind: 'universe' },
    })
  },

  setMainPaneRoute: (route) => set({ mainPaneRoute: route }),

  openCreationModal:  () => set({ creationModalOpen: true }),
  closeCreationModal: () => set({ creationModalOpen: false }),

  activateUniverse: async (name) => {
    const win = window as unknown as {
      go?: { main?: { App?: {
        UniverseActivate?: (name: string) => Promise<{ running?: boolean; error?: string }>
      } } }
    }
    const fn = win.go?.main?.App?.UniverseActivate
    if (!fn) {
      // Dev/mock — skip the spawn but still display the universe.
      useAppStore.getState().displayUniverse(name)
      return
    }
    const res = await fn(name)
    if (res.error) {
      // eslint-disable-next-line no-console
      console.warn('[universes] activate', name, 'failed:', res.error)
      // Surface to the user; could be replaced with a Mantine notification.
      window.alert(`Could not activate universe "${name}":\n\n${res.error}`)
      return
    }
    // Patch local busRunning + bump universeRevision so universe-scoped
    // panels re-fetch (even on re-activation of the same universe) +
    // display the universe.
    set((s) => ({
      universes:        s.universes.map((u) => u.name === name ? { ...u, busRunning: true } : u),
      universeRevision: s.universeRevision + 1,
    }))
    useAppStore.getState().displayUniverse(name)
    // Re-read the runtime facts: the list may have been read before this
    // activation, so whether the hub keeps running is known only now.
    await useAppStore.getState().refreshActiveUniverses()
  },

  deactivateUniverse: async (name) => {
    const win = window as unknown as {
      go?: { main?: { App?: {
        UniverseDeactivate?: (name: string) => Promise<{ running?: boolean; error?: string }>
      } } }
    }
    const fn = win.go?.main?.App?.UniverseDeactivate
    if (!fn) return
    const res = await fn(name)
    if (res.error) {
      // eslint-disable-next-line no-console
      console.warn('[universes] deactivate', name, 'failed:', res.error)
      window.alert(`Could not deactivate universe "${name}":\n\n${res.error}`)
      return
    }
    set((s) => {
      const universes = s.universes.map((u) => u.name === name ? { ...u, busRunning: false } : u)
      // If the deactivated universe was displayed, fall back to welcome.
      if (s.displayedUniverseName === name) {
        return { universes, displayedUniverseName: null, mainPaneRoute: { kind: 'welcome' } }
      }
      return { universes }
    })
  },

  restartUniverse: async (name) => {
    const win = window as unknown as {
      go?: { main?: { App?: {
        UniverseRestart?: (name: string) => Promise<{ running?: boolean; error?: string }>
      } } }
    }
    const fn = win.go?.main?.App?.UniverseRestart
    if (!fn) return false
    const res = await fn(name)
    if (res.error) {
      // eslint-disable-next-line no-console
      console.warn('[universes] restart', name, 'failed:', res.error)
      window.alert(`Could not restart universe "${name}":

${res.error}`)
      await useAppStore.getState().refreshActiveUniverses()
      return false
    }
    // Universe-scoped panels re-fetch from the new hub.
    set((s) => ({ universeRevision: s.universeRevision + 1 }))
    await useAppStore.getState().refreshActiveUniverses()
    return true
  },

  refreshActiveUniverses: async () => {
    const win = window as unknown as {
      go?: { main?: { App?: {
        UniverseListActive?: () => Promise<{
          active?: { name: string; homePath: string; startedAtUnix: number; hubKeepsRunning?: boolean }[]
          error?: string
        }>
      } } }
    }
    const fn = win.go?.main?.App?.UniverseListActive
    if (!fn) return
    const res = await fn()
    if (res.error || !res.active) return
    const active = new Map(res.active.map((a) => [a.name, a]))
    set((s) => ({
      universes: s.universes.map((u) => ({
        ...u,
        busRunning: active.has(u.name),
        hubKeepsRunning: active.get(u.name)?.hubKeepsRunning === true,
      })),
    }))
  },
}))

// ── Per-session selectors (explicit session/run targeting) ──────────────
// The chat content components (MessagePanel / MessageList / ChatInputPanel /
// StatusBar) take a `sessionId` and read THAT session's bucket directly via
// these selectors — NOT the global mirror. This is what isolates two
// parallel chats: each panel renders only its own run's data. Stable empty
// fallbacks keep zustand referential equality (no render loops).
const EMPTY_MESSAGES: MsgItem[] = []

export const useSessionMessages = (sid?: string): MsgItem[] =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].messages : EMPTY_MESSAGES))

export const useSessionStatus = (sid?: string): SessionStatus =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].status : 'idle'))

export const useSessionPrompt = (sid?: string): AskUserPayload | null =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].activePrompt : null))

export const useSessionStats = (sid?: string): SessionStats | null =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].stats : null))

export const useSessionName = (sid?: string): string =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].name : ''))

export const useSessionSummary = (sid?: string): string | null =>
  useAppStore((s) => (sid && s.sessions[sid] ? s.sessions[sid].summary : null))
