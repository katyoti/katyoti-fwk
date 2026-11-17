// SPDX-License-Identifier: Apache-2.0
// `/widget/deck` route handler — the slide-deck widget (modules/slidedeck/).
//
// The deck is markdown rendered live by reveal.js (DeckView). The host
// delivers the value by postMessage — `{ markdown, streaming? }`, once the
// harness reports `ready` and again on every change — so a long deck never
// rides in the URL. A `?value=` (base64url JSON of the same shape) is still
// honoured as the initial value, which is how a deck opens standalone in a
// browser to be presented on a second screen.
//
// Presenting in the system browser (OD-004): the host posts the deck to the
// store and opens `/widget/deck?deck=<id>&present=1&token=…`; the speaker
// view's preview frames reload that same URL, so they show the deck too.
//
// No deck yet (still streaming) → DeckView's own empty stage; the route never
// fabricates a demo deck.

import { getDeck } from '../deck-store.ts'
import { renderSchemaForm } from './_lib/render-schema.ts'
import { valueFromURL } from './_lib/schema-from-url.ts'

export function deckWidget(req: Request): Response {
  const url = new URL(req.url)
  // `?deck=<id>`: a deck held by the deck store (deck-store.ts) — the page
  // starts from it and follows its updates; `&present=1` fills the window.
  const deckId = url.searchParams.get('deck')
  const injected = valueFromURL(req)
  const value = deckId
    ? { deckId, markdown: getDeck(deckId)?.markdown ?? '', assets: getDeck(deckId)?.assets, presenting: url.searchParams.get('present') === '1' }
    : typeof injected === 'string' ? { markdown: injected } : injected

  // HarnessConfig requires a schema; the deck renderer does not read it.
  const html = renderSchemaForm({
    schema: {
      name: 'deck-placeholder',
      root: 'placeholder',
      types: { placeholder: { type: 'object', fields: {} } },
    },
    value: value as Record<string, unknown>,
    mode: 'view',
    pageTitle: 'Slide deck',
    description: '',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'deck',
    embedded: true,
    fillViewport: true,
  })

  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
