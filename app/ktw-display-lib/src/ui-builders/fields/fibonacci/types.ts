// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface FibonacciContent extends ContentType {
  type: 'fibonacci'
  /** Prepend 0 to the scale (default false). */
  allowZero?: boolean
}
