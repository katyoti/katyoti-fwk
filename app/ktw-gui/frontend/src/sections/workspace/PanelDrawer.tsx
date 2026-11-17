// SPDX-License-Identifier: Apache-2.0
// <PanelDrawer> — a declared panel of an object shown in a side drawer: what
// a page's `drawer: {panel, object}` action opens. The panel
// is read from the object's detail (its declared panels) and drawn by the one
// renderer every declared panel uses; the drawer closes on its own button.

import { useEffect, useState } from 'react'
import { Alert, Drawer, Loader, Text } from '@mantine/core'

import { WorkspaceObjectDetailFetch } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { RenderDefView, SourcePanel, type RenderDef } from './RenderDefView'
import { pageIdentity } from './resolvePanel'
import { setPanelDrawer } from '../../components/compositions/PageHost'
import { panelOf } from './panelOf'
import { objectRef } from './objectRef'


export function PanelDrawer({ universeName, object, panel, title, onClose }: {
  universeName: string
  object: string
  panel: string
  title?: string
  onClose: () => void
}) {
  const [detail, setDetail] = useState<main.WorkspaceObjectDetail | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let live = true
    WorkspaceObjectDetailFetch(universeName, object)
      .then((d) => { if (live) setDetail(d) })
      .catch((e) => { if (live) setError(String(e)) })
    return () => { live = false }
  }, [universeName, object])
  const tab = panelOf(detail?.tabs, panel)
  // The id its panels send: the one the page's row named (bare,
  // or qualified for another project), qualified once the detail names the
  // project holding the object.
  const sendId = detail?.id ? objectRef(detail.id, detail.project) : object
  return (
    <Drawer opened onClose={onClose} position="right" size="xl" title={title ?? tab?.name} data-guide="page.drawer">
      {error ? <Alert color="red">{error}</Alert>
        : !detail ? <Loader size="sm" />
        : !tab ? <Text size="sm" c="dimmed">{panel}</Text>
        : tab.kind === 'source' && tab.source ? (
          <SourcePanel source={tab.source} content={tab.content as unknown as RenderDef | undefined}
            input={(tab as { input?: Record<string, unknown> }).input} module={detail.module}
            universeName={universeName} objectId={sendId} objectName={detail.title || detail.id} detail={detail} />
        ) : tab.content ? (
          <RenderDefView key={pageIdentity(universeName, sendId, undefined, panel)} panel={panel}
            def={tab.content as unknown as RenderDef} universeName={universeName}
            objectId={sendId} objectName={detail.title || detail.id} detail={detail} />
        ) : null}
    </Drawer>
  )
}

// A page's drawer action opens a panel through this view (PageHost cannot
// import it: RenderDefView imports PageHost).
setPanelDrawer(PanelDrawer)
