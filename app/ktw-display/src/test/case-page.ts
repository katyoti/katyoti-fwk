// SPDX-License-Identifier: Apache-2.0
// Per-case page — renders a TestCase's schema using the shared schema-walker.
//
// Mounted in the harness's right-pane iframe at /test/<category>/<case>.
// The harness has its own outcome strip at the page level, so case pages
// pass `showOutcome: false` — interactions still emit postMessage events
// (caught by the harness's window-level listener), they just aren't echoed
// inside the case iframe.

import { renderSchemaForm } from '../widgets/_lib/render-schema.ts'
import type { TestCase } from './types.ts'

/**
 * Render a TestCase as a complete iframe-mountable page.
 */
export function renderCasePage(c: TestCase): Response {
  const html = renderSchemaForm({
    schema: c.schema,
    value: c.value,
    mode: c.mode ?? 'edit',
    pageTitle: c.title,
    description: c.description,
    showSchemaPane: true,
    showOutcome: false, // harness owns the outcome strip
    widgetName: c.id,
  })
  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
