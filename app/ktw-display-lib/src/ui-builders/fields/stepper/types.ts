// SPDX-License-Identifier: Apache-2.0
// StepperContent — `content: stepper`: Steps [{name, state}] (done, current or running, todo) as joined pills - done green, current filled, the rest grey.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface StepperContent extends ContentType {
  type: 'stepper'
}
