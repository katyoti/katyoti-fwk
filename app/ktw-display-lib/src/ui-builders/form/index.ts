// SPDX-License-Identifier: Apache-2.0
// Public surface of the form package — DSL extension types, runtime types,
// the `buildUIForm` / `buildUIForms` builders, the `<UIFormView>` React
// component, and the shared layout factories that custom layouts compose.

// DSL extension types (authored on schema definitions)
export type {
  UIFormDef,
  UIFormGroupDef,
  UIFormRef,
} from './ui-form.schema.ts'

// Runtime types (consumed by layouts and the form view)
export type {
  UIForm,
  UIFormButtonsRenderer,
  UIFormFields,
  UIFormGroup,
  UIFormGroups,
  UIFormGroupRenderer,
  UIFormRenderer,
  UIFormRendererParams,
  UIFormToolbarRenderer,
} from './ui-form.types.ts'

// Builders + overrides
export {
  buildUIForm,
  buildUIForms,
  buildFormGroups, // @internal — exposed for visibility-filter unit tests
  toUIForm,
  toUIFormDef,
  type BuildUIFormParams,
  type BuildUIFormsParams,
  type UIFormOverrides,
} from './ui-form.builder.ts'

// React component
export { UIFormView, type UIFormViewProps } from './ui-form.view.tsx'

// Shared layout primitives — exposed so authors writing a 5th custom
// layout under `content-types/form-layouts/<name>/` can compose buttons,
// toolbar and group renderers without re-implementing them.
export {
  FormButton,
  type FormButtonProps,
  FormButtons,
  FormToolbar,
  fol_buttons_layout,
  fol_group_layout,
  fol_toolbar_layout,
} from './layouts/_shared/index.ts'
