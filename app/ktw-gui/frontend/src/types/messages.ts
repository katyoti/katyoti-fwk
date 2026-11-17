// SPDX-License-Identifier: Apache-2.0
import type { Document } from '../generated/document'

// TypeScript types mirroring the KTS Terminal Protocol.
// These are the wire types the Go backend forwards to the frontend.
// See: internal/kts/protocol/terminal/

// ── Terminal protocol events (Go → JS via Wails) ───────────────────────

export interface MsgNewEvent {
  type: 'msg.new'
  session: string
  seqNo: number
  msg: Message
}

export interface MsgUpdateEvent {
  type: 'msg.update'
  session: string
  id: string
  content: ContentUnion
}

export interface MsgDeltaEvent {
  type: 'msg.delta'
  session: string
  id: string
  delta: string
}

export interface MsgDoneEvent {
  type: 'msg.done'
  session: string
  id: string
}

export interface StateSyncEvent {
  type: 'state.sync'
  sessions: Session[]
  history: Message[]
}

/** Cumulative session telemetry for the status bar (model + tokens). */
export interface StatsEvent {
  type: 'stats'
  session: string
  model?: string
  /** Distinct models used this session (≥2 ⇒ delegation across models). */
  models?: string[]
  provider?: string
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  /** Cumulative spend computed on the agent side from the model's
   *  configured $/MTok rates (LLMProviders.yaml). Shown only when
   *  costKnown. */
  costUSD: number
  costKnown: boolean
  currentInputTokens: number
  contextWindow?: number
  turns: number
  lastDurationMs?: number
  lastTtftMs?: number
}

export interface SessionCreatedEvent {
  type: 'session.created'
  session: Session
}

// ── Core protocol types ──────────────────────────────────────────────────

export interface Message {
  id: string
  sessionId: string
  /** Run id of the delegate that produced this message; lets the chat be
   *  filtered to one node in the delegation tree. Empty = the root run. */
  sourceRun?: string
  seqNo: number
  role: 'user' | 'assistant' | 'system' | 'tool'
  timestamp: string
  content: ContentUnion
  metadata?: MsgMetadata
}

export interface MsgMetadata {
  display?: DisplayData
  taskId?: string
  parentTaskId?: string
}

export interface Session {
  id: string
  parentSessionId?: string
  name: string
  status: 'running' | 'done' | 'error' | 'stopped'
  taskId: string
  messageCount: number
  startedAt: string
}

export interface DisplayData {
  icon?: string
  label?: string
  color?: string
  title?: string
  body?: string
}

// ── Content types (discriminated union by "type" field) ──────────────────

export interface TextContent {
  type: 'text'
  text: string
}

export interface ToolContent {
  type: 'tool'
  toolName: string
  status: 'running' | 'success' | 'error'
  input?: any
  output?: any
  error?: string
  duration?: number
  display?: DisplayData
}

export interface SystemContent {
  type: 'system'
  message: string
  level: 'info' | 'warning' | 'error' | 'success'
  title?: string
  /** The message's key and params (usermsg.Msg), said by `errors:<key>`. */
  msg?: { key: string; params?: Record<string, unknown> } | null
}

export interface PromptContent {
  type: 'prompt'
  promptType: 'text' | 'select' | 'confirm' | 'form'
  question: string
  options?: string[]
  placeholder?: string
  formSchema?: any
  formData?: any   // initial value to pre-fill the form (promptType==='form')
  answered: boolean
  answer?: string
  // Resolved UserComms channel the ask routed to ('direct' / 'inbox' / …).
  // Inbox asks can't be answered from the chat input (nothing waits on
  // the HumanGate) — the UI shows an "answer it in the Inbox" notice.
  channel?: string
  // Mounted UI artifact this prompt is bound to (channel==='form', the
  // ask-user-data tool) — the chat sends the ui_event submit/cancel back
  // keyed by this id.
  artifactId?: string
  // Form card heading (channel==='form'); question is the explanation line.
  title?: string
}

export interface DelegationContent {
  type: 'delegation'
  taskName: string
  mode: string
  status: 'running' | 'completed' | 'error'
  childSessionId?: string
  summary?: string
}

export interface CodeContent {
  type: 'code'
  code: string
  language: string
  title?: string
  filename?: string
}

/** A document message: the display envelope (generated from render.Document)
 *  plus its stream status. */
export type DocumentContent = Document & {
  type: 'document'
  status: 'streaming' | 'done'
}

export type ContentUnion =
  | TextContent
  | ToolContent
  | SystemContent
  | PromptContent
  | DelegationContent
  | CodeContent
  | DocumentContent
  | { type: string; [key: string]: any } // fallback for unknown content types

// ── LLM streaming protocol — standalone events (push) ───────────────────
//
// A run's UI artifacts reach the chat as its chat panel's events
// (lib/chatArtifacts.ts); the renderer registrations below
// are standalone events.

export interface UIRendererRegistrationEvent {
  type: 'ui.renderer_registration'
  rendererId: string
  source: 'system' | 'llm'
  config?: unknown
  ts: string
}

export interface UIRendererDeregistrationEvent {
  type: 'ui.renderer_deregistration'
  rendererId: string
  ts: string
}

// ── LLM streaming protocol — outbound command (host → agent) ────────────

export interface UIEventCmdParams {
  artifactId: string
  eventType: 'submit' | 'change' | 'click' | 'navigate' | 'custom'
  value?: unknown
}

export interface UIEventCmd {
  type: 'ui_event'
  params: UIEventCmdParams
}

// ── Frontend message models (stored in Zustand) ─────────────────────────
// These map 1:1 from terminal content types to renderable items.

export type ToolStatus    = 'running' | 'done' | 'error'
export type SessionStatus = 'idle' | 'running' | 'done' | 'error' | 'stopped'

/** Run id of the delegate that produced a message; used by the Team-panel
 *  selection to filter the unified chat to one node's stream. Empty/undefined
 *  = the root run (always shown). */
export interface SourceTagged {
  sourceRun?: string
  /** When the message was sent (ISO time): the persisted message's
   *  timestamp, else the moment the chat received it. Its header shows it. */
  at?: string
}

export interface TextMsg extends SourceTagged {
  kind:   'text'
  id:     string
  sender: string
  text:   string
  done:   boolean
}

export interface ToolMsg extends SourceTagged {
  kind:   'tool'
  id:     string
  sender: string
  name:   string
  input:  string
  output: string
  error:  string
  status: ToolStatus
  display?: DisplayData
}

export interface SysMsg extends SourceTagged {
  kind:   'system'
  id:     string
  sender: string
  level:  'info' | 'warn' | 'error' | 'success'
  text:   string
}

export interface PromptMsg extends SourceTagged {
  kind:       'prompt'
  id:         string
  sender:     string
  promptType: 'text' | 'select' | 'confirm'
  question:   string
  options?:   string[]
  placeholder?: string
  answered:   boolean
  answer?:    string
}

/** AnswerViewMsg — the user's submitted ask-user-data form answer, rendered
 *  read-only as a schema-driven view card in the chat thread (the "User
 *  Answer box"). Carries the schema AST + submitted value so the inline
 *  view renderer can lay it out. */
export interface AnswerViewMsg extends SourceTagged {
  kind:   'answer-view'
  id:     string
  sender: string
  schema: any
  value:  Record<string, unknown>
  title?: string
}

/** AskFormMsg — an ask-user-data form presented as a card in the chat
 *  thread. Interactive while unanswered (Next / Cancel); on Next it flips
 *  in place to a read-only view of the submitted value (handled locally by
 *  the renderer — no store round-trip). Carries the schema AST + the
 *  artifactId the chat posts the ui_event back on. */
export interface AskFormMsg extends SourceTagged {
  kind:        'ask-form'
  id:          string
  sender:      string
  artifactId?: string
  schema:      any
  title?:      string
  prompt?:     string
  value?:      Record<string, unknown>   // initial value to pre-fill the form
  // Resolved state — persisted in the store (NOT local React state) so the
  // form↔view transition survives a tab switch / re-mount. `submitted` set
  // → render the read-only view; `cancelled` → render a cancelled note.
  submitted?:  Record<string, unknown>
  cancelled?:  boolean
}

export type MsgItem = TextMsg | ToolMsg | SysMsg | PromptMsg | AnswerViewMsg | AskFormMsg

// ── Prompt payload (for the active prompt state) ────────────────────────

export interface AskUserPayload {
  id: string
  type: 'text' | 'select' | 'confirm' | 'form'
  prompt: string
  title?: string   // form card heading (channel='form'); prompt is the explanation
  options?: string[]
  placeholder?: string
  formSchema?: any  // schema AST (channel='form' / ask-user-data) or formdef JSON (ask-user form)
  // Resolved UserComms channel ('direct' / 'inbox' / 'form' / …). When 'inbox',
  // the input panel renders a go-to-Inbox notice instead of the answer
  // form — typing here can't reach an inbox-suspended task. When 'form'
  // (the ask-user-data tool), the form is in its own tab and the panel
  // renders an "open the form" button (focusing artifactId) + keeps the
  // composer live.
  channel?: string
  artifactId?: string
}
