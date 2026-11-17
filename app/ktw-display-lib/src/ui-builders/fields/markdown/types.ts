// SPDX-License-Identifier: Apache-2.0
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface MarkdownContent extends ContentType {
  type: 'markdown'
  /** Max character count for the editor. */
  maxLength?: number
}
