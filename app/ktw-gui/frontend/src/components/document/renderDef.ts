// SPDX-License-Identifier: Apache-2.0
// A render definition as a document — the frontend face of render.Def.Document()
// (internal/core/render/document.go), for the definitions the GUI itself
// assembles (a panel's declared content filled with its source's answer, a
// live poll). The kind → media type table is generated from Go.
import type { RenderDef } from '../../generated/render-def'
import { MEDIA_TYPE_DRAWIO, MEDIA_TYPE_UI, mediaTypeForKind } from '../../generated/media-types'
import type { Document } from './engines'

/** Keys of a render definition that belong to the envelope or the host, not
 *  to the component a ui-component document carries. */
const ENVELOPE_KEYS = new Set(['mediaType', 'file', 'title', 'live'])

export function renderDefDocument(def: RenderDef): Document {
  const d = def as Record<string, unknown>
  const mediaType = def.mediaType || mediaTypeForKind(def.kind)
  if (mediaType === MEDIA_TYPE_UI) {
    const component = Object.fromEntries(Object.entries(d).filter(([k]) => !ENVELOPE_KEYS.has(k)))
    return { mediaType, title: def.title, content: component }
  }
  if (def.file) {
    // The media type is fixed in Go when the file is read (DocumentOpen).
    return { mediaType, title: def.title, source: { path: def.file } }
  }
  let content = d.data
  if (mediaType === MEDIA_TYPE_DRAWIO && content && typeof content === 'object' && 'xml' in content) {
    content = (content as { xml?: unknown }).xml
  }
  return { mediaType, title: def.title, content }
}
