// SPDX-License-Identifier: Apache-2.0
// `/widget/drawio` route handler — drawio document widget.
//
// View-only viewer for `.drawio` (mxGraph XML). The host's `<UIDrawio>`
// typed wrapper base64url-encodes a `{ xml, filename?, format?, page? }`
// value and forwards it via `?value=<...>`. The runtime dispatches
// kind='drawio' from the URL path (see runtime/dispatch.ts) and the
// renderInner mounts `<DrawioView>` — which lazy-loads the vendored
// viewer-static.min.js (Apache 2.0) on first encounter.
//
// No diagram injected yet (still streaming) → the shared loading screen;
// the host swaps in the real `?value=` once the xml anchors. The route
// NEVER fabricates a demo diagram — demo diagrams live host-side in the
// dev demo pages.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { valueFromURL } from './_lib/schema-from-url.ts'

/** /widget/drawio handler. The diagram value is delivered via postMessage
 *  (the host posts `value` once the harness reports `ready`), NOT the URL —
 *  a real .drawio is large and a long `?value=` URL makes the harness never
 *  emit `ready` (blank-white viewer). So we ALWAYS mount the harness (even
 *  with no/empty value): it shows DrawioView's own loading/empty state until
 *  the host pushes the value, then re-renders. A `?value=` is still honoured
 *  as the initial value when present (backward-compat / small diagrams). */
export function drawioWidget(req: Request): Response {
  const injectedValue = valueFromURL(req)

  // The drawio renderInner reads `item` from RenderInnerCtx; the harness
  // populates `item` from `__KTW_RUNTIME_CONFIG__.initialValue`, which
  // renderSchemaForm sources from its `value` option (NOT `initialValue`
  // — the option name on RenderOptions is `value`).
  //
  // The `schema` field is required by HarnessConfig's typing but unused
  // by the drawio renderer — pass a placeholder.
  const html = renderSchemaForm({
    schema: {
      name: 'drawio-placeholder',
      root: 'placeholder',
      types: { placeholder: { type: 'object', fields: {} } },
    },
    value: injectedValue as Record<string, unknown>, // DrawioValue is a flat record
    mode: 'view',
    pageTitle: 'Drawio viewer',
    description: '',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'drawio',
    embedded: true,
    // The viewer is height:100% — let the page fill the (fillHeight) iframe so
    // the diagram uses the whole tab, not just DrawioView's 480px minHeight.
    fillViewport: true,
  })

  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
