// SPDX-License-Identifier: Apache-2.0
// ProgressSegmentsContent — `content: progress-segments`: Five counts [done, running, waiting, todo, blocked] as one segmented bar - its tooltip the counts.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface ProgressSegmentsContent extends ContentType {
  type: 'progress-segments'
}
