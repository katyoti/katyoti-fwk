// SPDX-License-Identifier: Apache-2.0
// `groups` form-layout — DEFAULT layout. Vertical Stack of (optionally
// collapsible) sections, each containing a SimpleGrid of fields. Built from
// the schema's `ui-form` extension `groups:` field. Maps directly onto the
// Mantine Grid via per-group `md:` spans.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/layouts/fol-form.tsx during
// gui-form Subtask 3 (2026-05-03). Wrapped behind the form-layouts catalog
// dispatcher so the form widget resolves layouts via
// `getFormLayout('groups')` instead of a hard-coded switch.

import { Grid, Stack } from '@mantine/core'
import type { UIFormGroups, UIFormRenderer } from '../../ui-form.types.ts'

/** Default-export factory: receives the resolved UIFormGroups (built by
 *  `buildUIForm()` from the schema's ui-form extension) and returns a
 *  UIFormRenderer that renders one Grid.Col per group with the group's
 *  buttons row at the bottom. */
const groupsLayoutFactory = (groupsDef: UIFormGroups): UIFormRenderer => {
  return (props) => {
    const { item, isNew } = props
    const buttons = groupsDef.getButtonsToRender(isNew, item)
    const groups = groupsDef.getGroupsToRender(isNew, item)
    return (
      <Stack>
        <Grid grow>
          {groups.map((group, index) => {
            const { md = 12, renderer } = group
            return (
              <Grid.Col span={md} key={`group-${index}`}>
                {renderer(props)}
              </Grid.Col>
            )
          })}
        </Grid>
        {buttons(props)}
      </Stack>
    )
  }
}

export default groupsLayoutFactory
