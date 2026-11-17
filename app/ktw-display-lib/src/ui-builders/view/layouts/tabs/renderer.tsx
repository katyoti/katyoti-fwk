// SPDX-License-Identifier: Apache-2.0
// `tabs` view-layout — each group becomes a Mantine Tab. Best for long
// views with naturally distinct sections (Profile / Activity /
// Permissions / etc.) where the user typically focuses on one section
// at a time.
//
// Adapted from gui-form's `content-types/form-layouts/tabs/renderer.tsx`
// (Subtask 3 of gui-view, 2026-05-04). Differences:
//   - Wired to UIViewGroups + UIViewRenderer instead of UIFormGroups + UIFormRenderer
//   - Falls back to single Paper render when 0 or 1 groups (same)
//   - ICON_MAP heuristic kept (same title→icon mapping)

import { Paper, Stack, Tabs } from '@mantine/core'
import {
  IconDatabase,
  IconFileDescription,
  IconFolders,
  IconLink,
  IconList,
  IconSettings,
  IconUser,
  type Icon as TablerIcon,
} from '@tabler/icons-react'
import { useMemo, useState } from 'react'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'

const ICON_MAP: Record<string, TablerIcon> = {
  informations: IconUser,
  info: IconUser,
  profile: IconUser,
  user: IconUser,
  preferences: IconSettings,
  settings: IconSettings,
  metadata: IconDatabase,
  data: IconDatabase,
  contexts: IconFolders,
  folders: IconFolders,
  description: IconFileDescription,
  details: IconFileDescription,
  relations: IconLink,
  links: IconLink,
  list: IconList,
  items: IconList,
}

function getGroupIcon(title?: string): TablerIcon {
  if (!title) return IconFileDescription
  const key = title.toLowerCase().trim()
  return ICON_MAP[key] || IconFileDescription
}

const viewTabsFactory = (groupsDef: UIViewGroups): UIViewRenderer => {
  return (props) => {
    const { item } = props
    const [activeTab, setActiveTab] = useState<string | null>(null)

    const groups = groupsDef.getGroupsToRender(item)
    const buttons = groupsDef.getButtonsToRender(item)

    useMemo(() => {
      if (!activeTab && groups.length > 0) {
        setActiveTab('tab-0')
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [groups.length])

    if (groups.length === 0) {
      return <Stack gap="md">{buttons(props)}</Stack>
    }

    if (groups.length === 1) {
      return (
        <Stack gap="md">
          <Paper p="md" withBorder>
            {groups[0].renderer(props)}
          </Paper>
          {buttons(props)}
        </Stack>
      )
    }

    return (
      <Stack gap="md">
        <Paper shadow="sm" p="lg" radius="md">
          <Tabs value={activeTab} onChange={setActiveTab}>
            <Tabs.List>
              {groups.map((group, index) => {
                const Icon = getGroupIcon(group.title)
                return (
                  <Tabs.Tab
                    key={`tab-${index}`}
                    value={`tab-${index}`}
                    leftSection={<Icon size={16} />}
                  >
                    {group.title || `Tab ${index + 1}`}
                  </Tabs.Tab>
                )
              })}
            </Tabs.List>

            {groups.map((group, index) => (
              <Tabs.Panel key={`panel-${index}`} value={`tab-${index}`} pt="lg">
                {group.renderer(props)}
              </Tabs.Panel>
            ))}
          </Tabs>
        </Paper>
        {buttons(props)}
      </Stack>
    )
  }
}

export default viewTabsFactory
