// SPDX-License-Identifier: Apache-2.0
// `detail-pane` view-layout — DEFAULT for gui-view. Vertical Stack with
// per-group bordered Paper containers + buttons row at the bottom.
// Best for record-detail pop-out + sidebar contexts.
//
// NEW from scratch (Subtask 3 of gui-view, 2026-05-04). Equivalent role
// in view-layouts/ to `groups` in form-layouts/ (= the default).

import { Stack } from '@mantine/core'
import type { UIViewGroups, UIViewRenderer } from '../../ui-view.types.ts'

const detailPaneFactory = (groupsDef: UIViewGroups): UIViewRenderer => {
  return (props) => {
    const { item } = props
    const groups = groupsDef.getGroupsToRender(item)
    const buttons = groupsDef.getButtonsToRender(item)
    return (
      <Stack gap="md">
        {groups.map((group, index) => (
          <Stack key={`group-${index}`}>{group.renderer(props)}</Stack>
        ))}
        {buttons(props)}
      </Stack>
    )
  }
}

export default detailPaneFactory
