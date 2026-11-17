// SPDX-License-Identifier: Apache-2.0
// `/widget/page` route handler — DSL-tree page widget.
//
// Renders any tree-shaped UI artifact described by the JSON DSL
// (UIPageNode discriminated union — see app/ktw-display/src/ui-builders/page/).
// Mounted by the host's `<UIPage>` typed wrapper when the LLM emits a
// `display-ui-component` tool call with `kind: "page"` + `dslTree`.
//
// Schema-bound nodes inside the tree (form/view/table/wizard) render
// INLINE inside this single iframe per
// the display module's decision on where the UI lives (modules/display/decisions/) — no nested iframes.
//
// URL contract:
//   - `?page=<base64url-encoded JSON DSL tree>` — the LLM-emit path.
//     The host base64url-encodes the artifact's `dslTree` and forwards.
//   - `?widgetId=<id>` — host-allocated, used for postMessage routing.
//   - `?token=<...>` — per-session auth (handled upstream in serve.ts).
//
// Wired by S5 of display-dsl-framework.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { pageFromURL, dataFromURL } from './_lib/schema-from-url.ts'

/** /widget/page handler. Reads `?page=<base64>` for the DSL tree and
 *  `?data=<base64>` for the outer data tree (Phase 3 dataField bindings).
 *  With no tree injected (still streaming) it renders the shared loading
 *  screen; the host swaps in the real `?page=<...>` once the tree anchors. */
export function pageWidget(req: Request): Response {
  const tree = pageFromURL(req)
  if (!tree) {
    return renderLoadingResponse('page')
  }

  const injectedData = dataFromURL(req) // outer data for dataField bindings (Phase 3)

  // The page harness reads `dslTree` + `data` from `__KTW_RUNTIME_CONFIG__`.
  // The `schema` field is required by HarnessConfig's typing but unused
  // by the page harness — pass a placeholder.
  const config = {
    // Placeholder schema (never read by the page renderer, but the config
    // type requires it for shape-compatibility with other harnesses).
    schema: { name: 'page-placeholder', root: 'placeholder', types: { placeholder: { type: 'object', fields: {} } } },
    initialValue: {},
    mode: 'view',
    dslTree: tree,
    data: injectedData,
    pageTitle: '', // host's tab UI provides the title
    description: '',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'page',
    embedded: true, // host's tab UI provides the container chrome
  }

  // renderSchemaForm is the iframe HTML shell — it serves the runtime
  // bundle and passes the config via __KTW_RUNTIME_CONFIG__. The
  // dispatcher in runtime/index.tsx picks UIPageHarness based on the
  // URL path (/widget/page → kind="page" → UIPageHarness).
  const html = renderSchemaForm({
    ...config,
    pageTitle: config.pageTitle || 'DSL page widget',
  })

  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
