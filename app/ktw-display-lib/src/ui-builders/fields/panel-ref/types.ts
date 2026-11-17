// SPDX-License-Identifier: Apache-2.0
// PanelRefContent — `content: {type: 'panel-ref', types, …}`: a string
// holding `<id>#<panel>`, its params the ObjectFilter of the objects whose
// panels it may name.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface PanelRefContent extends ContentType {
  type: 'panel-ref'
  types?: string[]
  capabilities?: string[]
  incoming?: string[]
  outgoing?: string[]
  when?: string
}
