// SPDX-License-Identifier: Apache-2.0
// alert content renderer — a text that waits on the person (« needs you ») as a
// small red filled chip; an empty value draws nothing.
// View and column draw the same mark (../_planning/marks.tsx), in the
// planning palette (../_planning/palette.ts).

import TextRenderer from '../text/renderer.tsx'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import { AlertChip } from '../_planning/marks.tsx'
import type { AlertContent } from './types.ts'

const view: ViewFieldRendererFactory<any, AlertContent> = ({ info }) => {
  return (item: JsonObject) => <AlertChip value={item?.[info.name]} testId={'view.' + info.name} />
}

// The form edits the word as text (a Select when the type declares an enum).
const form: FormFieldRendererFactory<any, AlertContent> = (p) => TextRenderer.form(p as never)

const column: ColumnRendererFactory<any, AlertContent> = ({ info }) => {
  return (item: JsonObject) => <AlertChip value={item?.[info.name]} testId={'column.' + info.name} />
}

const AlertRenderer: FieldRendererFactory<any, AlertContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default AlertRenderer
