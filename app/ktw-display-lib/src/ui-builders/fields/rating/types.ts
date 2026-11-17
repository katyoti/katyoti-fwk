// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface RatingContent extends ContentType {
  type: 'rating'
  /** Maximum stars displayed. Default 5. */
  max?: number
  /** Allow half-star increments. */
  allowHalf?: boolean
}
