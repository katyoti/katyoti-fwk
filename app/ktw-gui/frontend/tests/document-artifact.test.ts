// SPDX-License-Identifier: Apache-2.0
// A streamed artifact read as a document (src/components/document/artifact.ts).
// Run: cd app/ktw-gui/frontend && bun test tests
import { describe, expect, test } from 'bun:test'
import { artifactDocument } from '../src/components/document/artifact'
import type { ArtifactState } from '../src/store/artifactsSlice'

const artifact = (kind: string, value: unknown) =>
  ({ artifactId: 'a1', kind, value, status: 'streaming', patchCount: 0, mountedAt: 0 }) as unknown as ArtifactState

describe('artifactDocument', () => {
  test('a deck is its markdown, not a component object', () => {
    const md = '---\ntitle: T\n---\n\n# One'
    const doc = artifactDocument(artifact('deck', md), 'Deck')
    expect(doc.mediaType).toBe('text/markdown; variant=deck')
    expect(doc.content).toBe(md)
  })
  test('a deck given as {markdown} is unwrapped', () => {
    expect(artifactDocument(artifact('deck', { markdown: '# One' })).content).toBe('# One')
  })
  test('a deck with no data yet is empty text', () => {
    expect(artifactDocument(artifact('deck', undefined)).content).toBe('')
  })
  test('a ui-component is still a component object', () => {
    const doc = artifactDocument(artifact('view', { a: 1 }))
    expect(doc.mediaType).toBe('application/vnd.ktw.ui+json')
    expect((doc.content as { kind: string }).kind).toBe('view')
  })
  test('draw.io is still its XML', () => {
    expect(artifactDocument(artifact('drawio', { xml: '<mxfile/>' })).content).toBe('<mxfile/>')
  })
})
