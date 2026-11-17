// SPDX-License-Identifier: Apache-2.0
// StageContent — `content: stage`: A release stage (planned, frozen, candidate, canary, released, dropped) as a chip.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface StageContent extends ContentType {
  type: 'stage'
}
