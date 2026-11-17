// SPDX-License-Identifier: Apache-2.0
// formResetOptions — how a form takes a new value pushed from outside:
// a page form keeps every field the person changed since
// (react-hook-form's keepDirtyValues) and takes the rest from the new value,
// so a question asked again with its refused answer never wipes what the
// person typed meanwhile. A form that keeps no edits resets entirely.

import type { KeepStateOptions } from 'react-hook-form'

export function formResetOptions(keepEdits: boolean): KeepStateOptions | undefined {
  return keepEdits ? { keepDirtyValues: true } : undefined
}
