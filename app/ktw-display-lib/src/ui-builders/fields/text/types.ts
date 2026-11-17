// SPDX-License-Identifier: Apache-2.0
// TextContent — the meta-config interface for `content: { type: 'text', ... }`.
// Authored alongside the renderer so per-folder is self-contained.

import type { ContentType } from '../../../schema/schema-contents.ts'

// `options?: FieldOptions` is inherited from the generic `ContentType` envelope
// — a string field with options renders as a Select with a rich row per option,
// falling back to the bare schema `enum` keys when absent.
export interface TextContent extends ContentType {
  type: 'text'

  /** Min character count (HTML5 minlength). */
  minLength?: number
  /** Max character count (HTML5 maxlength). */
  maxLength?: number

  /** A regex or one of the predefined patterns ('email','url','uuid','ip',...). */
  pattern?: string

  /** Lowercase / uppercase / capitalize / upper-first transformation applied
   *  to displayed values (column / view contexts). */
  transformation?: 'lowercase' | 'uppercase' | 'capitalize' | 'upper-first'
}
