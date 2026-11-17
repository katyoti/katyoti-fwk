// SPDX-License-Identifier: Apache-2.0
// AlertContent — `content: alert`: a text that waits on the person (« needs
// you ») drawn as a small red filled chip; an empty value draws nothing.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface AlertContent extends ContentType {
  type: 'alert'
}
