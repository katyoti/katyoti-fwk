// SPDX-License-Identifier: Apache-2.0
// The chat draws a run's UI artifacts from its chat panel's events
// (src/lib/chatArtifacts.ts): the conversation replace and
// the artifact replaces of the session's `panel.event`, each artifact whole,
// laid in the artifacts store as it is - no patch applied by the GUI.
import { describe, expect, test } from 'bun:test'
import { artifactsOfPanelEvent, chatArtifactReader, chatArtifactTitle, type ChatArtifact, type ChatPanelEvent } from '../src/lib/chatArtifacts'
import { useArtifactsStore } from '../src/store/artifactsSlice'

const card = (a: Partial<ChatArtifact> & { artifactId: string }) => ({
  kind: 'card',
  id: `artifact:${a.artifactId}`,
  artifact: { status: 'streaming', ...a },
})

// A run's chat panel events as the terminal stream carries them (the Go
// bridge's shape, chatpanel.Bridge): open, f1 mounted, f1 patched, f1
// completed, d2 mounted beside a message.
const events: ChatPanelEvent[] = [
  { type: 'panel.event', session: 's1', event: { panel: 'p', seq: 1, kind: 'open', open: { type: 'ktw-agents:/panel-type/chat' } } },
  { type: 'panel.event', session: 's1', event: { panel: 'p', seq: 2, kind: 'ops', ops: [
    { op: 'replace', target: 'chat:artifacts', node: { kind: 'vbox', id: 'chat:artifacts', children: [card({ artifactId: 'f1', kind: 'form', value: { name: 'A' } })] } },
  ] as never } },
  { type: 'panel.event', session: 's1', event: { panel: 'p', seq: 3, kind: 'ops', ops: [
    { op: 'replace', target: 'artifact:f1', node: card({ artifactId: 'f1', kind: 'form', value: { name: 'Ada' } }) },
  ] as never } },
  { type: 'panel.event', session: 's1', event: { panel: 'p', seq: 4, kind: 'ops', ops: [
    { op: 'replace', target: 'artifact:f1', node: card({ artifactId: 'f1', kind: 'form', value: { name: 'Ada' }, status: 'complete' }) },
  ] as never } },
  { type: 'panel.event', session: 's1', event: { panel: 'p', seq: 5, kind: 'ops', ops: [
    { op: 'replace', target: 'chat:artifacts', node: { kind: 'vbox', id: 'chat:artifacts', children: [
      card({ artifactId: 'f1', kind: 'form', value: { name: 'Ada' }, status: 'complete' }),
      { kind: 'text', id: 'message:m1', content: 'here it is', role: 'assistant' },
      card({ artifactId: 'd2', kind: 'drawio', mediaType: 'application/vnd.jgraph.mxfile', value: '<mxfile/>' }),
    ] } },
  ] as never } },
]

describe('chat artifacts from the chat panel', () => {
  test('every artifact an event shows, in its order; none in an open or a message', () => {
    expect(artifactsOfPanelEvent(events[0].event)).toEqual([])
    expect(artifactsOfPanelEvent(events[2].event).map((a) => a.value)).toEqual([{ name: 'Ada' }])
    expect(artifactsOfPanelEvent(events[4].event).map((a) => a.artifactId)).toEqual(['f1', 'd2'])
  })

  test('the reader shows each change once, opens each artifact once, and leaves a followed panel alone', () => {
    const shown: ChatArtifact[] = []
    const opened: Array<[string, string | undefined]> = []
    const read = chatArtifactReader({ show: (a) => shown.push(a), open: (a, s) => opened.push([a.artifactId, s]) })
    for (const e of events) read(e)
    read({ ...events[2], follow: 'f-1' })
    expect(opened).toEqual([['f1', 's1'], ['d2', 's1']])
    // f1: mount, patch, completion; the last conversation replace names it
    // unchanged and is not drawn over it.
    expect(shown.map((a) => `${a.artifactId}:${a.status}`)).toEqual(['f1:streaming', 'f1:streaming', 'f1:complete', 'd2:streaming'])
  })

  test('the store lays the artifact as the chat panel shows it', () => {
    const s = useArtifactsStore.getState()
    s.reset()
    const read = chatArtifactReader({ show: (a) => s.showArtifact(a), open: () => {} })
    for (const e of events.slice(0, 3)) read(e)
    let f1 = useArtifactsStore.getState().artifacts.get('f1')!
    expect(f1.value).toEqual({ name: 'Ada' })
    expect(f1.status).toBe('streaming')
    expect(f1.mode).toBe('view')
    read(events[3])
    f1 = useArtifactsStore.getState().artifacts.get('f1')!
    expect(f1.status).toBe('complete')
    expect(f1.completedAt).toBeGreaterThan(0)
    expect(chatArtifactTitle(f1 as ChatArtifact)).toBe('Form: f1')
  })
})
