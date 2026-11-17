// SPDX-License-Identifier: Apache-2.0
// PriorityContent — `content: priority`: A priority P0-P3 as a chip - P0 red, P1 orange, P2 and P3 muted.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface PriorityContent extends ContentType {
  type: 'priority'
}
