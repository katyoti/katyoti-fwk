// SPDX-License-Identifier: Apache-2.0
// drawio engine — application/vnd.jgraph.mxfile (.drawio). Wraps the drawio
// components unchanged: UIDrawio views the diagram and follows it while it
// streams; in edit mode it mounts DrawioEditor, whose Save commits the XML
// (onChange) and whose Exit leaves editing. The content is the XML; the title
// is the file name.
import { UIDrawio } from '../../compositions'
import { MEDIA_TYPE_DRAWIO, MEDIA_TYPE_PDF, MEDIA_TYPE_PNG, MEDIA_TYPE_SVG } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentText } from '../content'

function DrawioView({ doc, mode, streaming, onChange, onExitEdit, host }: EngineProps) {
  const value = { xml: documentText(doc), filename: doc.title }
  return (
    <UIDrawio
      value={value}
      pushValue={value}
      universe={host.universe}
      fillHeight={host.fillHeight ?? true}
      editable={mode === 'edit'}
      onChange={onChange ? (xml: string) => onChange(xml) : undefined}
      onExit={onExitEdit}
      streamingComplete={host.streamingComplete ?? !streaming}
    />
  )
}

export const drawioEngine: DisplayEngine = {
  id: 'drawio',
  label: 'draw.io',
  accepts: [MEDIA_TYPE_DRAWIO],
  capabilities: {
    view: true, stream: true, edit: true, save: true,
    // Rendered by ktw-docs (the draw.io CLI when installed, else Kroki).
    export: [
      { mediaType: MEDIA_TYPE_SVG, label: 'SVG' },
      { mediaType: MEDIA_TYPE_PNG, label: 'PNG' },
      { mediaType: MEDIA_TYPE_PDF, label: 'PDF' },
    ],
  },
  builtin: true,
  component: DrawioView,
}
