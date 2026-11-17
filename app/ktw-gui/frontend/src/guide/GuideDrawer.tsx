// SPDX-License-Identifier: Apache-2.0
/**
 * GuideDrawer — the guide where there are no tabs (the universe picker and
 * the other non-universe routes). Same content as the `guide` tab. Mounted
 * once in BaseLayout beside the player.
 */

import { Drawer } from '@mantine/core'
import { GuideBrowser } from './GuideBrowser'
import { useGuideDrawer } from './surface'
import { useGuideT } from './i18n'

export function GuideDrawer() {
  const open      = useGuideDrawer((s) => s.open)
  const topics    = useGuideDrawer((s) => s.topics)
  const setTopics = useGuideDrawer((s) => s.setTopics)
  const close     = useGuideDrawer((s) => s.close)
  const t         = useGuideT()

  return (
    <Drawer
      opened={open}
      onClose={close}
      position="right"
      size="lg"
      title={t('drawer.title')}
      styles={{ body: { height: 'calc(100% - 60px)', padding: 0 } }}
    >
      {open && <GuideBrowser topics={topics} onTopicsChange={setTopics} onStarted={close} />}
    </Drawer>
  )
}
