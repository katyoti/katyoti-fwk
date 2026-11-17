// SPDX-License-Identifier: Apache-2.0
// react-hook-form re-exports + RHFFormProvider wrapper.
//
// Ported from _ktw_workspace/old_ui_main/src/modules/core/cpm-core/forms/utils.tsx
// during gui-form Subtask 1 (2026-05-03). Lives under src/ui-builders/form/_shared/
// because it's exclusively used by the form rendering surface — keeping it
// adjacent to its consumer instead of in a generic shared layer.
//
// The Form provider wraps a <form> element around react-hook-form's
// FormProvider context — every <Controller> child gets the form methods
// via context. onSubmit is wrapped through methods.handleSubmit() so the
// callback receives validated values typed against the form's resolver.

import type { ReactNode } from 'react'
import {
  FormProvider as Form,
  type SubmitHandler,
  type UseFormReturn,
} from 'react-hook-form'

export {
  Controller,
  useFieldArray,
  useForm,
  useFormContext,
  type UseFormReturn,
} from 'react-hook-form'

interface RHFFormProviderProps {
  children: ReactNode
  methods: UseFormReturn<any>
  onSubmit?: SubmitHandler<any>
  /** Extra attributes on the <form> element. */
  attrs?: Record<string, string>
}

/**
 * RHFFormProvider — wraps children in a react-hook-form FormProvider PLUS
 * a native <form> element. Pairs methods.handleSubmit() with the optional
 * onSubmit callback so values are pre-validated before reaching the handler.
 */
export function RHFFormProvider({ children, onSubmit, methods, attrs }: RHFFormProviderProps) {
  const formProps = onSubmit ? { onSubmit: methods.handleSubmit(onSubmit) } : {}
  return (
    <Form {...methods}>
      <form {...attrs} {...formProps}>{children}</form>
    </Form>
  )
}
