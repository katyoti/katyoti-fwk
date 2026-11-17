// SPDX-License-Identifier: Apache-2.0
/**
 * GalleryTabs — parent shell for the central panel's idle view.
 *
 * Wraps TaskGalleryPanel and SkillGalleryPanel in a Mantine Tabs container
 * so the user can switch between discovering tasks and discovering skills
 * without losing per-tab search/filter state (keepMounted).
 *
 * Mounted by NodeView when a context node is selected and no task is
 * running. The taskNames prop is forwarded to TaskGalleryPanel only —
 * skills are universe-wide, not node-scoped.
 *
 * Tools tab is intentionally absent; see ms07a/tool-service-ktsp-slot.md.
 */
import { Tabs } from '@mantine/core'
import { IconBook2, IconPlayerPlay } from '@tabler/icons-react'
import { TaskGalleryPanel } from './TaskGalleryPanel'
import { SkillGalleryPanel } from './SkillGalleryPanel'
import { useT } from '../../i18n'

interface GalleryTabsProps {
  taskNames?: string[]
}

export function GalleryTabs({ taskNames }: GalleryTabsProps = {}) {
  const t = useT('screens')
  return (
    <Tabs
      defaultValue="tasks"
      keepMounted
      style={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 }}
    >
      <Tabs.List>
        <Tabs.Tab value="tasks" leftSection={<IconPlayerPlay size={14} />}>
          {t('gallery.tasks')}
        </Tabs.Tab>
        <Tabs.Tab value="skills" leftSection={<IconBook2 size={14} />}>
          {t('gallery.skills')}
        </Tabs.Tab>
      </Tabs.List>

      <Tabs.Panel value="tasks" style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <TaskGalleryPanel taskNames={taskNames} />
      </Tabs.Panel>

      <Tabs.Panel value="skills" style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <SkillGalleryPanel />
      </Tabs.Panel>
    </Tabs>
  )
}
