// SPDX-License-Identifier: Apache-2.0
// `/widget/view` route handler — production view widget.
//
// Renders a host-injected schema (base64url `?schema=<...>`, optional
// `?value=<...>`) through the `buildUIViews()` + `<UIViewView>` pipeline.
// The runtime's `dispatchHarness()` reads `/widget/view` from the URL and
// picks `<UIViewHarness>` accordingly — the route handler just emits the
// HTML shell.
//
// Multi-view: a schema may declare several `ui-view` extensions; the
// iframe URL's `?view=<name>` query param picks between them at render
// time. `?empty=<strategy>` selects the empty-value rendering strategy.
//
// No schema injected → the shared "assistant is working" loading screen
// (the host remounts with the real schema once it anchors). The route
// NEVER fabricates demo data — demo schemas live host-side in the dev
// demo pages.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { schemaFromURL, valueFromURL } from './_lib/schema-from-url.ts'

/** /widget/view handler. Reads `?view=` and `?empty=` from the URL —
 *  the runtime React tree picks them up directly via `window.location.search`. */
export function viewWidget(req: Request): Response {
  const url = new URL(req.url)
  const mode = url.searchParams.get('mode') ?? 'view'

  const schema = schemaFromURL(req)
  if (!schema) {
    return renderLoadingResponse('view')
  }
  const value = valueFromURL(req)

  const html = renderSchemaForm({
    schema,
    value,
    pageTitle: 'View widget',
    description: 'Host-injected schema rendered through the buildUIView() pipeline.',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'view',
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
