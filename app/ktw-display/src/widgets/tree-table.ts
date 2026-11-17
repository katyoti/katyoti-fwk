// SPDX-License-Identifier: Apache-2.0
// `/widget/tree-table` route handler — production tree-table (treegrid) widget.
//
// Renders a host-injected schema (base64url `?schema=<...>`, optional
// `?value=<...>` array of nodes linked by a parent-id field) through the
// `buildUITreeTables()` + `<UITreeTableView>` pipeline. The runtime auto-detects
// the `ui-tree-table` extension and dispatches accordingly.
//
// No schema injected → the shared loading screen (the host remounts with the
// real schema once it anchors). The route NEVER fabricates demo data.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { schemaFromURL, valueFromURL } from './_lib/schema-from-url.ts'

export function treeTableWidget(req: Request): Response {
  const schema = schemaFromURL(req)
  if (!schema) {
    return renderLoadingResponse('tree-table')
  }
  const value = valueFromURL(req)

  const html = renderSchemaForm({
    schema,
    value,
    pageTitle: 'Tree-table widget',
    description: 'Host-injected schema rendered through the buildUITreeTable() pipeline.',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'tree-table',
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
