// SPDX-License-Identifier: Apache-2.0
// StatusContent — `content: status`: A standing or plan state word (done, running, waiting, on-hold, blocked, planned, dropped, …) as a chip in the Workflows view's status colours.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface StatusContent extends ContentType {
  type: 'status'
}
