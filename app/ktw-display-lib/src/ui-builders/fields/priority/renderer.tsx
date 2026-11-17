// SPDX-License-Identifier: Apache-2.0
// priority content renderer — A priority P0-P3 as a chip - P0 red, P1 orange, P2 and P3 muted.
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
import { PriorityChip } from '../_planning/marks.tsx'
import type { PriorityContent } from './types.ts'

const view: ViewFieldRendererFactory<any, PriorityContent> = ({ info }) => {
  return (item: JsonObject) => <PriorityChip value={item?.[info.name]} testId={'view.' + info.name} />
}

// The form edits the word as text (a Select when the type declares an enum).
const form: FormFieldRendererFactory<any, PriorityContent> = (p) => TextRenderer.form(p as never)

const column: ColumnRendererFactory<any, PriorityContent> = ({ info }) => {
  return (item: JsonObject) => <PriorityChip value={item?.[info.name]} testId={'column.' + info.name} />
}

const PriorityRenderer: FieldRendererFactory<any, PriorityContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default PriorityRenderer
