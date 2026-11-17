// SPDX-License-Identifier: Apache-2.0
// UIWizard builder — composes a schema + DSL wizard-extension into a
// UIWizard runtime.
//
// NEW from scratch (gui-action Subtask 4, 2026-05-04). For each declared
// step, synthesize a per-step UIFormDef that filters the root schema's
// fields down to the step's `fields[]` list, then call `buildUIForm()`
// to produce a step-form. The wizard view component swaps between these
// forms as the user advances.

import i18next from 'i18next'

import type { Schema } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import { type UITexts, makeUITexts } from '../../ui-core/i18n/index.ts'
import { buildUIForm, type UIFormDef } from '../form/index.ts'
import { createAction, type UIActionRef } from '../../ui-core/action/index.ts'

import type { UIWizardDef, UIWizardRef, UIWizardStepDef } from './ui-wizard.schema.ts'
import type { UIWizard, UIWizardStep } from './ui-wizard.types.ts'

// =================================================================================================
// Step → form synthesis
// =================================================================================================

/**
 * Build a UIFormDef for a single wizard step from its declared `fields[]`.
 * The synthesized form has ONE group containing the named fields and uses
 * the `single` form-layout (no surrounding section title — the wizard
 * shell already provides the step title).
 *
 * Empty `fields[]` is treated as a review step — synthesizes a form with
 * zero groups; the form-builder's default-group fallback then kicks in
 * and renders ALL fields read-only-ish (the form-builder doesn't have a
 * read-only mode in v1; future iteration could swap to UIView for the
 * review step).
 */
function synthStepFormDef(stepDef: UIWizardStepDef): UIFormDef {
  if (!stepDef.fields || stepDef.fields.length === 0) {
    // Review step: empty groups → form-builder default-group fallback
    // (renders all root-schema fields). Caller can later pin this to
    // UIView once gui-view <UIView> is consumed.
    return { type: 'ui-form' }
  }
  return {
    type: 'ui-form',
    layout: 'single',
    groups: [
      {
        title: stepDef.title,
        description: stepDef.description,
        fields: stepDef.fields,
      },
    ],
  }
}

/**
 * Build a per-step submit action labeled appropriately. Wizard steps
 * SUBSTITUTE the form's default actions (Save/Cancel/Reset/Delete) with
 * a single action — non-last steps get "Next", last step gets "Save &
 * Complete". This avoids the confusion of a full form action row inside
 * a wizard step (where Cancel/Reset/Delete don't have wizard-level
 * semantics).
 */
function buildStepSubmitAction(isLast: boolean, texts: UITexts): UIActionRef {
  return createAction({
    id: 'submit',
    scope: 'item',
    label: isLast
      ? texts.t('buttons.complete', { defaultValue: 'Save & Complete' })
      : texts.t('buttons.next', { defaultValue: 'Next' }),
    icon: isLast ? 'IconCheck' : 'IconArrowRight',
  })
}

function buildStep<Item = any>(
  schema: Schema,
  stepDef: UIWizardStepDef,
  texts: UITexts,
  isLast: boolean,
): UIWizardStep<Item> {
  if (stepDef.node) {
    // A node step: no form - the page draws the node, « Suivant » accepts it.
    return { id: stepDef.id, title: stepDef.title, description: stepDef.description, node: stepDef.node, editable: stepDef.editable }
  }
  const formDef = synthStepFormDef(stepDef)
  const form = buildUIForm<Item>({
    schema,
    formRef: formDef,
    texts,
    // SUBSTITUTE the default form actions with our single Next/Save button.
    // Wizard chrome (Back / Cancel) lives outside the form — see UIWizardView.
    actionRefs: [buildStepSubmitAction(isLast, texts)],
    actionMode: 'substitute',
  })
  return {
    id: stepDef.id,
    title: stepDef.title,
    description: stepDef.description,
    form,
    editable: stepDef.editable,
    // canAdvance: v1 doesn't resolve named-string predicates; gated mode
    // falls back to RHF's per-field validation (when wired). Future:
    // resolve `stepDef.canAdvance` against a named-predicate registry.
  }
}

// =================================================================================================
// Wizard builder
// =================================================================================================

export function toUIWizardDef(schema: Schema, wizardRef?: UIWizardRef): UIWizardDef {
  if ((wizardRef === undefined || wizardRef === null || typeof wizardRef === 'string') && schema !== undefined) {
    wizardRef = SchemaQuery.getNamedSchemaExtension<UIWizardDef>(schema, undefined, 'ui-wizard', wizardRef)
  }
  if (wizardRef && typeof wizardRef === 'object' && 'type' in wizardRef && wizardRef.type === 'ui-wizard') {
    return wizardRef as UIWizardDef
  }
  // Fallback: a single-step wizard containing every root field. Useful
  // for `?wizard=default` deep-links when the schema has no ui-wizard
  // extension declared yet.
  return { type: 'ui-wizard', steps: [{ id: 'all', title: 'All', fields: [] }] }
}

export function toUIWizard<Item = any>(
  schema: Schema,
  wizardDef: UIWizardDef,
  texts: UITexts,
): UIWizard<Item> {
  const navigation = wizardDef.navigation ?? 'linear'
  const lastIndex = wizardDef.steps.length - 1
  const steps: UIWizardStep<Item>[] = wizardDef.steps.map((stepDef, index) =>
    buildStep<Item>(schema, stepDef, texts, index === lastIndex),
  )

  // Default value = the schema's root-type defaults (each step's form
  // captures field-level defaults; we just merge the root once).
  const rootDefault = SchemaQuery.getDefaultValue(schema) as Item

  return {
    schema,
    definition: wizardDef,
    texts,
    navigation,
    steps,
    getDefaultValue: () => ({ ...(rootDefault as object) } as Item),
    // The renderer is provided by `<UIWizardView>` — exposing it here
    // would create a runtime/types/component coupling. The harness
    // imports UIWizardView directly and passes the built UIWizard in.
    renderer: () => null,
  }
}

export type BuildUIWizardParams<Item = any> = {
  schema: Schema
  wizardRef?: UIWizardRef
  texts?: UITexts
}

export function buildUIWizard<Item = any>({
  schema,
  wizardRef,
  texts,
}: BuildUIWizardParams<Item>): UIWizard<Item> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'wizard')
  const wizardDef = toUIWizardDef(schema, wizardRef)
  return toUIWizard<Item>(schema, wizardDef, resolvedTexts)
}

export type BuildUIWizardsParams<Item = any> = Omit<BuildUIWizardParams<Item>, 'wizardRef'>

/**
 * Discover every `ui-wizard` extension on the schema. Synthesizes a
 * `'default'` entry when no extension claims that name.
 */
export function buildUIWizards<Item = any>({
  schema,
  texts,
}: BuildUIWizardsParams<Item>): Record<string, UIWizard<Item>> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'wizard')
  const wizards: Record<string, UIWizard<Item>> = {}
  const wizardDefs = SchemaQuery.getSchemaExtension<UIWizardDef>(schema, undefined, 'ui-wizard')
  for (const wizardDef of wizardDefs) {
    const key = wizardDef.name || 'default'
    wizards[key] = toUIWizard<Item>(schema, wizardDef, resolvedTexts)
  }
  if (wizards['default'] === undefined) {
    wizards['default'] = toUIWizard<Item>(
      schema,
      { type: 'ui-wizard', steps: [{ id: 'all', title: 'All', fields: [] }] },
      resolvedTexts,
    )
  }
  return wizards
}
