// SPDX-License-Identifier: Apache-2.0
// <FileReaderDrawer> — a file of an object read in place in a side drawer:
// what a page's `drawer: {file, object?}` action opens - a
// study's section (markdown), a diagram (draw.io), a deposited document -,
// never by leaving for the Files tab. The file is shown through the one
// document entry point (DocumentView): read and typed in Go, its media type
// picking the engine; a relative link inside it opens its target in the same
// drawer.

import { useState } from 'react'
import { Drawer } from '@mantine/core'

import { DocumentView } from '../../components/document/DocumentView'
import type { Document } from '../../components/document/engines'
import { setFileReaderDrawer } from '../../components/compositions/PageHost'
import { readerDocument, readerTitle } from '../../components/compositions/fileDrawer'

export function FileReaderDrawer({ universeName, object, file, title, onClose }: {
  universeName: string
  object: string
  file: string
  title: string
  onClose: () => void
}) {
  // A link followed inside the reader shows its target here, titled by it.
  const [shown, setShown] = useState<{ file: string; title: string }>({ file, title })
  return (
    <Drawer opened onClose={onClose} position="right" size="xl" title={shown.title}
      data-guide="page.reader" data-guide-name={shown.file}>
      <div style={{ height: 'calc(100vh - 90px)' }}>
        <DocumentView key={shown.file} doc={readerDocument(shown.file, shown.title) as Document}
          universe={universeName} nodeId={object} host={{ fillHeight: true }}
          onOpenFile={(p) => setShown({ file: p, title: readerTitle(p) })} />
      </div>
    </Drawer>
  )
}

// A page's file drawer opens through this view (PageHost cannot import it:
// DocumentView reaches PageHost).
setFileReaderDrawer(FileReaderDrawer)
