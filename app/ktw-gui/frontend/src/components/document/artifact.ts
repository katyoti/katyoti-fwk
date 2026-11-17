// SPDX-License-Identifier: Apache-2.0
// A streamed LLM artifact (the artifacts store) as a document. The store keeps
// what the display-ui-component stream delivers; this is the one place that
// reads it as an envelope, and writes an engine's edit back.
import type { ArtifactState } from '../../store/artifactsSlice'
import { MEDIA_TYPE_DRAWIO, MEDIA_TYPE_UI, mediaTypeForKind } from '../../generated/media-types'
import type { Document } from './engines'

type DrawioValue = { xml?: string; filename?: string } | string | undefined

/** The document an artifact displays: a draw.io diagram's XML; the text of any
 *  other document kind (a deck's markdown, markdown, html, mermaid) — its data,
 *  as render.Def.Document says; or the ui-component its kind names, with the
 *  mode each component type starts in. */
export function artifactDocument(a: ArtifactState, title?: string): Document {
  const mediaType = mediaTypeForKind(a.kind)
  if (mediaType === MEDIA_TYPE_DRAWIO) {
    const v = a.value as DrawioValue
    const xml = typeof v === 'string' ? v : v?.xml ?? ''
    const filename = typeof v === 'string' ? undefined : v?.filename
    return { mediaType, title: filename ?? title ?? a.title, content: xml }
  }
  if (mediaType !== MEDIA_TYPE_UI) {
    const v = a.value as unknown
    // A deck may also arrive as {markdown} (display-ui-component accepts both).
    const text = typeof v === 'string'
      ? v
      : v && typeof v === 'object' && typeof (v as { markdown?: unknown }).markdown === 'string'
        ? (v as { markdown: string }).markdown
        : v ?? ''
    return { mediaType, title: title ?? a.title, content: text }
  }
  const base = { kind: a.kind, schema: a.schema, variant: a.variant, ui: a.ui, data: a.value }
  let component: Record<string, unknown>
  switch (a.kind) {
    case 'page':
      component = { kind: 'page', dslTree: a.dslTree, data: a.data }
      break
    case 'form':
      component = { ...base, mode: a.mode ?? 'edit' }
      break
    case 'wizard':
      component = { ...base, mode: a.mode === 'edit' ? 'edit' : 'create' }
      break
    default:
      component = { ...base, mode: 'view' }
  }
  return { mediaType, title: title ?? a.title, content: component }
}

/** The store value an engine's committed edit becomes (draw.io keeps its
 *  `{xml, filename}` shape in the store). */
export function artifactValueFromContent(a: ArtifactState, content: unknown): unknown {
  if (mediaTypeForKind(a.kind) === MEDIA_TYPE_DRAWIO) {
    const prev = a.value as DrawioValue
    const filename = typeof prev === 'string' ? undefined : prev?.filename ?? 'diagram.drawio'
    return { xml: String(content ?? ''), filename }
  }
  return content
}
