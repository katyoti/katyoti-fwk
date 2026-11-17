// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface SliderContent extends ContentType {
  type: 'slider'
  /** Inclusive minimum (default 0). */
  min?: number
  /** Inclusive maximum (default 100). */
  max?: number
  /** Step increment (default 1). */
  step?: number
  /** Show tick marks at major positions. */
  showMarks?: boolean
}
