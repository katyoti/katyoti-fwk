// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { createFormControl } from 'react-hook-form'

import { formResetOptions } from '../src/ui-builders/form/form-reset.ts'

// A question asked again after a refusal pushes the refused
// answer as the form's new value; a page form keeps what the person typed
// since and takes the rest.

function typedThenPushed(keepEdits: boolean) {
  const { formControl } = createFormControl({ defaultValues: { title: '', summary: '' } })
  formControl.register('title')
  formControl.register('summary')
  formControl.setValue('title', 'Probe document', { shouldDirty: true })
  formControl.reset({ title: '', summary: 'untitled' }, formResetOptions(keepEdits))
  return formControl.getValues()
}

describe('a value pushed into a page form keeps the person\'s edits', () => {
  test('keeping edits: the edited title stays, the summary comes from the new value', () => {
    expect(typedThenPushed(true)).toEqual({ title: 'Probe document', summary: 'untitled' })
  })

  test('without keeping edits the form takes the new value entirely', () => {
    expect(typedThenPushed(false)).toEqual({ title: '', summary: 'untitled' })
  })
})
