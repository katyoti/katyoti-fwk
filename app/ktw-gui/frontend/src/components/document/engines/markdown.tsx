// SPDX-License-Identifier: Apache-2.0
// Markdown engine — the one markdown setup of the GUI (components/Markdown).
import { Box, ScrollArea } from '@mantine/core'
import { Markdown } from '../../Markdown'
import { MEDIA_TYPE_DOCX, MEDIA_TYPE_HTML, MEDIA_TYPE_MARKDOWN, MEDIA_TYPE_PDF, MEDIA_TYPE_PPTX } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentText } from '../content'
import { folderOf } from '../fileLinks'

function MarkdownView({ doc, host }: EngineProps) {
  const body = (
    // Full width, beside its table of contents when long.
    <Box p="md">
      <Markdown toc text={documentText(doc)}
        media={host.file ? { folder: folderOf(host.file.path), embed: host.file.embed, open: host.file.open } : undefined} />
    </Box>
  )
  return host.fillHeight === false ? body : <ScrollArea style={{ height: '100%' }}>{body}</ScrollArea>
}

export const markdownEngine: DisplayEngine = {
  id: 'markdown',
  label: 'Markdown',
  accepts: [MEDIA_TYPE_MARKDOWN],
  capabilities: {
    view: true, stream: true, save: true,
    // Converted by ktw-docs (Pandoc); mermaid blocks are drawn on the way.
    export: [
      { mediaType: MEDIA_TYPE_PDF, label: 'PDF' },
      { mediaType: MEDIA_TYPE_DOCX, label: 'Word' },
      { mediaType: MEDIA_TYPE_PPTX, label: 'PowerPoint' },
      { mediaType: MEDIA_TYPE_HTML, label: 'HTML' },
    ],
  },
  builtin: true,
  component: MarkdownView,
}
