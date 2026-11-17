// SPDX-License-Identifier: Apache-2.0
// `two-column` view-layout — two SimpleGrid columns; groups distributed
// alternately to balance vertical fill. Honors per-group `md` if set.
// Best for wider viewports / dense detail pages where vertical scrolling
// should be minimized.
//
// NEW from scratch (Subtask 3 of gui-view, 2026-05-04). Collapses to a
// single column at narrow viewports via Mantine's responsive
// `cols={{ base: 1, md: 2 }}` syntax.

import { SimpleGrid, Stack } from '@mantine/core'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'

const twoColumnFactory = (groupsDef: UIViewGroups): UIViewRenderer => {
  return (props) => {
    const { item } = props
    const groups = groupsDef.getGroupsToRender(item)
    const buttons = groupsDef.getButtonsToRender(item)

    // Distribute groups across two columns (alternating index parity).
    const left = groups.filter((_, i) => i % 2 === 0)
    const right = groups.filter((_, i) => i % 2 === 1)

    return (
      <Stack gap="md">
        <SimpleGrid cols={{ base: 1, md: 2 }} spacing="md">
          <Stack gap="md">
            {left.map((group, index) => (
              <Stack key={`left-${index}`}>{group.renderer(props)}</Stack>
            ))}
          </Stack>
          <Stack gap="md">
            {right.map((group, index) => (
              <Stack key={`right-${index}`}>{group.renderer(props)}</Stack>
            ))}
          </Stack>
        </SimpleGrid>
        {buttons(props)}
      </Stack>
    )
  }
}

export default twoColumnFactory
