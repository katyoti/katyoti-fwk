// SPDX-License-Identifier: Apache-2.0
// A run's UI artifacts as the chat draws them: read from
// its chat panel's events (`panel.event` of the session's terminal stream,
// internal/kts/protocol/terminal/chatpanel) - the one feed of every chat,
// the display panels' switch on or off. Each artifact node carries the
// artifact whole, every patch the agent sent applied by the bridge, its
// status streaming until it is completed: the chat shows it as it is, it
// applies no patch of its own.

import type { PanelEvent } from '@ktw/display-lib/ui-builders/page/panel.ts'

/** The conversation node of the chat panel, and the prefix of an
 *  artifact's node id (chatpanel.ArtifactsNode, chatpanel.NodeID). */
export const CHAT_ARTIFACTS_NODE = 'chat:artifacts'
const ARTIFACT_NODE_PREFIX = 'artifact:'

/** One UI artifact as the chat panel shows it (chatpanel.Artifact). */
export interface ChatArtifact {
  artifactId: string
  kind?: string
  mediaType?: string
  title?: string
  schema?: unknown
  variant?: string
  ui?: Record<string, unknown>
  value?: unknown
  dslTree?: unknown
  data?: unknown
  mode?: string
  actions?: Record<string, { tool: string; argsTemplate?: Record<string, unknown> }>
  status: 'streaming' | 'complete'
}

/** The chat panel event as the session's terminal stream carries it. */
export interface ChatPanelEvent {
  type?: string
  session?: string
  /** Set on a followed display panel's event (displayPanels.ts): never the
   *  chat's own. */
  follow?: string
  event?: PanelEvent
}

function artifactOfNode(node: unknown): ChatArtifact | undefined {
  const n = node as { id?: unknown; artifact?: unknown } | null
  if (!n || typeof n.id !== 'string' || !n.id.startsWith(ARTIFACT_NODE_PREFIX)) return undefined
  const a = n.artifact as ChatArtifact | undefined
  if (!a || typeof a.artifactId !== 'string' || a.artifactId === '') return undefined
  return a
}

/** Every artifact a chat panel event shows, in its order: those of the
 *  conversation node a replace of it carries, and the one a replace of an
 *  artifact's node carries (chatpanel.ArtifactsOf). */
export function artifactsOfPanelEvent(ev: PanelEvent | undefined): ChatArtifact[] {
  if (!ev || ev.kind !== 'ops') return []
  const out: ChatArtifact[] = []
  for (const op of ev.ops ?? []) {
    const o = op as { op?: string; target?: string; node?: { children?: unknown[] } }
    if (o.op !== 'replace' || !o.node) continue
    if (o.target === CHAT_ARTIFACTS_NODE) {
      for (const k of o.node.children ?? []) {
        const a = artifactOfNode(k)
        if (a) out.push(a)
      }
    } else if (typeof o.target === 'string' && o.target.startsWith(ARTIFACT_NODE_PREFIX)) {
      const a = artifactOfNode(o.node)
      if (a) out.push(a)
    }
  }
  return out
}

/** The tab title of an artifact: its title, else its kind and id. */
export function chatArtifactTitle(a: ChatArtifact): string {
  if (a.title) return a.title
  if (a.kind) return `${a.kind.charAt(0).toUpperCase()}${a.kind.slice(1)}: ${a.artifactId}`
  return a.artifactId
}

/** What the chat does with one artifact: show it in the artifacts store,
 *  and open its document tab the first time it is seen. */
export interface ChatArtifactSink {
  show(a: ChatArtifact): void
  open(a: ChatArtifact, session: string | undefined): void
}

/** A reader of the chat panel events of the sessions' streams: it draws
 *  each artifact an event shows when the bridge changed it since it last
 *  drew it - a conversation replace names every artifact again, and an
 *  artifact the person edited in its tab is not drawn over with what the
 *  agent sent before -, and opens its tab the first time. A followed
 *  display panel's event (it names its follow) is not the chat's. */
export function chatArtifactReader(sink: ChatArtifactSink): (evt: ChatPanelEvent | undefined) => void {
  const drawn = new Map<string, string>()
  return (evt) => {
    if (!evt || evt.follow) return
    for (const a of artifactsOfPanelEvent(evt.event)) {
      const sig = JSON.stringify(a)
      const before = drawn.get(a.artifactId)
      if (before === sig) continue
      drawn.set(a.artifactId, sig)
      sink.show(a)
      if (before === undefined) sink.open(a, evt.session)
    }
  }
}
