// SPDX-License-Identifier: Apache-2.0
// stepper content renderer — Steps [{name, state}] (done, current or running, todo) as joined pills - done green, current filled, the rest grey.
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
import { Stepper } from '../_planning/marks.tsx'
import type { StepperContent } from './types.ts'

const view: ViewFieldRendererFactory<any, StepperContent> = ({ info }) => {
  return (item: JsonObject) => <Stepper value={item?.[info.name]} testId={'view.' + info.name} />
}

// Computed by its producer: the form shows it, read-only.
const form: FormFieldRendererFactory<any, StepperContent> = ({ info }) => {
  return (item) => (
    <Input.Wrapper label={info.label} description={info.description}>
      <div><Stepper value={(item as JsonObject | undefined)?.[info.name]} testId={'input.' + info.name} /></div>
    </Input.Wrapper>
  )
}

const column: ColumnRendererFactory<any, StepperContent> = ({ info }) => {
  return (item: JsonObject) => <Stepper value={item?.[info.name]} testId={'column.' + info.name} />
}

const StepperRenderer: FieldRendererFactory<any, StepperContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default StepperRenderer
