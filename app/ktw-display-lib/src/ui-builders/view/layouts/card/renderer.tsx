// SPDX-License-Identifier: Apache-2.0
// `card` view-layout — each group renders inside a Mantine Card with
// shadow + title. Stacks vertically with consistent gap. Visual emphasis
// per group (Identity / Permissions / Activity / etc).
//
// NEW from scratch (Subtask 3 of gui-view, 2026-05-04). The group's own
// renderer (vol_group_layout) already wraps fields in a Paper; here we
// wrap THAT in a higher-emphasis Card to elevate the section visually.

import { Card, Stack, Text } from '@mantine/core'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'

const cardFactory = (groupsDef: UIViewGroups): UIViewRenderer => {
  return (props) => {
    const { item } = props
    const groups = groupsDef.getGroupsToRender(item)
    const buttons = groupsDef.getButtonsToRender(item)
    return (
      <Stack gap="md">
        {groups.map((group, index) => (
          <Card key={`card-${index}`} withBorder shadow="sm" radius="md" padding="lg">
            {group.title && (
              <Text size="lg" fw={600} mb="md">
                {group.title}
              </Text>
            )}
            {group.renderer(props)}
          </Card>
        ))}
        {buttons(props)}
      </Stack>
    )
  }
}

export default cardFactory
