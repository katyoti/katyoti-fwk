// SPDX-License-Identifier: Apache-2.0
// `/widget/table` route handler — production table widget.
//
// Renders a host-injected schema (base64url `?schema=<...>`, optional
// `?value=<...>` array) through the `buildUITables()` + `<UITableView>`
// pipeline. The runtime auto-detects the `ui-table` extension and
// dispatches to `<UITableHarness>` accordingly.
//
// A schema may declare both a `ui-table` (column config + filter
// integration) and a `ui-filter` (search + options + tabs) extension.
// `?table=<name>` swaps to a non-default table variant; `?filter=<name>`
// swaps to a non-default filter set (when multiple are declared).
//
// No schema injected → the shared "assistant is working" loading screen
// (the host remounts with the real schema once it anchors). The route
// NEVER fabricates demo data — demo schemas + rows live host-side in the
// dev demo pages.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { schemaFromURL, valueFromURL } from './_lib/schema-from-url.ts'

export function tableWidget(req: Request): Response {
  const schema = schemaFromURL(req)
  if (!schema) {
    return renderLoadingResponse('table')
  }
  const value = valueFromURL(req)

  const html = renderSchemaForm({
    schema,
    value,
    pageTitle: 'Table widget',
    description: 'Host-injected schema rendered through the buildUITable() pipeline.',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'table',
    mode: 'view',
    embedded: true,
  })

  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
