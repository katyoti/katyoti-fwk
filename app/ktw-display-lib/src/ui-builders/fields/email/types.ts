// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface EmailContent extends ContentType {
  type: 'email'
}
