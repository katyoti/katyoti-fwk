// SPDX-License-Identifier: Apache-2.0
// The presenter of a standalone deck page (modules/slidedeck/, th13): one
// HTML file that holds a deck, its theme and this script, and presents with no
// server and no network — opened by a double click, sent by email.
//
// The page carries a JSON block `<script type="application/json"
// id="ktw-deck">` = { markdown, theme?: { body, assets: { <path>: <data URI> } } }
// written by the export (src/deck-export.ts). This script renders DeckView in
// its presenting mode from it: the same component the GUI shows, so the deck
// looks and moves the same.

import { createRoot } from 'react-dom/client'
import { DeckView, type LoadedTheme, type Theme } from '@ktw/display-lib/ui-builders/deck/index.ts'

interface StandaloneDeck {
  markdown: string
  theme?: { body: Theme; assets: Record<string, string> }
  /** The files beside the deck, by relative path, as data: URIs. */
  assets?: Record<string, string>
}

const raw = document.getElementById('ktw-deck')?.textContent ?? '{}'
const data = JSON.parse(raw) as StandaloneDeck
const theme: LoadedTheme | undefined = data.theme
  ? { body: data.theme.body, assetURL: (p: string) => data.theme!.assets[p] ?? '' }
  : undefined

createRoot(document.getElementById('root')!).render(
  <DeckView source={data.markdown ?? ''} presenting annotations={false} theme={theme}
    localAsset={(p: string) => data.assets?.[p] ?? p} />,
)
