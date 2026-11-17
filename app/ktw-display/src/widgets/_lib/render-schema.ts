// SPDX-License-Identifier: Apache-2.0
// Iframe HTML shell — thin generator that returns a self-contained iframe-mountable
// HTML page wired to the React+Mantine runtime bundle served at /widget/runtime.js.
//
// The runtime bundle (built by src/runtime/build.ts) bootstraps a React tree with
// MantineProvider + I18nextProvider + react-hook-form's FormProvider, reads the
// schema + initial value from `__KTW_RUNTIME_CONFIG__`, walks `root.fields`, and
// dispatches each field through the content-types registry into the right
// FieldRendererFactory. See S5r2 of `unify-content-types-as-folders` for the
// full architecture story.
//
// This file only renders the HTML shell. All form / field / outcome behavior
// lives in the runtime bundle.

export interface RenderOptions {
  schema: unknown
  value?: unknown
  mode?: string
  pageTitle?: string
  description?: string
  /** Show the source-schema pane on the right. Default: true. */
  showSchemaPane?: boolean
  /** Show the per-page outcome panel below the form. Default: true.
   *  Test cases set this to false because the harness has its own outcome strip. */
  showOutcome?: boolean
  /** postMessage `widget` field (for routing on the host side). */
  widgetName?: string
  /** Optional DSL tree — set ONLY by the `/widget/page` route. The
   *  UIPageHarness reads this via `__KTW_RUNTIME_CONFIG__.dslTree`;
   *  other harnesses ignore it. Wired by display-dsl-framework S5. */
  dslTree?: unknown
  /** When true, the harness renders without page chrome (no Card, no
   *  title, no schema pane, no full-viewport background). Set when the
   *  iframe is hosted inside a host-side tab. Wired by display-dsl-
   *  framework S9 polish. */
  embedded?: boolean
  /** Optional outer data tree for `dataField` bindings inside the
   *  dslTree (page kind only). Wired by display-package-restructure
   *  Phase 3 (2026-05-06). */
  data?: unknown
  /** Make the page fill the iframe viewport (html/body/#ktw-root = 100%
   *  height) instead of sizing to content. Set ONLY for fill-the-tab kinds
   *  like drawio (whose viewer is height:100%); leaving it off keeps the
   *  content-height auto-resize behaviour every other widget relies on. */
  fillViewport?: boolean
}

/**
 * Render a complete iframe-mountable HTML page that embeds the React+Mantine
 * runtime bundle and configures it via a global `__KTW_RUNTIME_CONFIG__`.
 */
export function renderSchemaForm(opts: RenderOptions): string {
  const {
    schema,
    value = {},
    mode = 'edit',
    pageTitle = 'Schema-driven form',
    description = '',
    showSchemaPane = true,
    showOutcome = true,
    widgetName = 'schema-form',
    dslTree,
    embedded = false,
    data,
    fillViewport = false,
  } = opts

  const config: Record<string, unknown> = {
    schema,
    initialValue: value,
    mode,
    pageTitle,
    description,
    showSchemaPane,
    showOutcome,
    widgetName,
    embedded,
  }
  // dslTree is only set for /widget/page mounts; omit otherwise so the
  // serialized config stays small.
  if (dslTree !== undefined) {
    config.dslTree = dslTree
  }
  if (data !== undefined) {
    config.data = data
  }

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>${escapeHtml(pageTitle)}</title>
  <link rel="stylesheet" href="/widget/runtime.css" />
  <style>
    /* Hard dark fallback so the iframe never flashes white before React +
       Mantine mount (ktw-gui is dark). Mantine's own body background layers
       on top once the provider applies forceColorScheme="dark". */
    :root { color-scheme: dark; }
    html, body { margin: 0; background: #1a1b1e; }
    ${fillViewport ? 'html, body, #ktw-root { height: 100%; }' : ''}
  </style>
</head>
<body>
  <div id="ktw-root"></div>
  <script>
    window.__KTW_RUNTIME_CONFIG__ = ${JSON.stringify(config)};
  </script>
  <script type="module" src="/widget/runtime.js"></script>
</body>
</html>`
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => {
    switch (c) {
      case '&': return '&amp;'
      case '<': return '&lt;'
      case '>': return '&gt;'
      case '"': return '&quot;'
      default: return '&#39;'
    }
  })
}
