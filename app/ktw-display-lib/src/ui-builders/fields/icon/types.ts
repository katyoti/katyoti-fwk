// SPDX-License-Identifier: Apache-2.0
// IconContent — `content: icon`: the value is a Tabler icon name
// (`external-link`, `IconExternalLink`) drawn as that icon; an empty value
// draws nothing.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface IconContent extends ContentType {
  type: 'icon'
  /** JSON-encoded value→color map (a Mantine color name or a CSS color) for
   *  the icon, looked up by the value of `colorField` (or the icon's own). */
  colorMap?: string
  /** The sibling field whose value picks the color in `colorMap`. */
  colorField?: string
}
