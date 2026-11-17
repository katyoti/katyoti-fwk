// SPDX-License-Identifier: Apache-2.0
// GateDotsContent — `content: gate-dots`: gates [{name, passed?, why?}] as dots;
// `gate-dots(labels)` draws each gate « ● Label (reason) » inline.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface GateDotsContent extends ContentType {
  type: 'gate-dots'
  /** Each gate with its label beside its dot, its reason in brackets while
   *  it does not pass. */
  labels?: boolean
}
