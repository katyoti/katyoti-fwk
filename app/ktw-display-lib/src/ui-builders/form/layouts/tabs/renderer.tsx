// SPDX-License-Identifier: Apache-2.0
// `tabs` form-layout — renders each form group as a Mantine Tab. Ideal for
// long forms with naturally distinct sections (Profile / Settings / Data /
// etc.). Falls back to a single-Paper render when the form has 0 or 1
// groups.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/layouts/fol-tabs.tsx during
// gui-form Subtask 3 (2026-05-03). Wrapped behind the form-layouts catalog
// dispatcher so the form widget resolves layouts via
// `getFormLayout('tabs')`. Falls back to `IconFileDescription` when a
// group's title doesn't match the ICON_MAP heuristic.

import { Paper, Stack, Tabs, Text } from '@mantine/core'
import {
  IconAlertTriangle,
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
import { type UseFormReturn, useFormContext, useWatch } from 'react-hook-form'
import { tabCount, tabInError } from '../../../fields/editing.ts'
import type { UIFormGroup, UIFormGroups, UIFormRenderer } from '../../ui-form.types.ts'

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

/** TabMark is what a tab shows after its title: an alert
 *  mark when one of its fields shows an error, else the number of rows of
 *  its array fields (or of the list its group's `count` names; none when
 *  `count: false`), nothing when there are none. Inside a form it counts
 *  the form's live values (a row added or deleted shows at once); `values`
 *  is read only without a form. */
export function TabMark({ group, index, values }: { group: Pick<UIFormGroup, 'fields' | 'arrayFields' | 'count'>; index: number; values: unknown }) {
  const form = useFormContext()
  if (!form) return <Mark group={group} index={index} values={values} inError={false} />
  return <LiveTabMark group={group} index={index} values={values} form={form} />
}

function LiveTabMark({ group, index, values, form }: { group: Pick<UIFormGroup, 'fields' | 'arrayFields' | 'count'>; index: number; values: unknown; form: UseFormReturn }) {
  const live = useWatch({ control: form.control })
  return <Mark group={group} index={index} values={live ?? values} inError={tabInError(form.formState.errors, group.fields)} />
}

function Mark({ group, index, values, inError }: { group: Pick<UIFormGroup, 'arrayFields' | 'count'>; index: number; values: unknown; inError: boolean }) {
  if (inError) {
    return (
      <Text component="span" c="red" ml={4} data-testid={`form.tab.${index}.alert`} aria-label="!">
        <IconAlertTriangle size={12} />
      </Text>
    )
  }
  const n = tabCount(values, group.arrayFields, group.count)
  if (n === undefined) return null
  return (
    <Text component="span" size="xs" c="dimmed" ml={4} data-testid={`form.tab.${index}.count`}>
      {n}
    </Text>
  )
}

const tabsLayoutFactory = (groupsDef: UIFormGroups): UIFormRenderer => {
  return (props) => {
    const { item, isNew } = props
    const [activeTab, setActiveTab] = useState<string | null>(null)

    const buttons = groupsDef.getButtonsToRender(isNew, item)
    const groups = groupsDef.getGroupsToRender(isNew, item)

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
                    data-testid={`form.tab.${index}`}
                    leftSection={<Icon size={16} />}
                  >
                    {group.title || `Tab ${index + 1}`}
                    <TabMark group={group} index={index} values={props.item} />
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

export default tabsLayoutFactory
