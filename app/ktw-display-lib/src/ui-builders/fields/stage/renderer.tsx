// SPDX-License-Identifier: Apache-2.0
// stage content renderer — A release stage (planned, frozen, candidate, canary, released, dropped) as a chip.
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
import { StageChip } from '../_planning/marks.tsx'
import type { StageContent } from './types.ts'

const view: ViewFieldRendererFactory<any, StageContent> = ({ info }) => {
  return (item: JsonObject) => <StageChip value={item?.[info.name]} testId={'view.' + info.name} />
}

// The form edits the word as text (a Select when the type declares an enum).
const form: FormFieldRendererFactory<any, StageContent> = (p) => TextRenderer.form(p as never)

const column: ColumnRendererFactory<any, StageContent> = ({ info }) => {
  return (item: JsonObject) => <StageChip value={item?.[info.name]} testId={'column.' + info.name} />
}

const StageRenderer: FieldRendererFactory<any, StageContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default StageRenderer
