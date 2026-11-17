// SPDX-License-Identifier: Apache-2.0
// gate-dots content renderer — Gates [{name, passed?, why?}] as dots - passed green, failed red, unknown grey - each with « name: why » on hover.
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
import { GateDots } from '../_planning/marks.tsx'
import type { GateDotsContent } from './types.ts'

const view: ViewFieldRendererFactory<any, GateDotsContent> = ({ info }) => {
  return (item: JsonObject) => <GateDots value={item?.[info.name]} labels={info.contentType.labels === true} testId={'view.' + info.name} />
}

// Computed by its producer: the form shows it, read-only.
const form: FormFieldRendererFactory<any, GateDotsContent> = ({ info }) => {
  return (item) => (
    <Input.Wrapper label={info.label} description={info.description}>
      <div><GateDots value={(item as JsonObject | undefined)?.[info.name]} testId={'input.' + info.name} /></div>
    </Input.Wrapper>
  )
}

const column: ColumnRendererFactory<any, GateDotsContent> = ({ info }) => {
  return (item: JsonObject) => <GateDots value={item?.[info.name]} labels={info.contentType.labels === true} testId={'column.' + info.name} />
}

const GateDotsRenderer: FieldRendererFactory<any, GateDotsContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default GateDotsRenderer
