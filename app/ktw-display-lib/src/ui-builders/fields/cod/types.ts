// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface CodContent extends ContentType {
  type: 'cod'
  /** Sibling field name for Business Value (default 'business_value'). */
  value?: string
  /** Sibling field name for Time Criticality (default 'time_criticality'). */
  criticality?: string
  /** Sibling field name for Risk / Opportunity (default 'risk_opportunity'). */
  risk?: string
}
