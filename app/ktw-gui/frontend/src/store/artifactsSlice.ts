// SPDX-License-Identifier: Apache-2.0
/**
 * Artifacts store — the UI artifacts a run shows in the chat. Sister slice
 * to teamSlice; both consume terminal-protocol events bridged via Wails
 * IPC and exposed through `useAgentEvents`.
 *
 * Each artifact is keyed by the agent-assigned `artifactId`. The chat draws
 * a run's artifacts from its chat panel's events
 * (lib/chatArtifacts.ts): each carries the artifact whole, the agent's
 * patches applied by the chat panel bridge, so `showArtifact()` lays it
 * here as it is - streaming until it is completed. `mountArtifact()` and
 * `completeArtifact()` stay for an artifact the host makes itself (a
 * Mode-B action shortcut's answer).
 *
 * After completion, the artifact stays in the store (interactive — the
 * user can still trigger ui_event submits / clicks). It's removed only
 * on session end or explicit unmount.
 *
 * The per-kind renderers read from this store; it is the single source of
 * truth for artifact state in the host.
 */

import { create } from 'zustand'
import type { ChatArtifact } from '../lib/chatArtifacts'

export type ArtifactStatus = 'streaming' | 'complete'

export interface ArtifactState {
  /** Stable agent-assigned identity. */
  artifactId: string
  /** Composition kind — picks which composition the host's <UIComponent>
   *  dispatcher renders. Added by display-dsl-framework S6/S7 (ms06). */
  kind?: 'form' | 'view' | 'table' | 'wizard' | 'page' | string
  /** Schema declaring the value's shape (canonical AST inline, or a
   *  data-type URI the document entry point resolves). Omitted for pages. */
  schema?: unknown
  /** Which `ui-<kind>` extension of the schema to use, by name. */
  variant?: string
  /** An inline `ui-<kind>` block laid over the schema (resolved in Go). */
  ui?: Record<string, unknown>
  /** Current value — patches apply here. */
  value: unknown
  /** Root of a UIPageNode tree — set ONLY when kind='page'. Patches
   *  mutate this directly (or `value` for non-page kinds). */
  dslTree?: unknown
  /** Outer data tree for `dataField` bindings inside the dslTree. Set
   *  ONLY when kind='page' and the LLM provides it. Wired by display-
   *  package-restructure Phase 3 (2026-05-06). */
  data?: unknown
  /** CRUD mode the artifact is rendered in. */
  mode: string
  /** Optional title for the host's tab UI. */
  title?: string
  /** Optional submit handler declared at mount time. */
  submitHandler?: { handler: string; method?: string }
  /** Mode-B declarative action shortcuts (US-S3): actionId →
   *  {tool, argsTemplate}. Declared actions are handled by the host
   *  locally (template-substituted); others round-trip as ui_event. */
  actions?: Record<string, { tool: string; argsTemplate?: Record<string, unknown> }>
  /** Lifecycle status. */
  status: ArtifactStatus
  /** Total number of patch ops applied so far (for diagnostics). */
  patchCount: number
  /** Mount timestamp (epoch ms). */
  mountedAt: number
  /** Completion timestamp (epoch ms), set when status flips to 'complete'. */
  completedAt?: number
}

export interface RendererRegistration {
  rendererId: string
  source: 'system' | 'llm'
  config?: unknown
  registeredAt: number
}

interface ArtifactsState {
  /** Live artifacts keyed by artifactId. */
  artifacts: Map<string, ArtifactState>
  /** Custom renderer registrations (sandboxed for source: 'llm'). */
  renderers: Map<string, RendererRegistration>

  // ── Inbound actions (called from useAgentEvents) ───────────────────

  mountArtifact(input: {
    artifactId: string
    kind?: string
    schema?: unknown
    variant?: string
    ui?: Record<string, unknown>
    value?: unknown
    dslTree?: unknown
    data?: unknown
    mode: string
    title?: string
    submitHandler?: { handler: string; method?: string }
    actions?: Record<string, { tool: string; argsTemplate?: Record<string, unknown> }>
  }): void

  /** Lay an artifact as the chat panel shows it: mounted the first time,
   *  every field replaced after (the patches are already applied). */
  showArtifact(a: ChatArtifact): void

  /** Set the artifact's value directly. Used by host-side user-driven
   *  edits (e.g. drawio editor "Done" — replaces the streamed XML with
   *  the user's edited copy). Bypasses the patch pipeline entirely.
   *  Distinct from `showArtifact` (the agent's ingress path). */
  setArtifactValue(input: { artifactId: string; value: unknown }): void

  completeArtifact(input: { artifactId: string }): void

  registerRenderer(input: {
    rendererId: string
    source: 'system' | 'llm'
    config?: unknown
  }): void

  deregisterRenderer(input: { rendererId: string }): void

  // ── Diagnostics + manual unmount (test surfaces / dev-page) ────────

  unmountArtifact(artifactId: string): void
  reset(): void
}

export const useArtifactsStore = create<ArtifactsState>((set) => ({
  artifacts: new Map(),
  renderers: new Map(),

  mountArtifact: ({ artifactId, kind, schema, variant, ui, value, dslTree, data, mode, title, submitHandler, actions }) =>
    set((state) => {
      const artifacts = new Map(state.artifacts)
      artifacts.set(artifactId, {
        artifactId,
        kind,
        schema,
        variant,
        ui,
        value,
        dslTree,
        data,
        mode,
        title,
        submitHandler,
        actions,
        status: 'streaming',
        patchCount: 0,
        mountedAt: Date.now(),
      })
      return { artifacts }
    }),

  showArtifact: (a) =>
    set((state) => {
      const existing = state.artifacts.get(a.artifactId)
      const artifacts = new Map(state.artifacts)
      const complete = a.status === 'complete'
      artifacts.set(a.artifactId, {
        ...existing,
        artifactId: a.artifactId,
        kind: a.kind,
        schema: a.schema,
        variant: a.variant,
        ui: a.ui,
        value: a.value,
        dslTree: a.dslTree,
        data: a.data,
        mode: a.mode ?? existing?.mode ?? 'view',
        title: a.title ?? existing?.title,
        actions: a.actions,
        status: complete ? 'complete' : 'streaming',
        patchCount: existing ? existing.patchCount + 1 : 0,
        mountedAt: existing?.mountedAt ?? Date.now(),
        completedAt: complete ? existing?.completedAt ?? Date.now() : undefined,
      })
      return { artifacts }
    }),

  setArtifactValue: ({ artifactId, value }) =>
    set((state) => {
      const existing = state.artifacts.get(artifactId)
      if (!existing) {
        // eslint-disable-next-line no-console
        console.warn(`[artifacts] setArtifactValue for unknown artifactId="${artifactId}" — ignoring`)
        return state
      }
      const artifacts = new Map(state.artifacts)
      artifacts.set(artifactId, { ...existing, value })
      return { artifacts }
    }),

  completeArtifact: ({ artifactId }) =>
    set((state) => {
      const existing = state.artifacts.get(artifactId)
      if (!existing) return state
      const artifacts = new Map(state.artifacts)
      artifacts.set(artifactId, {
        ...existing,
        status: 'complete',
        completedAt: Date.now(),
      })
      return { artifacts }
    }),

  registerRenderer: ({ rendererId, source, config }) =>
    set((state) => {
      const renderers = new Map(state.renderers)
      renderers.set(rendererId, {
        rendererId,
        source,
        config,
        registeredAt: Date.now(),
      })
      return { renderers }
    }),

  deregisterRenderer: ({ rendererId }) =>
    set((state) => {
      const renderers = new Map(state.renderers)
      renderers.delete(rendererId)
      return { renderers }
    }),

  unmountArtifact: (artifactId) =>
    set((state) => {
      const artifacts = new Map(state.artifacts)
      artifacts.delete(artifactId)
      return { artifacts }
    }),

  reset: () => set({ artifacts: new Map(), renderers: new Map() }),
}))
