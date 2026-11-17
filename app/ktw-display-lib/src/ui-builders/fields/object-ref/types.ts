// SPDX-License-Identifier: Apache-2.0
// ObjectRefContent — `content: {type: 'object-ref', types, capabilities, …}`:
// a string holding an object's id, its params the ObjectFilter of the
// objects it may name.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface ObjectRefContent extends ContentType {
  type: 'object-ref'
  types?: string[]
  capabilities?: string[]
  incoming?: string[]
  outgoing?: string[]
  when?: string
}
