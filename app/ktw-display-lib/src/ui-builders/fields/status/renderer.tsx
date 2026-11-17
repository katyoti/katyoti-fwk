// SPDX-License-Identifier: Apache-2.0
// status content renderer — A standing or plan state word (done, running, waiting, on-hold, blocked, planned, dropped, …) as a chip in the Workflows view's status colours.
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
import { StatusChip } from '../_planning/marks.tsx'
import type { StatusContent } from './types.ts'

const view: ViewFieldRendererFactory<any, StatusContent> = ({ info }) => {
  return (item: JsonObject) => <StatusChip value={item?.[info.name]} testId={'view.' + info.name} />
}

// The form edits the word as text (a Select when the type declares an enum).
const form: FormFieldRendererFactory<any, StatusContent> = (p) => TextRenderer.form(p as never)

const column: ColumnRendererFactory<any, StatusContent> = ({ info }) => {
  return (item: JsonObject) => <StatusChip value={item?.[info.name]} testId={'column.' + info.name} />
}

const StatusRenderer: FieldRendererFactory<any, StatusContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default StatusRenderer
