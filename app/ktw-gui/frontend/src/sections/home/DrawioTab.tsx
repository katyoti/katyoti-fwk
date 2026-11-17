// SPDX-License-Identifier: Apache-2.0
/**
 * DrawioTab — the Home tab's "Drawio" action: a new diagram, shown through the
 * document entry point, so Edit (the draw.io editor) and Save (a .drawio file)
 * come from the drawio engine's capabilities.
 */
import { useState } from 'react'
import { DocumentView } from '../../components/document/DocumentView'
import { MEDIA_TYPE_DRAWIO } from '../../generated/media-types'
import { useAppStore } from '../../store'

const BLANK_DIAGRAM =
  '<mxfile host="ktw-gui"><diagram name="Page-1" id="page-1"><mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="1100" pageHeight="850"><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel></diagram></mxfile>'

/** The new diagram's file name (data: the name it is saved under). */
const DIAGRAM_FILE = 'diagram.drawio' // i18n-ignore

export function DrawioTab() {
  const universe = useAppStore((s) => s.displayedUniverseName ?? undefined)
  const [xml, setXml] = useState(BLANK_DIAGRAM)
  return (
    <DocumentView
      doc={{ mediaType: MEDIA_TYPE_DRAWIO, title: DIAGRAM_FILE, content: xml }}
      universe={universe}
      header
      host={{ fillHeight: true }}
      onContentChange={(content) => setXml(String(content ?? ''))}
    />
  )
}
