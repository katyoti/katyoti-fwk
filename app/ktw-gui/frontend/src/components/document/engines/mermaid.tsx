// SPDX-License-Identifier: Apache-2.0
// Mermaid engine — text/vnd.mermaid: one diagram (the same component draws
// ```mermaid blocks inside markdown).
import { Box, ScrollArea } from '@mantine/core'
import { MEDIA_TYPE_MERMAID, MEDIA_TYPE_PDF, MEDIA_TYPE_PNG, MEDIA_TYPE_SVG } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentText } from '../content'
import { Mermaid, renderMermaidSvg } from '../Mermaid'

function MermaidView({ doc, streaming }: EngineProps) {
  // A half-written diagram does not parse: draw it once the producer is done.
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">{streaming ? null : <Mermaid code={documentText(doc)} />}</Box>
    </ScrollArea>
  )
}

export const mermaidEngine: DisplayEngine = {
  id: 'mermaid',
  label: 'Mermaid',
  accepts: [MEDIA_TYPE_MERMAID],
  capabilities: {
    view: true, save: true,
    export: [
      { mediaType: MEDIA_TYPE_SVG, label: 'SVG' },
      { mediaType: MEDIA_TYPE_PNG, label: 'PNG' },
      { mediaType: MEDIA_TYPE_PDF, label: 'PDF' },
    ],
  },
  // SVG is drawn here, in the browser; PNG and PDF go through ktw-docs.
  exportDocument: async (doc, mediaType) => {
    if (mediaType !== MEDIA_TYPE_SVG) throw new Error(`the mermaid engine exports ${mediaType} through the host`)
    const svg = await renderMermaidSvg(documentText(doc))
    return { mediaType: MEDIA_TYPE_SVG, title: (doc.title ?? 'diagram').replace(/\.(mmd|mermaid)$/i, '') + '.svg', content: svg }
  },
  builtin: true,
  component: MermaidView,
}
