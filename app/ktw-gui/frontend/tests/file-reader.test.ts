// SPDX-License-Identifier: Apache-2.0
// A section, a diagram or a document is read in place in a
// reader drawer, never by leaving for the Files tab - a page's
// `drawer: {file, object?, title?}` action, behind its switch.

import { describe, expect, test } from 'bun:test'
import { resolveAction } from '@ktw/display-lib/ui-builders/page/ui-page.interactive.ts'
import { fileDrawer, readerDocument, readerTitle } from '../src/components/compositions/fileDrawer'

describe('a file drawer', () => {
  test('opens a markdown section in the reader, on the page object', () => {
    const a = resolveAction({ label: 'Lire', drawer: { file: '${item.path}', title: '${item.title}' } },
      { path: 'etudes/erp/01-contexte.md', title: 'Contexte' }, { id: 'erp-acme' })
    expect(a.drawer).toEqual({ file: 'etudes/erp/01-contexte.md', title: 'Contexte' })
    const open = fileDrawer(a.drawer!, 'erp-acme', true)
    expect(open).toEqual({ kind: 'reader', object: 'erp-acme', file: 'etudes/erp/01-contexte.md', title: 'Contexte' })
    expect(readerDocument(open!.file, (open as { title: string }).title)).toEqual({ source: { path: 'etudes/erp/01-contexte.md' }, title: 'Contexte' })
  })

  test('opens a draw.io diagram in the reader, titled by its file when untitled', () => {
    const a = resolveAction({ drawer: { file: '${item.path}', object: '${item.owner}' } }, { path: 'diagrams/flux.drawio', owner: 'erp-acme' }, { id: 'other' })
    expect(fileDrawer(a.drawer!, 'other', true)).toEqual({ kind: 'reader', object: 'erp-acme', file: 'diagrams/flux.drawio', title: 'flux.drawio' })
    expect(readerTitle('a/b/c.md')).toBe('c.md')
    expect(readerTitle('x.md', '  ')).toBe('x.md')
  })

  test('with the switch off, opens the file on the Files tab as before', () => {
    expect(fileDrawer({ file: 'a.md' }, 'erp-acme', false)).toEqual({ kind: 'files', object: 'erp-acme', file: 'a.md' })
  })

  test('opens nothing without a file or an object', () => {
    expect(fileDrawer({ file: '' }, 'erp-acme', true)).toBeUndefined()
    expect(fileDrawer({ file: 'a.md' }, '', true)).toBeUndefined()
  })

  test('a drawer holding a node keeps its templates for the page to bind', () => {
    const node = { kind: 'text', content: '${item.path}' }
    const a = resolveAction({ drawer: { node } as never }, { path: 'x' })
    expect(a.drawer).toEqual({ node } as never)
  })
})

