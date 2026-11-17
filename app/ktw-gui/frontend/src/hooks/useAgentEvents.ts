// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react'
import { openOf } from './toolOpen'
import { openRunResult } from './runOpen'
import { errorText } from '../i18n/errors'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store'
import { useTabsStore } from '../store/tabsSlice'
import { openObjectTab } from '../sections/workspace/openObjectTab'
import { useTeamStore } from '../store/teamSlice'
import { useArtifactsStore } from '../store/artifactsSlice'
import { chatArtifactReader, chatArtifactTitle, type ChatArtifact, type ChatPanelEvent } from '../lib/chatArtifacts'
import type {
  MsgNewEvent, MsgUpdateEvent, MsgDeltaEvent, MsgDoneEvent,
  StatsEvent,
  SessionCreatedEvent,
  TextContent, ToolContent, SystemContent, PromptContent, DocumentContent,
  CodeContent,
  UIRendererRegistrationEvent, UIRendererDeregistrationEvent,
  ContentUnion,
} from '../types/messages'

// Team-tab event payload shapes — mirror terminal protocol team.* events
// emitted by the translator (see internal/kts/protocol/terminal/events.go).
interface TeamWorkflowStartedEvent { type: string; runId: string; parentRunId?: string; name?: string; inputs?: Record<string, unknown>; ts: string }
interface TeamWorkflowDoneEvent    { type: string; runId: string; output?: unknown; ts: string }
interface TeamWorkflowFailedEvent  { type: string; runId: string; error?: string; ts: string }
interface TeamChildStartedEvent    { type: string; runId: string; parentRunId?: string; name: string; kind: string; mode?: string; ts: string }
interface TeamChildDoneEvent       { type: string; runId: string; name: string; output?: unknown; ts: string }
interface TeamChildFailedEvent     { type: string; runId: string; name: string; error?: string; ts: string }
interface TeamMessageEvent         { type: string; from: string; to?: string; team?: string; message: string; summary?: string; ts: string }
interface TeamTeamEventMsg         { type: string; action: string; teamId: string; teamName?: string; state?: string; memberId?: string; runId?: string; parentRunId?: string; detail?: Record<string, unknown>; ts: string }
interface TeamCallCompletedMsg     { type: string; node: string; uri: string; runId?: string; durationMs: number; attempt: number; outcome: string; error?: string; ts: string }
interface TeamLedgerMsg            { type: string; runId?: string; state?: Record<string, unknown>; version?: number; complete?: boolean; reason?: string; ts: string }

const tsToEpoch = (ts: string): number => {
  const t = Date.parse(ts)
  return isNaN(t) ? Date.now() : t
}

/** When a task's `complete` result is "just a summary" — a plain string,
 *  or an object whose only field is a non-empty `summary` string — return
 *  that summary text; otherwise null (a structured result the Output tab
 *  should render field-by-field). */
function summaryFromCompleteInput(input: unknown): string | null {
  if (typeof input === 'string') {
    const t = input.trim()
    return t ? t : null
  }
  if (input && typeof input === 'object') {
    const obj = input as Record<string, unknown>
    const keys = Object.keys(obj)
    if (keys.length === 1 && keys[0] === 'summary' && typeof obj.summary === 'string') {
      const t = obj.summary.trim()
      return t ? t : null
    }
  }
  return null
}

/** Title-case a kebab/snake action name for display: "compose-artifact"
 *  → "Compose Artifact", "chat" → "Chat". No hardcoded special cases. */
const titleCaseName = (name: string): string =>
  name
    .split(/[-_]/)
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join(' ')

// The agent's CWM write tools: the cwm-object-* / cwm-edge-* tools cover
// create / update / delete / move / split; matching by suffix below catches
// later additions. Reads (`*-get`, `*-list`, `*-search`) do NOT trigger a
// refresh — tree state is unchanged by read traffic.
const CWM_WRITE_TOOL_NAMES = new Set([
  'cwm-object-create',
  'cwm-object-update',
  'cwm-object-move',
  'cwm-object-split',
  'cwm-object-delete',
  'cwm-object-archive',
  'cwm-object-named-file-write',
  'cwm-edge-create',
  'cwm-edge-delete',
])

/** Parse a CWM write tool name (`cwm-<entity>-<op>`) into a user-facing
 *  verb + entity word. Returns null for names that don't fit the shape. */
function describeCWMWrite(toolName: string): { verb: string; entity: string } | null {
  if (toolName === 'cwm-object-named-file-write') return { verb: 'Wrote', entity: 'file' }
  const m = toolName.match(/^cwm-([a-z]+)-([a-z]+)$/)
  if (!m) return null
  const entity = m[1] // object | edge | source
  const op = m[2]     // create | update | move | split | delete | reanchor
  const verbs: Record<string, string> = {
    create: 'Created', update: 'Updated', move: 'Moved', split: 'Split',
    delete: 'Removed', archive: 'Archived', reanchor: 'Re-anchored',
  }
  return { verb: verbs[op] ?? op, entity }
}

/** React to a successful CWM-write agent tool: bump the tree, raise a
 *  toast so the change is VISIBLE (sdd-ux-p0 "watch the dossier fill"),
 *  and auto-reveal a freshly created object (open its pane). */
/** The last object each run created: shown again when the run ends, so a
 *  task that makes a deck (a translation, a short version) ends on it even
 *  when its chat took the focus back. */
const createdBySession = new Map<string, { id: string; title?: string }>()

function handleCWMWrite(toolName: string, outputStr: string, session?: string): void {
  const store = useAppStore.getState()
  // Bump first so the tree re-fetches even if anything below throws.
  store.bumpWorkspaceTree()

  const desc = describeCWMWrite(toolName)
  if (!desc) return

  let label = ''
  let created: { id: string; title?: string } | null = null
  try {
    const out = outputStr ? JSON.parse(outputStr) : null
    const obj = out?.object
    const title: string | undefined = obj?.name
    if (obj?.id) {
      label = title ? `“${title}”` : obj.id
      if (toolName === 'cwm-object-create') created = { id: obj.id, title }
    }
  } catch {
    // Output wasn't JSON — fall back to the generic "<verb> <entity>" message.
  }

  const color = desc.verb === 'Removed' ? 'red' : desc.verb === 'Created' ? 'teal' : 'blue'
  store.pushToast({ message: (label ? `${desc.verb} ${label}` : `${desc.verb} ${desc.entity}`).trim(), color })

  // Reveal a newly created object: open its pane.
  if (created) {
    const uni = store.displayedUniverseName
    if (uni) openObjectTab(uni, created)
    if (session) createdBySession.set(session, created)
  }
}

function isCWMWriteTool(toolName: string): boolean {
  if (CWM_WRITE_TOOL_NAMES.has(toolName)) return true
  // Future-proof: any cwm-*-create / -update / -move / -delete that
  // shows up after this list is written still triggers a refresh.
  if (!toolName.startsWith('cwm-')) return false
  return (
    toolName.endsWith('-create') ||
    toolName.endsWith('-update') ||
    toolName.endsWith('-move') ||
    toolName.endsWith('-delete')
  )
}

/** Where the chat draws a run's artifacts: the artifacts store, and the
 *  artifact's document tab - opened the first time, its content read from
 *  the store, flipped done when the artifact completes. */
export const chatArtifactSink = {
  show(a: ChatArtifact) {
    useArtifactsStore.getState().showArtifact(a)
    if (a.status !== 'complete') return
    const existing = useAppStore.getState().documents.get(`art:${a.artifactId}`)
    if (existing && existing.status !== 'done') {
      useAppStore.getState().openDocument({ ...existing, status: 'done' })
    }
  },
  open(a: ChatArtifact, session: string | undefined) {
    useAppStore.getState().openDocument({
      id: `art:${a.artifactId}`,
      title: chatArtifactTitle(a),
      mediaType: a.mediaType ?? '',
      content: '',
      artifactId: a.artifactId,
      status: a.status === 'complete' ? 'done' : 'streaming',
      sessionId: session,
    })
  },
}

/**
 * Wires KTS Terminal Protocol events to the Zustand store.
 * The Go backend forwards raw terminal events as Wails runtime events
 * using the terminal event type as the event name:
 *   "msg.new", "msg.update", "msg.delta", "msg.done",
 *   "state.sync", "session.created"
 *
 * Must be called once in the root App component.
 */
export function useAgentEvents() {
  useEffect(() => {
    // Wails runtime is injected by the WebView — not available in plain browser.
    if (!(window as any).runtime) return

    const drawArtifacts = chatArtifactReader(chatArtifactSink)

    const off = [
      // ── Streaming text deltas ──────────────────────────────────────
      EventsOn('msg.delta', (evt: MsgDeltaEvent) => {
        const store = useAppStore.getState()
        // Route to document if this ID belongs to an open document tab
        // (progressive document streaming — the translator emits msg.delta
        // for document messages that are in "streaming" state).
        if (store.documents.has(evt.id)) {
          store.appendDocumentDelta(evt.id, evt.delta)
          return
        }
        // Otherwise: treat as text streaming. Route to the event's session
        // bucket (parallel chats); existence is checked within that bucket,
        // not the global mirror.
        const exists = (store.sessions[evt.session]?.messages ?? []).some(m => m.id === evt.id)
        if (!exists) {
          store.addMessage({
            kind: 'text', id: evt.id, sender: 'assistant', text: evt.delta, done: false,
          }, evt.session)
        } else {
          store.appendTextDelta(evt.id, evt.delta, evt.session)
        }
      }),

      // ── Streaming done ─────────────────────────────────────────────
      EventsOn('msg.done', (evt: MsgDoneEvent) => {
        useAppStore.getState().finalizeText(evt.id, evt.session)
      }),

      // ── Session telemetry (status bar: model + cumulative tokens) ──
      EventsOn('stats', (evt: StatsEvent) => {
        useAppStore.getState().setStats({
          model: evt.model,
          models: evt.models,
          provider: evt.provider,
          inputTokens: evt.inputTokens,
          outputTokens: evt.outputTokens,
          cacheReadTokens: evt.cacheReadTokens,
          cacheCreationTokens: evt.cacheCreationTokens,
          costUSD: evt.costUSD,
          costKnown: evt.costKnown,
          currentInputTokens: evt.currentInputTokens,
          contextWindow: evt.contextWindow,
          turns: evt.turns,
          lastDurationMs: evt.lastDurationMs,
          lastTtftMs: evt.lastTtftMs,
        }, evt.session)
      }),

      // ── New persisted message ──────────────────────────────────────
      EventsOn('msg.new', (evt: MsgNewEvent) => {
        const { msg } = evt
        const content = msg.content as ContentUnion
        // The originating delegate's run id — lets the Team panel filter the
        // unified chat to one node's stream. Empty for the root run's own text.
        const sourceRun = msg.sourceRun
        // The time it was sent - its chat header shows it, not the render time.
        const at = msg.timestamp || undefined

        switch (content.type) {
          case 'text': {
            const tc = content as TextContent
            const store = useAppStore.getState()
            // If deltas already created this message, reconcile it with
            // the authoritative persisted text and mark done. The delta
            // stream can end early (a tool flush mid-message, or dropped
            // tail deltas) leaving the chat truncated while the journal —
            // built from this same msg.new — has the full text. Applying
            // tc.text here makes the chat match. Idempotent when the
            // deltas were already complete.
            const existing = (store.sessions[evt.session]?.messages ?? []).find(m => m.id === msg.id)
            if (existing && existing.kind === 'text') {
              store.setTextFinal(msg.id, tc.text, evt.session, sourceRun, at)
            } else {
              store.addMessage({
                kind: 'text', id: msg.id, sender: msg.role,
                text: tc.text, done: true,
                at,
              }, evt.session, sourceRun)
            }
            break
          }

          case 'tool': {
            const tc = content as ToolContent
            const store = useAppStore.getState()
            // A summary-only `complete` result (its only field is `summary`,
            // or a plain string) is rendered as the final markdown chat
            // message instead of a raw-JSON Output tab. ChatPanel reads the
            // session summary to suppress the Output tab.
            if (tc.toolName === 'complete') {
              const summary = summaryFromCompleteInput(tc.input)
              if (summary !== null) {
                store.setSessionSummary(summary, evt.session)
                store.addMessage({
                  kind: 'text', id: `${msg.id}:summary`, sender: 'assistant',
                  text: summary, done: true,
                  at,
                }, evt.session, sourceRun)
                break
              }
            }
            const inputStr = tc.input ? JSON.stringify(tc.input) : ''
            store.addMessage({
              kind: 'tool', id: msg.id, sender: 'assistant',
              name: tc.toolName, input: inputStr,
              output: '', error: '', status: 'running',
              display: tc.display,
              at,
            }, evt.session, sourceRun)
            break
          }

          case 'system': {
            const sc = content as SystemContent
            useAppStore.getState().addMessage({
              kind: 'system', id: msg.id, sender: 'system',
              level: sc.level === 'warning' ? 'warn' : sc.level as any,
              // A keyed message (a failed model call's cause) is said in the
              // person's language with its next step.
              text: errorText({ error: sc.message, errorMsg: sc.msg }),
              at,
            }, evt.session, sourceRun)
            break
          }

          case 'document': {
            const dc = content as DocumentContent
            // Open a new document tab (auto-focused in ChatPanel).
            useAppStore.getState().openDocument({
              id: msg.id,
              title: dc.title ?? '',
              mediaType: dc.mediaType,
              content: typeof dc.content === 'string' ? dc.content : '',
              status: dc.status,
              sessionId: evt.session,
            })
            break
          }

          case 'code': {
            // A code block opens as a document of its language's media type
            // (text/x-<language>): the text / code engine shows it.
            const cc = content as CodeContent
            useAppStore.getState().openDocument({
              id: msg.id,
              title: cc.title || cc.filename || cc.language || 'code',
              mediaType: cc.language ? `text/x-${cc.language.toLowerCase()}` : 'text/plain',
              content: cc.code,
              status: 'done',
              sessionId: evt.session,
            })
            break
          }

          case 'prompt': {
            const pc = content as PromptContent
            // ask-user-data form: render it as an interactive CARD in the
            // chat thread (not the bottom input area). The card flips to a
            // read-only view in place on Next.
            if (pc.channel === 'form') {
              useAppStore.getState().addMessage({
                kind: 'ask-form',
                id: msg.id,
                sender: 'assistant',
                artifactId: pc.artifactId,
                schema: pc.formSchema,
                title: pc.title,
                prompt: pc.question,
                value: pc.formData,
                at,
              }, evt.session, sourceRun)
              break
            }
            // Set the active prompt for the input UI.
            useAppStore.getState().setPrompt({
              id: msg.id,
              type: pc.promptType,
              prompt: pc.question,
              options: pc.options,
              placeholder: pc.placeholder,
              formSchema: pc.formSchema,
              channel: pc.channel,
              artifactId: pc.artifactId,
              title: pc.title,
            }, evt.session)
            break
          }

          default:
            // Unknown content type — render as system info.
            useAppStore.getState().addMessage({
              kind: 'system', id: msg.id, sender: 'system',
              level: 'info', text: JSON.stringify(content),
              at,
            }, evt.session, sourceRun)
        }
      }),

      // ── Update existing message ────────────────────────────────────
      EventsOn('msg.update', (evt: MsgUpdateEvent) => {
        const content = evt.content as ContentUnion

        switch (content.type) {
          case 'tool': {
            const tc = content as ToolContent
            const update: any = {}
            if (tc.status === 'success') {
              update.status = 'done'
              update.output = tc.output != null ? (typeof tc.output === 'string' ? tc.output : JSON.stringify(tc.output)) : ''
              // cwm-v3 US-3 Phase 5 + sdd-ux-p0: when the agent
              // successfully mutates the CWM graph, bump the workspace
              // tree so WorkspaceItemsTree re-fetches within the same
              // turn, raise a toast so the change is VISIBLE, and
              // auto-reveal a freshly created node. Without the bump the
              // user had to click refresh to see a node the agent just
              // created / renamed / deleted. We watch the canonical
              // write tools by prefix (stable names; prefix covers
              // future additions like cwm-node-move automatically).
              if (tc.toolName && isCWMWriteTool(tc.toolName)) {
                handleCWMWrite(tc.toolName, update.output, evt.session)
              } else {
                // A tool answering {ok, open} (study-create, dossier-create)
                // opens what it made, as a created object does.
                const target = openOf(update.output)
                const uni = useAppStore.getState().displayedUniverseName
                if (target && uni) {
                  openRunResult(uni, target)
                  if (evt.session) createdBySession.set(evt.session, target)
                }
              }
            } else if (tc.status === 'error') {
              update.status = 'error'
              update.error = tc.error || 'Unknown error'
            }
            if (tc.display) update.display = tc.display
            useAppStore.getState().updateTool(evt.id, update, evt.session)
            break
          }

          case 'prompt': {
            const pc = content as PromptContent
            if (pc.answered) {
              // Clear the active prompt.
              useAppStore.getState().setPrompt(null, evt.session)
            }
            break
          }

          case 'document': {
            const dc = content as DocumentContent
            const store = useAppStore.getState()
            // If content is provided, replace the full document (streaming → done).
            // Otherwise just update the status.
            const existing = store.documents.get(evt.id)
            if (existing) {
              store.openDocument({
                ...existing,
                content: typeof dc.content === 'string' ? dc.content : existing.content,
                status: dc.status,
              })
            } else {
              // No prior msg.new — treat as new document. MUST carry the
              // session so it scopes to the owning chat panel (without it the
              // doc bleeds into every session's panel — wrong-session bug).
              store.openDocument({
                id: evt.id,
                title: dc.title ?? '',
                mediaType: dc.mediaType,
                content: typeof dc.content === 'string' ? dc.content : '',
                status: dc.status,
                sessionId: evt.session,
              })
            }
            break
          }
        }
      }),

      // ── Session created ────────────────────────────────────────────
      EventsOn('session.created', (evt: SessionCreatedEvent) => {
        const sid = evt.session?.id
        const name = evt.session?.name ?? 'chat'
        const store = useAppStore.getState()
        if (sid) {
          // Create this session's bucket + FOCUS it (parallel chats: each
          // run gets its own state; focusing mirrors it into the global
          // view the chat components read). focusSession also sets
          // runningSessionId/runningTaskId.
          store.ensureSession(sid, name)
          store.focusSession(sid)
          store.setSessionStatus('running', sid)
          // Bind the team store's session so new run scopes carry the
          // session id (Team tab session scoping + cold-load join key).
          useTeamStore.getState().bindSession(sid)
        } else {
          store.setSessionStatus('running')
        }
        // Open / focus this session's OWN chat tab (`chat:<sid>`) so each
        // parallel run has its own surface. The tab id was a fixed 'chat'
        // before — that is what made two chats collide. ChatPanel reads
        // its tab's session via the focused mirror; switching tabs
        // re-focuses (see tab-click wiring).
        const uni = store.displayedUniverseName
        if (uni && sid) {
          const tabId = `chat:${sid}`
          // Title generically as "<node name> : <action>" — the action is
          // the session/task name, the node name (when scoped to one) comes
          // from the store. Global-scope chats drop the node prefix. No
          // hardcoded labels.
          const action = titleCaseName(name)
          const nodeName = store.activeNodeName
          const label = nodeName ? `${nodeName} : ${action}` : action
          useTabsStore.getState().openTab(uni, {
            id:        tabId,
            kind:      'chat',
            label,
            closeable: true,
            payload:   { taskName: name, sessionId: sid, scopedNodeName: nodeName ?? undefined },
          })
          useTabsStore.getState().setActive(uni, tabId)
        }
      }),

      // ── State sync (connect handshake) ─────────────────────────────
      EventsOn('state.sync', (evt: { panels?: ChatPanelEvent[] }) => {
        // Initial sync — session is running; a reopened session's
        // artifacts come with it, from its journaled chat panel events.
        useAppStore.getState().setSessionStatus('running')
        for (const p of evt?.panels ?? []) drawArtifacts(p)
      }),

      // ── Session ended (transport closed) ───────────────────────────
      // ── A workflow's answer names what it made ({ok, open}) ───────
      // A creation run with no model (« Nouvelle étude de l'existant »), or
      // an agent whose final result is an {ok, open} answer, opens the
      // object it made, as a tool answering {ok, open} does; the task's
      // entry form gives way to it.
      EventsOn('run.open', (evt: { session?: string; open?: string }) => {
        const target = openOf(JSON.stringify({ ok: true, open: evt.open }))
        const uni = useAppStore.getState().displayedUniverseName
        if (!target || !uni) return
        openRunResult(uni, target)
        if (evt.session) createdBySession.set(evt.session, target)
      }),

      EventsOn('session.ended', (evt: { status: string; session?: string }) => {
        useAppStore.getState().setSessionStatus((evt.status as any) || 'done', evt.session)
        const created = evt.session ? createdBySession.get(evt.session) : undefined
        if (created && evt.session) {
          createdBySession.delete(evt.session)
          const uni = useAppStore.getState().displayedUniverseName
          if (uni && (!evt.status || evt.status === 'done')) openObjectTab(uni, created)
        }
      }),

      // ── Team / multi-agent events (ms08 task-engine bridge) ────────
      // Translator publishes these from workflow.* / child.* / to.* / group.*
      // bus topics; consumed by the Team tab via useTeamStore.
      EventsOn('team.workflow_started', (evt: TeamWorkflowStartedEvent) => {
        useTeamStore.getState().onWorkflowStarted({ runId: evt.runId, parentRunId: evt.parentRunId ?? '', name: evt.name ?? '', ts: tsToEpoch(evt.ts) })
      }),
      EventsOn('team.workflow_done', (evt: TeamWorkflowDoneEvent) => {
        useTeamStore.getState().onWorkflowDone({ runId: evt.runId, output: evt.output, ts: tsToEpoch(evt.ts) })
      }),
      EventsOn('team.workflow_failed', (evt: TeamWorkflowFailedEvent) => {
        useTeamStore.getState().onWorkflowFailed({ runId: evt.runId, error: evt.error, ts: tsToEpoch(evt.ts) })
      }),
      EventsOn('team.child_started', (evt: TeamChildStartedEvent) => {
        useTeamStore.getState().onChildStarted({
          runId: evt.runId,
          parentRunId: evt.parentRunId ?? '',
          name: evt.name,
          kind: evt.kind,
          mode: evt.mode ?? '',
          ts: tsToEpoch(evt.ts),
        })
      }),
      EventsOn('team.child_done', (evt: TeamChildDoneEvent) => {
        useTeamStore.getState().onChildDone({
          runId: evt.runId,
          name: evt.name,
          output: evt.output,
          ts: tsToEpoch(evt.ts),
        })
      }),
      EventsOn('team.child_failed', (evt: TeamChildFailedEvent) => {
        useTeamStore.getState().onChildFailed({
          runId: evt.runId,
          name: evt.name,
          error: evt.error,
          ts: tsToEpoch(evt.ts),
        })
      }),
      EventsOn('team.message', (evt: TeamMessageEvent) => {
        useTeamStore.getState().onMessage({
          from: evt.from,
          to: evt.to ?? '',
          team: evt.team ?? '',
          message: evt.message,
          summary: evt.summary ?? '',
          ts: tsToEpoch(evt.ts),
        })
      }),
      // Team-lens narrative: TeamManager lifecycle (created / member-joined /
      // message / state-changed / completed) with the correlation envelope.
      EventsOn('team.team_event', (evt: TeamTeamEventMsg) => {
        useTeamStore.getState().onTeamEvent({
          action: evt.action,
          teamId: evt.teamId,
          teamName: evt.teamName ?? '',
          state: evt.state ?? '',
          memberId: evt.memberId ?? '',
          runId: evt.runId ?? '',
          detail: evt.detail,
          ts: tsToEpoch(evt.ts),
        })
      }),
      // Workflow `call` primitive observability rows (NOT team members).
      EventsOn('team.call_completed', (evt: TeamCallCompletedMsg) => {
        useTeamStore.getState().onCallCompleted({
          node: evt.node,
          uri: evt.uri,
          runId: evt.runId ?? '',
          durationMs: evt.durationMs,
          attempt: evt.attempt,
          outcome: evt.outcome,
          error: evt.error,
          ts: tsToEpoch(evt.ts),
        })
      }),
      // Shared-state ledger (Magentic pattern) → the Team tab's Ledger lens.
      EventsOn('team.ledger', (evt: TeamLedgerMsg) => {
        useTeamStore.getState().onLedger({
          runId: evt.runId,
          state: evt.state ?? {},
          version: evt.version ?? 0,
          complete: evt.complete,
          reason: evt.reason,
          ts: tsToEpoch(evt.ts),
        })
      }),

      // ── A run's UI artifacts: its chat panel's events ──────────────
      // The one feed of the chat's artifacts, the display panels' switch
      // on or off: each event carries the artifacts it
      // shows whole, the agent's patches applied by the chat panel bridge.
      EventsOn('panel.event', (evt: ChatPanelEvent) => drawArtifacts(evt)),

      // ── LLM streaming protocol — renderer registrations ───────────
      EventsOn('ui.renderer_registration', (evt: UIRendererRegistrationEvent) => {
        useArtifactsStore.getState().registerRenderer({
          rendererId: evt.rendererId,
          source: evt.source,
          config: evt.config,
        })
      }),
      EventsOn('ui.renderer_deregistration', (evt: UIRendererDeregistrationEvent) => {
        useArtifactsStore.getState().deregisterRenderer({
          rendererId: evt.rendererId,
        })
      }),
    ]

    return () => { off.forEach((fn) => fn?.()) }
  }, [])
}
