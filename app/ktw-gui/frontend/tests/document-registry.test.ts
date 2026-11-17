// SPDX-License-Identifier: Apache-2.0
// Resolution rules of the display-engine registry.
// Run: cd app/ktw-gui/frontend && bun test tests
import { describe, expect, test } from 'bun:test'
import { EngineRegistry, matchScore, parseMediaType, type RegisteredEngine } from '../src/components/document/engineRegistry'

const engine = (id: string, accepts: string[], builtin = true): RegisteredEngine<string> => ({
  id, label: id, accepts, capabilities: { view: true }, builtin, component: id,
})

function registry() {
  const r = new EngineRegistry<string>('download')
  r.register(engine('download', []))
  r.register(engine('text', ['text/*', 'application/json', 'application/yaml', 'application/xml', '*/*+json', '*/*+xml']))
  r.register(engine('markdown', ['text/markdown']))
  r.register(engine('ui', ['application/vnd.ktw.ui+json']))
  r.register(engine('image', ['image/*']))
  r.register(engine('svg', ['image/svg+xml']))
  return r
}

describe('parseMediaType', () => {
  test('lower-cases and reads parameters', () => {
    expect(parseMediaType('Text/HTML; Charset="UTF-8"')).toEqual({ type: 'text', subtype: 'html', params: { charset: 'UTF-8' } })
  })
})

describe('matchScore', () => {
  test('exact with parameters beats exact beats type/* beats */*', () => {
    const mt = 'application/vnd.acme+json; v=2'
    expect(matchScore('application/vnd.acme+json; v=2', mt)).toBeGreaterThan(matchScore('application/vnd.acme+json', mt))
    expect(matchScore('application/vnd.acme+json', mt)).toBeGreaterThan(matchScore('application/*', mt))
    expect(matchScore('application/*', mt)).toBeGreaterThan(matchScore('*/*', mt))
  })
  test('a parameter the document does not carry, or carries differently, does not match', () => {
    expect(matchScore('application/x; v=2', 'application/x')).toBe(-1)
    expect(matchScore('application/x; v=2', 'application/x; v=3')).toBe(-1)
  })
  test('a structured suffix pattern matches any type with that suffix', () => {
    expect(matchScore('*/*+json', 'application/vnd.ktw.ui+json')).toBe(1)
    expect(matchScore('*/*+json', 'application/json')).toBe(-1)
  })
})

describe('EngineRegistry.resolve', () => {
  test('exact beats type/*', () => {
    const r = registry()
    expect(r.resolve('text/markdown')?.id).toBe('markdown')
    expect(r.resolve('image/svg+xml')?.id).toBe('svg')
    expect(r.resolve('image/png')?.id).toBe('image')
  })
  test('type/* beats a structured-suffix pattern (an SVG is an image, not XML text)', () => {
    const r = registry()
    r.unregister('svg')
    expect(r.resolve('image/svg+xml')?.id).toBe('image')
  })
  test('an exact vendor type beats the text engine’s structured-suffix pattern', () => {
    expect(registry().resolve('application/vnd.ktw.ui+json')?.id).toBe('ui')
  })
  test('an unknown textual type falls to the text engine', () => {
    const r = registry()
    expect(r.resolve('text/x-go')?.id).toBe('text')
    expect(r.resolve('application/vnd.acme.graph+json')?.id).toBe('text')
    expect(r.resolve('application/yaml')?.id).toBe('text')
  })
  test('anything else falls to the download card, never to nothing', () => {
    const r = registry()
    expect(r.resolve('application/octet-stream')?.id).toBe('download')
    expect(r.resolve('')?.id).toBe('download')
  })
  test('parameters of the document are ignored by a pattern without them', () => {
    expect(registry().resolve('text/markdown; charset=utf-8')?.id).toBe('markdown')
  })
  test('a built-in engine wins a tie over a contributed one, whatever the order', () => {
    const r = registry()
    r.register(engine('sidecar-md', ['text/markdown'], false))
    expect(r.resolve('text/markdown')?.id).toBe('markdown')
  })
  test('a contributed engine is chosen for a type only it accepts', () => {
    const r = registry()
    r.register(engine('graph', ['application/vnd.leanix.graph+json'], false))
    expect(r.resolve('application/vnd.leanix.graph+json')?.id).toBe('graph')
  })
  test('re-registering an id replaces the engine and notifies', () => {
    const r = registry()
    let calls = 0
    const off = r.subscribe(() => calls++)
    const v = r.getVersion()
    r.register({ ...engine('markdown', ['text/markdown']), label: 'Markdown v2' })
    off()
    expect(r.get('markdown')?.label).toBe('Markdown v2')
    expect(r.list().filter((e) => e.id === 'markdown')).toHaveLength(1)
    expect(calls).toBe(1)
    expect(r.getVersion()).toBe(v + 1)
  })
})

// A slide deck is markdown with a variant (modules/slidedeck/): the deck
// engine takes it, plain markdown stays with the markdown engine, and a host
// without the deck engine still shows a deck — as markdown.
describe('slide deck media type', () => {
  const DECK = 'text/markdown; variant=deck'
  test('the deck engine wins over markdown for a deck', () => {
    const r = registry()
    r.register(engine('deck', [DECK]))
    expect(r.resolve(DECK)?.id).toBe('deck')
    expect(r.resolve('text/markdown; variant="Deck"')?.id).toBe('markdown')
    expect(r.resolve('text/markdown')?.id).toBe('markdown')
  })
  test('without the deck engine a deck shows as markdown', () => {
    expect(registry().resolve(DECK)?.id).toBe('markdown')
  })
})
