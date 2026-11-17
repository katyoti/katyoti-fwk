// SPDX-License-Identifier: Apache-2.0
// UIWizardView — React component that renders a built UIWizard.
//
// NEW from scratch (gui-action Subtask 4, 2026-05-04). One component, one
// iframe. State drives step-switching:
//   - `current` (number): index of the active step
//   - `accumulator` (Record<stepId, value>): per-step submitted values,
//     merged into a single Item on final-step submit
//
// Navigation modes:
//   - linear: only Prev/Next; previous steps locked unless step.editable
//   - free:   Stepper.Step is clickable → jump to any step
//   - gated:  Next disabled when the active step's form is invalid (RHF
//             driven via the form's own submit gating)
//
// On the final step, the form's Save action is re-labeled "Complete" and
// triggers `onComplete(merged)` instead of advancing.

import { Box, Button, Group, Stack, Stepper, Text, Title } from '@mantine/core'
import { IconArrowRight, IconCheck, IconX } from '@tabler/icons-react'
import { useState, type ReactNode } from 'react'

import { UIFormView } from '../form/index.ts'
import type { UIPageNode } from '../page/ui-page.types.ts'
import type { UIWizard } from './ui-wizard.types.ts'

export type UIWizardViewProps<Item = any> = {
  wizard: UIWizard<Item>
  item?: Item
  onComplete?: (value: Item) => Promise<void> | void
  onCancel?: () => void
  /** Draws a node step's page node - the page the wizard sits in passes its
   *  own renderer. Without it a node step says so. */
  renderNode?: (node: UIPageNode) => ReactNode
  /** `stacked` draws every step together, in order, each marked done,
   *  current or next; `paged` (the default) only the current one.
 */
  layout?: 'paged' | 'stacked'
  /** The current step's id, set from outside - by events, as the data the
   *  page receives moves: the stepper follows it and the wizard draws no
   *  « Suivant » or « Retour ». */
  current?: string
  /** The completion and cancel buttons' words, the declared actions'. */
  labels?: { complete?: string; cancel?: string }
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
}

/** wizardIndex is the current step's index: the one `current` names when it
 *  is given and known, else the wizard's own. */
export function wizardIndex(steps: { id: string }[], current: string | undefined, own: number): number {
  if (current === undefined) return own
  const i = steps.findIndex((s) => s.id === current)
  return i < 0 ? own : i
}

/** stepState is where a step stands against the current one. */
export function stepState(index: number, current: number): 'done' | 'current' | 'next' {
  return index < current ? 'done' : index === current ? 'current' : 'next'
}

export function UIWizardView<Item = any>({ wizard, item, onComplete, onCancel, renderNode, layout = 'paged', current: driven, labels, attrs }: UIWizardViewProps<Item>) {
  const { steps, navigation, texts } = wizard
  const [own, setCurrent] = useState(0)
  const current = wizardIndex(steps, driven, own)
  const byEvents = driven !== undefined
  const [accumulator, setAccumulator] = useState<Record<string, unknown>>(
    (item ?? wizard.getDefaultValue()) as Record<string, unknown>,
  )

  if (steps.length === 0) {
    return <Text c="dimmed">No steps configured for this wizard.</Text>
  }

  const isLast = current >= steps.length - 1
  const activeStep = steps[current]

  const stepClickable = navigation === 'free' && !byEvents
  const completeLabel = labels?.complete ?? texts.t('buttons.complete', { defaultValue: 'Save & Complete' })
  const cancelLabel = labels?.cancel ?? texts.t('buttons.cancel', { defaultValue: 'Cancel' })

  const goPrev = () => setCurrent((c) => Math.max(c - 1, 0))
  const goNext = () => setCurrent((c) => Math.min(c + 1, steps.length - 1))

  // Step submit: merge form values into the accumulator. Last step
  // emits onComplete; non-last advances to the next step.
  const handleStepSubmit = async (data: Item) => {
    const merged = { ...accumulator, ...(data as object) } as Item
    setAccumulator(merged as Record<string, unknown>)
    if (isLast) {
      if (onComplete) await onComplete(merged)
    } else if (!byEvents) {
      goNext()
    }
  }
  // A step's own values: a form's submit merges them, the steps stay where
  // they are (a stacked or event-driven wizard moves by itself).
  const keepStep = (data: Item) => setAccumulator((a) => ({ ...a, ...(data as object) }))

  const drawStep = (step: typeof steps[number], index: number, keep: boolean) => step.node
    ? (renderNode ? renderNode(step.node) : (
      <Text c="dimmed" size="sm">{texts.t('wizard.nodeUnavailable', { defaultValue: 'This step shows only inside a page.' })}</Text>
    ))
    : step.form && (
      <UIFormView form={step.form} item={accumulator as Item} isNew={index === 0}
        onSubmit={keep ? keepStep : handleStepSubmit} hideTitle />
    )

  const stepper = (
    <Stepper active={current} onStepClick={stepClickable ? setCurrent : undefined} allowNextStepsSelect={stepClickable}
      data-guide="wizard.stepper">
      {steps.map((step) => (
        <Stepper.Step key={step.id} label={step.title} description={step.description} />
      ))}
    </Stepper>
  )

  const finish = (
    <Group justify="space-between">
      {onCancel ? (
        <Button variant="default" leftSection={<IconX size={16} />} onClick={onCancel} data-testid="wizard.cancel">{cancelLabel}</Button>
      ) : <span />}
      {onComplete && (
        <Button leftSection={<IconCheck size={16} />} onClick={() => void onComplete(accumulator as Item)} data-testid="wizard.complete">
          {completeLabel}
        </Button>
      )}
    </Group>
  )

  if (layout === 'stacked') {
    return (
      <Stack {...attrs} gap="md">
        {stepper}
        {steps.map((step, i) => (
          <Box key={step.id} data-testid={`wizard.step.${step.id}`} data-state={stepState(i, current)}
            style={{ opacity: stepState(i, current) === 'next' ? 0.6 : 1 }}>
            <Title order={4} mb="xs">{`${i + 1} · ${step.title}`}{stepState(i, current) === 'done' && ' ✓'}</Title>
            {drawStep(step, i, true)}
          </Box>
        ))}
        {finish}
      </Stack>
    )
  }

  if (byEvents) {
    return (
      <Stack {...attrs} gap="md">
        {stepper}
        <Box style={{ minHeight: 200 }} data-testid={`wizard.step.${activeStep.id}`} data-state="current">
          {drawStep(activeStep, current, true)}
        </Box>
        {finish}
      </Stack>
    )
  }

  return (
    <Stack {...attrs} gap="md">
      <Title order={2}>{texts.t('wizard.title', { defaultValue: 'Wizard' })}</Title>

      <Stepper
        active={current}
        onStepClick={stepClickable ? setCurrent : undefined}
        allowNextStepsSelect={navigation === 'free'}
      >
        {steps.map((step) => (
          <Stepper.Step key={step.id} label={step.title} description={step.description} />
        ))}
      </Stepper>

      <Box style={{ minHeight: 200 }} data-testid={`wizard.step.${activeStep.id}`}>
        {activeStep.node ? (
          <Stack gap="md">
            {renderNode ? renderNode(activeStep.node) : (
              <Text c="dimmed" size="sm">{texts.t('wizard.nodeUnavailable', { defaultValue: 'This step shows only inside a page.' })}</Text>
            )}
            <Group justify="flex-end">
              {/* « Suivant » accepts the step as it stands. */}
              <Button data-testid="wizard.next" leftSection={isLast ? <IconCheck size={16} /> : <IconArrowRight size={16} />}
                onClick={() => void handleStepSubmit({} as Item)}>
                {isLast ? completeLabel : texts.t('buttons.next', { defaultValue: 'Next' })}
              </Button>
            </Group>
          </Stack>
        ) : activeStep.form && (
        <UIFormView
          form={activeStep.form}
          item={accumulator as Item}
          isNew={current === 0}
          onSubmit={handleStepSubmit}
          hideTitle
          // The form's per-step submit (labeled "Next" or "Save & Complete")
          // routes to handleStepSubmit. The wizard's chrome below adds
          // Back + Cancel — those are wizard-level concerns, kept out of
          // the form's action row to avoid the confusing 4-button mix
          // (Save / Cancel / Reset / Delete) inside a single step.
        />
        )}
      </Box>

      <Group justify="space-between">
        <Group gap="xs">
          {onCancel && (
            <Button
              variant="default"
              leftSection={<IconX size={16} />}
              onClick={onCancel}
              data-testid="wizard.cancel"
            >
              {cancelLabel}
            </Button>
          )}
          {current > 0 && (
            <Button variant="default" onClick={goPrev}>
              {texts.t('buttons.back', { defaultValue: 'Back' })}
            </Button>
          )}
        </Group>
        <Group gap="xs">
          <Text size="sm" c="dimmed">
            {texts.t('wizard.stepProgress', {
              defaultValue: `Step ${current + 1} of ${steps.length}`,
              current: current + 1,
              total: steps.length,
            })}
          </Text>
          {isLast && (
            <Text size="xs" c="dimmed" component="span">
              <IconCheck size={12} style={{ verticalAlign: 'middle' }} />{' '}
              {texts.t('wizard.lastStepHint', { defaultValue: 'final step' })}
            </Text>
          )}
        </Group>
      </Group>
    </Stack>
  )
}
