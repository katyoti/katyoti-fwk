// SPDX-License-Identifier: Apache-2.0
// tree content renderer — an array-of-nested-objects FIELD renders as a
// tree-table (treegrid) inside a ui-view. Opt-in via `content: { type: 'tree',
// extension? }` on an array field, where `extension` is a named `ui-tree-table`
// on the item type, an inline spec, or absent (synthesized default).

import { buildUITreeTable, UITreeTableView, type UITreeTable } from '../../tree-table/index.ts'
import { makeArrayFieldRenderer } from '../_array-display.tsx'
import type { JsonObject } from '../../../types/json-types.ts'

const TreeRenderer = makeArrayFieldRenderer({
  extType: 'ui-tree-table',
  build: ({ schema, texts, data, ref }) => buildUITreeTable({ schema, texts, data, treeRef: ref }),
  render: (built, rows) => <UITreeTableView tree={built as UITreeTable} data={rows as JsonObject[]} />,
})

export default TreeRenderer
