// SPDX-License-Identifier: Apache-2.0
// `wizard` form-layout — NEW (Subtask 3 of gui-form). Each schema group
// becomes one step. Mantine `Stepper` shows progress; only the active
// step's group is mounted. The bottom row exposes Prev / Next navigation
// + the form's regular buttons row on the FINAL step (so submit only
// appears once the user has walked through every step).
//
// Note: per-step validation gating ("cannot advance until step valid") is
// declared in the manifest but kept as a follow-up — the Mantine Stepper
// supports this via `allowNextStepsSelect` / per-step `loading` state, but
// wiring it requires reading react-hook-form's per-field-name validation
// state and mapping it back to the step's field set. For v1 of this layout
// we render unconstrained Next/Prev navigation.

import { Button, Group, Paper, Stack, Stepper, Text } from '@mantine/core'
import { useState } from 'react'
import type { UIFormGroups, UIFormRenderer } from '../../ui-form.types.ts'

const wizardLayoutFactory = (groupsDef: UIFormGroups): UIFormRenderer => {
  return (props) => {
    const { item, isNew } = props
    const [active, setActive] = useState(0)

    const buttons = groupsDef.getButtonsToRender(isNew, item)
    const groups = groupsDef.getGroupsToRender(isNew, item)

    if (groups.length === 0) {
      return <Stack gap="md">{buttons(props)}</Stack>
    }

    if (groups.length === 1) {
      return (
        <Stack gap="md">
          {groups[0].renderer(props)}
          {buttons(props)}
        </Stack>
      )
    }

    const lastIndex = groups.length - 1
    const isLast = active >= lastIndex
    const next = () => setActive((current) => Math.min(current + 1, lastIndex))
    const prev = () => setActive((current) => Math.max(current - 1, 0))

    return (
      <Stack gap="md">
        <Paper shadow="sm" p="lg" radius="md">
          <Stepper active={active} onStepClick={setActive}>
            {groups.map((group, index) => (
              <Stepper.Step key={`step-${index}`} label={group.title || `Step ${index + 1}`}>
                {group.renderer(props)}
              </Stepper.Step>
            ))}
            <Stepper.Completed>
              <Text c="dimmed" ta="center">
                All steps complete — review and submit.
              </Text>
            </Stepper.Completed>
          </Stepper>
        </Paper>

        <Group justify="space-between">
          <Button variant="default" onClick={prev} disabled={active === 0}>
            Previous
          </Button>
          {isLast ? buttons(props) : <Button onClick={next}>Next</Button>}
        </Group>
      </Stack>
    )
  }
}

export default wizardLayoutFactory
