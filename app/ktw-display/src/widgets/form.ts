// SPDX-License-Identifier: Apache-2.0
// `/widget/form` route handler — production form widget.
//
// Renders a host-injected schema (base64url `?schema=<...>`, optional
// `?value=<...>`) through the `buildUIForms()` + `<UIFormView>` pipeline.
// The runtime auto-detects the `ui-form` extension on the root type and
// dispatches to `<UIFormHarness>` — the route handler just emits the HTML
// shell; the schema drives harness selection.
//
// Multi-form: a schema may declare several `ui-form` extensions; the
// iframe URL's `?form=<name>` query param picks between them at render
// time (default `'default'`). `?mode=` selects create vs edit.
//
// No schema injected → a skeleton placeholder (the host remounts with the
// real schema once it anchors). The route NEVER fabricates demo data —
// demo schemas live host-side in the dev demo pages, injected like any
// other host schema.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { schemaFromURL, valueFromURL } from './_lib/schema-from-url.ts'

/** /widget/form handler. Reads `?mode=` and `?form=` from the URL — the
 *  bun route only forwards the request through; the runtime React tree
 *  reads `window.location.search` directly to pick the form variant. */
export function formWidget(req: Request): Response {
  const url = new URL(req.url)
  const mode = url.searchParams.get('mode') ?? 'edit'

  const schema = schemaFromURL(req)
  if (!schema) {
    return renderLoadingResponse('form')
  }
  const value = valueFromURL(req)

  const html = renderSchemaForm({
    schema,
    value,
    pageTitle: 'Form widget',
    description: 'Host-injected schema rendered through the buildUIForm() pipeline.',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'form',
    mode,
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
