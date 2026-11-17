// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { ExportFailure } from '../src/components/document/ExportFailure'
import { withExportSource } from '../src/components/document/exportRun'

// A document export from the file viewer: the host converts a copy, so the
// export carries the file the document was read from (its folder is where
// relative images and diagrams resolve), and a failed export says why where
// it was asked, not only in a tooltip.
const src = (p: string) => readFileSync(join(import.meta.dir, '../src', p), 'utf-8')
const html = (node: React.ReactNode) => renderToStaticMarkup(<MantineProvider>{node}</MantineProvider>)

describe('the export of a document read from a file', () => {
  test('carries the file it was read from, with the content as shown', () => {
    const shown = { mediaType: 'text/markdown', title: 'study.md', content: '![flow](media/flow.drawio)' }
    expect(withExportSource(shown, { path: 'study.md' })).toEqual({ ...shown, source: { path: 'study.md' } })
  })

  test('a document read from no file goes as it is', () => {
    const inline = { mediaType: 'text/markdown', content: '# Note' }
    expect(withExportSource(inline, undefined)).toBe(inline)
    expect(withExportSource(inline, { uri: 'ktw-example:/skill/x' })).toBe(inline)
  })

  test('the document view hands the export its source', () => {
    const view = src('components/document/DocumentView.tsx')
    expect(view).toContain('source={doc.source}')
    expect(view).toContain('withExportSource(d, source)')
  })
})

describe('a failed export', () => {
  test('shows its reason beside the badge', () => {
    const out = html(<ExportFailure label="échec de l’export" reason="Word: pandoc not found" />)
    expect(out).toContain('data-testid="document-export-failure"')
    expect(out).toContain('échec de l’export')
    expect(out).toContain('Word: pandoc not found')
  })

  test('the export menu and the run panel both show it, never only a tooltip', () => {
    const view = src('components/document/DocumentView.tsx')
    expect(view.match(/<ExportFailure label=\{t\('documentView\.exportFailed'\)\} reason=\{error\} \/>/g)?.length).toBe(2)
    expect(view).not.toContain('<Tooltip label={error}')
  })
})
