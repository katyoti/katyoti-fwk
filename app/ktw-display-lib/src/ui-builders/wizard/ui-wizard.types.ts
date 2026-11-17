// SPDX-License-Identifier: Apache-2.0
// UIWizard runtime types — what the wizard rendering pipeline operates on.
//
// NEW from scratch (gui-action Subtask 4, 2026-05-04). No old-code template;
// designed to mirror the form/view/table package shape:
//   - DSL extension types in ui-wizard.schema.ts
//   - Runtime types here
//   - Builder in ui-wizard.builder.ts
//   - React component in ui-wizard.view.tsx
//   - Public barrel in index.ts
//
// Wizard is one composition with one iframe. Inside the iframe, React
// state drives step-switching. Each step renders a form using
// buildUIForm + <UIFormView> (the gui-form pipeline). State accumulates
// across steps; the final submit emits a `submit` postMessage with the
// merged value.

import type { ReactNode } from 'react'
import type { Schema } from '../../schema/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import type { UIForm } from '../form/index.ts'
import type { UIPageNode } from '../page/ui-page.types.ts'
import type { UIWizardDef, UIWizardStepDef } from './ui-wizard.schema.ts'

/** Navigation modes — how the user moves between steps.
 *  - `linear`: strict prev/next; previous steps locked once passed
 *  - `free`: jump to any step via the Stepper indicator
 *  - `gated`: per-step `canAdvance(value)` predicate gates the Next button
 */
export type UIWizardNavigationMode = 'linear' | 'free' | 'gated'

/** Built wizard step — pre-resolved form for the step's fields plus the
 *  declarative step metadata (title / description / canAdvance / etc). */
export type UIWizardStep<Item = any> = {
  id: string
  title: string
  description?: string
  /** The form for this step (built once at wizard-build time, rendered
   *  by <UIWizardView> when the step is active); none for a node step. */
  form?: UIForm<Item>
  /** The page node the step draws instead of a form. */
  node?: UIPageNode
  /** When true, the user can navigate back to this step after passing
   *  it (linear mode only — free mode allows any backward jump). */
  editable?: boolean
  /** Predicate gating Next button (gated mode only). Receives the
   *  step's current values; returns true when ready to advance. */
  canAdvance?: (value: Item) => boolean
}

/** Render parameters for the wizard surface. */
export type UIWizardRendererParams<Item = any> = {
  /** Initial value (optional — wizard accumulator seeds from this). */
  item?: Item
  /** Fired when the wizard reaches the final step + the user clicks Save.
   *  Receives the fully accumulated value (merge of every step's submit). */
  onComplete?: (value: Item) => Promise<void> | void
  /** Fired on Cancel click. */
  onCancel?: () => void
}

export type UIWizardRenderer<Item = any> = (props: UIWizardRendererParams<Item>) => ReactNode

/** Composed wizard — what `buildUIWizard()` returns. */
export type UIWizard<Item = any> = {
  schema: Schema
  definition: UIWizardDef
  texts: UITexts
  navigation: UIWizardNavigationMode
  steps: UIWizardStep<Item>[]
  /** Initial accumulator value (per-step defaults merged together). */
  getDefaultValue(): Item
  /** Renders the full wizard chrome (Stepper + active step + nav buttons). */
  renderer: UIWizardRenderer<Item>
}

// Re-export the step def type so consumers can author type-safe
// configurations without spelunking into the schema file.
export type { UIWizardStepDef }
