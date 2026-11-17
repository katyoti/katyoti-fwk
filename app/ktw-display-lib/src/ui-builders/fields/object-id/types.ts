// SPDX-License-Identifier: Apache-2.0
// ObjectIdContent — `content: object-id`: An object's id as a mono chip; a click opens the object through the host.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface ObjectIdContent extends ContentType {
  type: 'object-id'
}
