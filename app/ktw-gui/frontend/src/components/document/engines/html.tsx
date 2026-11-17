// SPDX-License-Identifier: Apache-2.0
// HTML engine — a sandboxed srcdoc iframe.
import { useDebouncedValue } from '@mantine/hooks'
import { MEDIA_TYPE_HTML } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { documentText } from '../content'

function HtmlView({ doc }: EngineProps) {
  // Each srcDoc change re-parses the iframe (no in-place update), so while a
  // document streams its updates are debounced to ~5 a second instead of one
  // reload per delta.
  const [html] = useDebouncedValue(documentText(doc), 200)
  return (
    <iframe
      srcDoc={html}
      sandbox="allow-scripts"
      title={doc.title ?? 'HTML document'}
      style={{ width: '100%', height: '100%', border: 'none', background: 'var(--mantine-color-body)' }}
    />
  )
}

export const htmlEngine: DisplayEngine = {
  id: 'html',
  label: 'HTML',
  accepts: [MEDIA_TYPE_HTML],
  capabilities: { view: true, stream: true, save: true },
  builtin: true,
  component: HtmlView,
}
