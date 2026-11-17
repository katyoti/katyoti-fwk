// SPDX-License-Identifier: Apache-2.0
// Image engine — image/*: the bytes (base64) or an SVG's text, drawn at their
// own size, scaled down to fit.
import { Center, ScrollArea } from '@mantine/core'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentDataUrl } from '../content'

function ImageView({ doc }: EngineProps) {
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Center p="md">
        <img src={documentDataUrl(doc)} alt={doc.title ?? ''} style={{ maxWidth: '100%', height: 'auto' }} />
      </Center>
    </ScrollArea>
  )
}

export const imageEngine: DisplayEngine = {
  id: 'image',
  label: 'Image',
  accepts: ['image/*'],
  capabilities: { view: true, save: true },
  builtin: true,
  component: ImageView,
}
