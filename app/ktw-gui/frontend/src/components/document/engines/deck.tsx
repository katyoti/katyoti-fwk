// SPDX-License-Identifier: Apache-2.0
// deck engine — text/markdown; variant=deck (.deck.md), a slide deck in the
// slidedeck grammar (modules/slidedeck/). UIDeck shows it with reveal.js in
// the ktw-display iframe and follows it while an agent writes it; its Present
// button goes full screen. The content is the markdown.
//
// Edit opens the deck settings panel beside the deck (the deck's settings and
// the shown slide's); Save hands the markdown back to the host.
import { UIDeck } from '../../compositions'
import { MEDIA_TYPE_DECK } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentText } from '../content'

function DeckView({ doc, mode, streaming, onChange, onExitEdit, host }: EngineProps) {
  return (
    <UIDeck
      markdown={documentText(doc)}
      deckKey={doc.source?.path || doc.source?.uri || doc.title}
      objectId={host.context?.objectId || undefined}
      sourcePath={absolutePath(doc.source?.path)}
      streaming={streaming && !host.streamingComplete}
      universe={host.universe}
      fillHeight={host.fillHeight ?? true}
      editing={mode === 'edit'}
      onSave={onChange ? (markdown) => onChange(markdown) : undefined}
      onEditEnd={onExitEdit}
    />
  )
}

/** A deck file's absolute path (its images are read beside it); a module's
 *  relative path is not one. */
const absolutePath = (p?: string) => (p && (/^[A-Za-z]:[\\/]/.test(p) || p.startsWith('/')) ? p : undefined)

export const deckEngine: DisplayEngine = {
  id: 'deck',
  label: 'Slide deck',
  accepts: [MEDIA_TYPE_DECK],
  capabilities: {
    view: true, stream: true, save: true, edit: true,
    // No export here: the deck's own ⇩ Export menu (UIDeck) is the one list —
    // PDF, PowerPoint as pictures or editable slides, one HTML page that
    // presents offline.
  },
  builtin: true,
  component: DeckView,
}
