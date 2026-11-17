// SPDX-License-Identifier: Apache-2.0
// UIWizardDef — the DSL `ui-wizard` extension shape.
//
// NEW from scratch (gui-action Subtask 4, 2026-05-04). No old-code
// equivalent — `wizard` was a form-LAYOUT in the old code (not a separate
// composition) and didn't carry per-step state or validation.
//
// Schema authors attach via:
//   extensions: [
//     { type: 'ui-wizard', name: 'onboarding',
//       navigation: 'gated',
//       steps: [
//         { id: 'identity', title: 'Who are you?', fields: ['name', 'email'] },
//         { id: 'role', title: 'Pick a role', fields: ['role'] },
//         { id: 'review', title: 'Review', fields: [] },
//       ],
//     }
//   ]

import type { SchemaTypeExtension } from '../../schema/index.ts'
import type { UIPageNode } from '../page/ui-page.types.ts'

/** Per-step declarative shape. A step renders a form built from a subset
 *  of the root schema's fields (named by `fields[]`), or any page node
 *  (`node`). */
export type UIWizardStepDef = {
  /** Stable id used as React key + URL fragment for deep-linking. */
  id: string
  /** Step title — appears in the Stepper indicator + the step's form header. */
  title: string
  /** Optional one-line description shown below the title. */
  description?: string
  /**
   * Field names from the root schema to include in this step's form.
   * An empty array means a "review" step — no editable fields, just a
   * read-only display of the accumulated value. Ignored when `node` is set.
   */
  fields?: string[]
  /**
   * Any page node drawn as the step instead of a form - a picker, a list,
   * a section of them. « Suivant » accepts the step as it
   * stands: the node keeps its own state, the wizard's value is unchanged.
   * Drawn where the wizard sits in a page (the page renders it).
   */
  node?: UIPageNode
  /**
   * When true, the user can navigate back to this step after passing it
   * (linear-mode-only refinement; free mode allows any backward jump).
   * Default: false (linear mode locks past steps).
   */
  editable?: boolean
  /**
   * Optional gate expression name (resolved at runtime by the wizard
   * builder). v1 ignores this — gated mode just disables Next when the
   * step's form has unresolved required fields. Future: arbitrary
   * predicate functions registered by name.
   */
  canAdvance?: string
}

/**
 * UIWizardDef — the schema `ui-wizard` extension. Authors attach via
 * `extensions: [{ type: 'ui-wizard', steps: [...], navigation: ... }]`.
 */
export type UIWizardDef = SchemaTypeExtension<{
  type: 'ui-wizard'

  /** Wizard name in case of multi-wizard. Defaults to `'default'`. */
  name?: string

  /** Navigation mode. Default: `'linear'`. */
  navigation?: 'linear' | 'free' | 'gated'

  /** Ordered step list. Last step is conventionally a review step
   *  (empty `fields[]`); v1 treats it the same as any other step
   *  except the final-step Submit emits the wizard's onComplete. */
  steps: UIWizardStepDef[]

  /** Override for the final-step submit button label. Default: i18n
   *  `buttons.complete` → "Complete". */
  submit_label?: string

  /** Override for the cancel button label. */
  cancel_label?: string
}>

/** UIWizardRef — schema attaches this to ui-wizard extension lookups. */
export type UIWizardRef = string | UIWizardDef
