// SPDX-License-Identifier: Apache-2.0
// Text / code engine: plain text, JSON, YAML, XML, source code — and every
// structured (+json / +xml) type no other engine claims. Code is highlighted
// by its language (text/x-<language>, json, yaml, xml, …).
import { Box, Code, ScrollArea } from '@mantine/core'
import { CodeHighlight } from '@mantine/code-highlight'
import hljs from '../../../lib/hljs'
import { TEXT_ENGINE_ID, type DisplayEngine, type EngineProps } from '../engines'
import { codeLanguage, documentText } from '../content'

/** Past this size highlighting costs more than it gives. */
const HIGHLIGHT_MAX = 200_000

function TextView({ doc }: EngineProps) {
  const text = documentText(doc)
  // highlight.js throws on a language it does not know: those stay plain.
  const lang = codeLanguage(doc.mediaType)
  const language = lang && hljs.getLanguage(lang) ? lang : ''
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">
        {language && text.length <= HIGHLIGHT_MAX ? (
          <CodeHighlight code={text} language={language} withCopyButton />
        ) : (
          <Code block style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 12 }}>{text}</Code>
        )}
      </Box>
    </ScrollArea>
  )
}

export const textEngine: DisplayEngine = {
  id: TEXT_ENGINE_ID,
  label: 'Text',
  accepts: ['text/*', 'application/json', 'application/yaml', 'application/x-yaml', 'application/xml', '*/*+json', '*/*+xml'],
  capabilities: { view: true, stream: true, save: true },
  builtin: true,
  component: TextView,
}
