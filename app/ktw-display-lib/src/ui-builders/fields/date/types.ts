// SPDX-License-Identifier: Apache-2.0
// DateContent — the meta-config interface for `content: { type: 'date', ... }`.
// Authored alongside the renderer so per-folder is self-contained.

import type { ContentType } from '../../../schema/schema-contents.ts'

/** ISO 8601 sub-format the value carries. Default: `date` (YYYY-MM-DD). */
export type DatePattern = 'date' | 'date-time' | 'time'

export interface DateContent extends ContentType {
  type: 'date'

  /** ISO 8601 sub-format: date (default) / date-time / time.
   *  Drives which Mantine date input is mounted in the form context. */
  pattern?: DatePattern

  /** Earliest ISO date the input accepts. */
  min?: string
  /** Latest ISO date the input accepts. */
  max?: string

  /** Locale-friendly value-format string for display in view / column
   *  contexts. When unset, the renderer renders the raw ISO string. */
  valueFormat?: string
}
