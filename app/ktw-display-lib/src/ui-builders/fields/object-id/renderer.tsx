// SPDX-License-Identifier: Apache-2.0
// object-id content renderer — An object's id as a mono chip; a click opens the object through the host.
// View and column draw the same mark (../_planning/marks.tsx), in the
// planning palette (../_planning/palette.ts).

import ObjectRefRenderer from '../object-ref/renderer.tsx'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import { IdChip } from '../_planning/marks.tsx'
import type { ObjectIdContent } from './types.ts'

const view: ViewFieldRendererFactory<any, ObjectIdContent> = ({ info }) => {
  return (item: JsonObject) => <IdChip value={item?.[info.name]} testId={'view.' + info.name} />
}

// The form picks the object as an object-ref does.
const form: FormFieldRendererFactory<any, ObjectIdContent> = (p) => ObjectRefRenderer.form(p as never)

const column: ColumnRendererFactory<any, ObjectIdContent> = ({ info }) => {
  return (item: JsonObject) => <IdChip value={item?.[info.name]} testId={'column.' + info.name} />
}

const ObjectIdRenderer: FieldRendererFactory<any, ObjectIdContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default ObjectIdRenderer
