// SPDX-License-Identifier: Apache-2.0
// progress-segments content renderer — Five counts [done, running, waiting, todo, blocked] as one segmented bar - its tooltip the counts.
// View and column draw the same mark (../_planning/marks.tsx), in the
// planning palette (../_planning/palette.ts).

import { Input } from '@mantine/core'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import { SegmentBar } from '../_planning/marks.tsx'
import type { ProgressSegmentsContent } from './types.ts'

const view: ViewFieldRendererFactory<any, ProgressSegmentsContent> = ({ info }) => {
  return (item: JsonObject) => <SegmentBar value={item?.[info.name]} testId={'view.' + info.name} />
}

// Computed by its producer: the form shows it, read-only.
const form: FormFieldRendererFactory<any, ProgressSegmentsContent> = ({ info }) => {
  return (item) => (
    <Input.Wrapper label={info.label} description={info.description}>
      <div><SegmentBar value={(item as JsonObject | undefined)?.[info.name]} testId={'input.' + info.name} /></div>
    </Input.Wrapper>
  )
}

const column: ColumnRendererFactory<any, ProgressSegmentsContent> = ({ info }) => {
  return (item: JsonObject) => <SegmentBar value={item?.[info.name]} testId={'column.' + info.name} />
}

const ProgressSegmentsRenderer: FieldRendererFactory<any, ProgressSegmentsContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default ProgressSegmentsRenderer
