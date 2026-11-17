// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `stepper` node: the states of a
// workflow in order - the steps before `current` done, `current` active, all
// done when `completed` -, a click on a step running the node's action with
// the step as its item. An object's header shows its workflow with it. With
// `variant: dots` it is a row of dots, each step's state in the shared status
// colours and its name on hover, small enough for a list row:
// done green, running orange and pulsing, on-you (a person's step) violet,
// failed or blocked red, todo empty.

import { dataAttrs } from './data-attrs.ts'
import { Box, Group, Stepper, Tooltip } from '@mantine/core'

import { usePageContext, useRunAction } from '../page-context.tsx'
import { boundValue } from './marks.ts'
import type { UIStepperNode } from '../ui-page.interactive.ts'
import { statusChip } from '../../fields/_planning/palette.ts'

/** stepperActive is the index Mantine's Stepper takes: the current step's,
 *  the step count when every step is done, 0 when the current step is none
 *  of them (nothing done yet). */
export function stepperActive(node: Pick<UIStepperNode, 'steps' | 'current' | 'completed'>): number {
  if (node.completed) return node.steps.length
  const i = node.steps.findIndex((s) => s.id === node.current)
  return i < 0 ? 0 : i
}

/** stepDotStatus is a step's state as its dot shows it: its own, else done
 *  before the current step, running at it, todo after - every step done when
 *  the workflow completed. */
export function stepDotStatus(node: Pick<UIStepperNode, 'steps' | 'current' | 'completed'>, i: number): string {
  const own = node.steps[i]?.status
  if (own) return own
  if (node.completed) return 'done'
  const at = node.steps.findIndex((s) => s.id === node.current)
  if (at < 0) return 'todo'
  return i < at ? 'done' : i === at ? 'running' : 'todo'
}

/** The keyframes of a running dot's pulse. */
const DOT_PULSE_CSS = '@keyframes ktw-dot-pulse{0%,100%{opacity:1}50%{opacity:.35}}'

/** stepDotLook is a dot's fill, border and whether it pulses: todo empty,
 *  every other state filled in its status colour, running pulsing. */
export function stepDotLook(status: string): { background: string; border: string; pulse: boolean } {
  const look = statusChip(status)
  return { background: status === 'todo' ? 'transparent' : look.fg, border: look.border, pulse: status === 'running' }
}

function StepDots({ node }: { node: UIStepperNode }) {
  const run = useRunAction()(node.action)
  const { draft } = usePageContext()
  return (
    <Group {...dataAttrs(node)} gap={4} wrap="nowrap" data-element={node.id ? `stepper:${node.id}` : undefined}>
      {node.steps.some((_, i) => stepDotStatus(node, i) === 'running') && <style>{DOT_PULSE_CSS}</style>}
      {node.steps.map((s, i) => {
        const status = stepDotStatus(node, i)
        const look = stepDotLook(status)
        const label = String(boundValue(draft, s.labelField, s.label))
        return (
          <Tooltip key={s.id} label={`${label} · ${status}`} withArrow>
            <Box component="span" data-step={s.id} data-status={status} aria-label={`${label}: ${status}`}
              onClick={run ? () => run(s) : undefined}
              style={{ width: 9, height: 9, borderRadius: '50%', display: 'inline-block', flex: '0 0 auto',
                background: look.background, border: `1.5px solid ${look.border}`,
                animation: look.pulse ? 'ktw-dot-pulse 1.2s ease-in-out infinite' : undefined,
                cursor: run ? 'pointer' : 'default' }} />
          </Tooltip>
        )
      })}
    </Group>
  )
}

export function StepperRenderer({ node }: { node: UIStepperNode }) {
  if (node.variant === 'dots') return <StepDots node={node} />
  return <StepperSteps node={node} />
}

function StepperSteps({ node }: { node: UIStepperNode }) {
  const run = useRunAction()(node.action)
  const { draft } = usePageContext()
  return (
    <Stepper {...dataAttrs(node)} active={stepperActive(node)} size={node.size ?? 'sm'} data-element={node.id ? `stepper:${node.id}` : undefined}
      onStepClick={run ? (i) => run(node.steps[i]) : undefined} allowNextStepsSelect={!!run}>
      {node.steps.map((s) => (
        <Stepper.Step key={s.id} label={String(boundValue(draft, s.labelField, s.label))} description={s.description} data-guide="stepper.step" data-guide-name={s.id} />
      ))}
    </Stepper>
  )
}
