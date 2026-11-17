// SPDX-License-Identifier: Apache-2.0
// The GUI's built-in display engines, registered on import. Adding an engine
// is one line here.
import { registerEngines } from './engines'
import { downloadEngine } from './engines/download'
import { deckEngine } from './engines/deck'
import { drawioEngine } from './engines/drawio'
import { htmlEngine } from './engines/html'
import { imageEngine } from './engines/image'
import { markdownEngine } from './engines/markdown'
import { mermaidEngine } from './engines/mermaid'
import { pdfEngine } from './engines/pdf'
import { textEngine } from './engines/text'
import { uiComponentsEngine } from './engines/uiComponents'

registerEngines(
  downloadEngine, textEngine, markdownEngine, htmlEngine, mermaidEngine,
  uiComponentsEngine, drawioEngine, deckEngine, imageEngine, pdfEngine,
)
