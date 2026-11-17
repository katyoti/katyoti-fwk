// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface NumberContent extends ContentType {
  type: 'number'
  /** Inclusive minimum. */
  min?: number
  /** Inclusive maximum. */
  max?: number
  /** Step increment. */
  step?: number
  /** Display decimal places. */
  decimals?: number
  /** Use a thousand separator in display. */
  thousandSeparator?: boolean
}
