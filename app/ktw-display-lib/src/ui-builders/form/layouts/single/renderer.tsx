// SPDX-License-Identifier: Apache-2.0
// `single` form-layout — NEW (Subtask 3 of gui-form). The simplest possible
// surface: every group rendered inline in a vertical Stack with the buttons
// row at the bottom. No Grid columns, no tabs, no wizard steps. Used for
// short forms (login, signup, single-page edit) where visual sectioning
// would be overkill.
//
// Inputs: same UIFormGroups the registry-driven dispatcher hands every
// layout — buildUIForm is layout-agnostic.

import { Stack } from '@mantine/core'
import type { UIFormGroups, UIFormRenderer } from '../../ui-form.types.ts'

const singleLayoutFactory = (groupsDef: UIFormGroups): UIFormRenderer => {
  return (props) => {
    const { item, isNew } = props
    const buttons = groupsDef.getButtonsToRender(isNew, item)
    const groups = groupsDef.getGroupsToRender(isNew, item)
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

export default singleLayoutFactory
