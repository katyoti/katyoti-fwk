// SPDX-License-Identifier: Apache-2.0
/**
 * action-shortcut — Mode-B dispatcher for declarative artifact action
 * shortcuts (US-S3 crud action wiring).
 *
 * When an artifact declares `actions: {<actionId>: {tool, argsTemplate}}`
 * and that action fires, the host handles it LOCALLY: the argsTemplate
 * is substituted against the action payload and the tool is executed
 * natively — no agent round-trip, no LLM latency.
 *
 * Sandbox: only tools with a native host handler run; v1 supports
 * `display-ui-component` (mount another artifact from the substituted
 * args — e.g. table row-click → detail view of the clicked row).
 * Anything else returns false and the caller falls back to Mode A
 * (ui_event → agent).
 */
import { MEDIA_TYPE_DRAWIO, MEDIA_TYPE_UI } from '../generated/media-types'
import { useArtifactsStore } from '../store/artifactsSlice'
import { useAppStore } from '../store'
import { substituteTemplate, type UIActionShortcut } from './action-template'

/**
 * Execute one declared shortcut against the action payload.
 * Returns true when handled (Mode B), false to fall back to Mode A.
 */
export function dispatchActionShortcut(
  shortcut: UIActionShortcut,
  payload: Record<string, unknown>,
): boolean {
  if (shortcut.tool !== 'display-ui-component') {
    // eslint-disable-next-line no-console
    console.warn(`[action-shortcut] unsupported tool ${JSON.stringify(shortcut.tool)} — falling back to Mode A`)
    return false
  }
  const args = substituteTemplate(shortcut.argsTemplate ?? {}, payload) as Record<string, unknown>
  const artifactId = typeof args.artifactId === 'string' ? args.artifactId : ''
  const kind = typeof args.kind === 'string' ? args.kind : ''
  if (!artifactId || !kind) {
    // eslint-disable-next-line no-console
    console.warn('[action-shortcut] display-ui-component shortcut needs artifactId + kind — falling back to Mode A', args)
    return false
  }

  // Native equivalent of the agent's display-ui-component mount:
  // store the artifact + open its tab. Host-mounted artifacts are
  // complete immediately (nothing is streaming).
  const artifacts = useArtifactsStore.getState()
  artifacts.mountArtifact({
    artifactId,
    kind,
    schema: args.schema,
    variant: typeof args.variant === 'string' ? args.variant : undefined,
    ui: args.ui && typeof args.ui === 'object' ? args.ui as Record<string, unknown> : undefined,
    value: kind === 'page' ? undefined : args.data,
    dslTree: args.dslTree,
    data: kind === 'page' ? args.data : undefined,
    mode: typeof args.mode === 'string' ? args.mode : 'view',
    title: typeof args.title === 'string' ? args.title : undefined,
    actions: args.actions as never,
  })
  artifacts.completeArtifact({ artifactId })

  const title =
    (typeof args.title === 'string' && args.title) || `${kind}: ${artifactId}`
  useAppStore.getState().openDocument({
    id: `art:${artifactId}`,
    title,
    mediaType: kind === 'drawio' ? MEDIA_TYPE_DRAWIO : MEDIA_TYPE_UI,
    content: '',
    artifactId,
    status: 'done',
    // Scope to the active session so the artifact shows in THIS chat panel,
    // not bleeding into every open session (wrong-session bug).
    sessionId: useAppStore.getState().runningSessionId ?? undefined,
  })
  return true
}
