// SPDX-License-Identifier: Apache-2.0
// The GUI's built-in engines, resolved through THE registry the app uses —
// the check behind display.g2-document-by-media-type-AC2 ("the drawio engine
// is registered for application/vnd.jgraph.mxfile"). Removing an engine from
// builtin.ts turns its row red.
// Run: cd app/ktw-gui/frontend && bun test tests
import { describe, expect, test } from 'bun:test'
import { displayEngines } from '../src/components/document/engines'
import '../src/components/document/builtin'

describe('built-in display engines', () => {
  const cases: [string, string][] = [
    ['application/vnd.jgraph.mxfile', 'drawio'],
    ['text/markdown', 'markdown'],
    ['text/html', 'html'],
    ['text/vnd.mermaid', 'mermaid'],
    ['application/vnd.ktw.ui+json', 'ui-components'],
    ['text/markdown; variant=deck', 'deck'],
    ['text/x-go', 'text'],
    ['application/json', 'text'],
    ['image/png', 'image'],
    ['application/pdf', 'pdf'],
    ['application/octet-stream', 'download'],
  ]
  for (const [mediaType, engine] of cases) {
    test(`${mediaType} → ${engine}`, () => {
      expect(displayEngines.resolve(mediaType)?.id).toBe(engine)
    })
  }

  test('the drawio engine can view, stream, edit and save', () => {
    const drawio = displayEngines.resolve('application/vnd.jgraph.mxfile')
    expect(drawio?.builtin).toBe(true)
    expect(drawio?.capabilities).toMatchObject({ view: true, stream: true, edit: true, save: true })
  })
})

describe('export formats', () => {
  const formats = (mediaType: string) =>
    (displayEngines.resolve(mediaType)?.capabilities.export ?? []).map((f) => f.label)

  test('each engine declares what it exports', () => {
    expect(formats('text/markdown')).toEqual(['PDF', 'Word', 'PowerPoint', 'HTML'])
    expect(formats('text/vnd.mermaid')).toEqual(['SVG', 'PNG', 'PDF'])
    expect(formats('application/vnd.jgraph.mxfile')).toEqual(['SVG', 'PNG', 'PDF'])
    expect(formats('application/vnd.ktw.ui+json')).toEqual(['JSON (the component)'])
    expect(formats('text/plain')).toEqual([])
  })

  test('a component exports itself as JSON in the browser', async () => {
    const ui = displayEngines.resolve('application/vnd.ktw.ui+json')!
    const out = await ui.exportDocument!(
      { mediaType: 'application/vnd.ktw.ui+json', title: 'Book view', content: { kind: 'view', schema: 'ktw-demo:/data-type/book' } },
      'application/json',
    )
    expect(out.mediaType).toBe('application/json')
    expect(out.title).toBe('Book-view.json')
    expect(JSON.parse(String(out.content))).toEqual({ kind: 'view', schema: 'ktw-demo:/data-type/book' })
  })

  test('mermaid converts SVG itself and leaves PNG / PDF to the host', async () => {
    const mermaid = displayEngines.resolve('text/vnd.mermaid')!
    expect(mermaid.exportDocument).toBeDefined()
    await expect(mermaid.exportDocument!({ mediaType: 'text/vnd.mermaid', content: 'flowchart LR\n A-->B' }, 'image/png')).rejects.toThrow()
  })
})
