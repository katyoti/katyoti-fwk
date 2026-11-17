// SPDX-License-Identifier: Apache-2.0
// Public surface of the deck ui-builder: the markdown splitter and the
// grammar's types, generated from internal/extensions/slidedeck/api.
// See modules/slidedeck/.

export { DeckView } from './DeckView.tsx'
export type { DeckViewProps } from './DeckView.tsx'
export { parseDeck, parseSettings, DEFAULT_LAYOUT, DEFAULT_SLOT } from './parse.ts'
export { DEFAULT_THEME, themeCSS, slideLook } from './theme.ts'
export type { LoadedTheme, SlideLook, Theme } from './theme.ts'
export type { Slide, SlotContent } from './parse.ts'
export type { Deck } from './generated/deck-model.ts'
export type { DeckSettings } from './generated/deck.ts'
export type { SlideSettings } from './generated/deck-slide.ts'
export type { Layout } from './generated/deck-layout.ts'
export type { Finding } from './generated/deck-finding.ts'
export { localAssetPath } from './assets.ts'
