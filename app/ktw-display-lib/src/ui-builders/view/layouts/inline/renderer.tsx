// SPDX-License-Identifier: Apache-2.0
// `inline` view-layout — flat horizontal Group of all fields, group
// framing dropped. Best for compact row-summary surfaces (table-cell
// expansion, tooltip card, breadcrumb summary).
//
// NEW from scratch (Subtask 3 of gui-view, 2026-05-04). Renders each
// group's content inline with `wrap` so narrow viewports degrade
// gracefully. Per-group titles are dropped (the inline shape can't
// accommodate them sensibly).

import { Group, Stack } from '@mantine/core'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'

const inlineFactory = (groupsDef: UIViewGroups): UIViewRenderer => {
  return (props) => {
    const { item } = props
    const groups = groupsDef.getGroupsToRender(item)
    const buttons = groupsDef.getButtonsToRender(item)
    return (
      <Stack gap="sm">
        <Group gap="md" align="flex-start" wrap="wrap">
          {groups.map((group, index) => (
            <Group key={`inline-${index}`} gap="sm" align="flex-start">
              {group.renderer(props)}
            </Group>
          ))}
        </Group>
        {buttons(props)}
      </Stack>
    )
  }
}

export default inlineFactory
