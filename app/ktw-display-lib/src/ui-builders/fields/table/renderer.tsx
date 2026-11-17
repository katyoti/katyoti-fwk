// SPDX-License-Identifier: Apache-2.0
// table content renderer — an array-of-objects FIELD renders as a flat table
// (ui-table) inside a ui-view, and is edited in a form as a table whose rows
// are edited in place (editor.tsx). Opt-in via `content: { type: 'table', extension? }`
// on an array field, where `extension` is a named `ui-table` on the item type,
// an inline spec, or absent (synthesized default — all fields as columns).

import { buildUITable, UITableView, type UITable } from '../../table/index.ts'
import { makeArrayFieldRenderer } from '../_array-display.tsx'
import { tableForm } from './editor.tsx'
import type { JsonObject } from '../../../types/json-types.ts'

const TableRenderer = {
  ...makeArrayFieldRenderer({
    extType: 'ui-table',
    build: ({ schema, texts, data, ref }) => buildUITable({ schema, texts, data, tableRef: ref }),
    render: (built, rows) => <UITableView table={built as UITable} data={rows as JsonObject[]} />,
  }),
  form: tableForm,
}

export default TableRenderer
