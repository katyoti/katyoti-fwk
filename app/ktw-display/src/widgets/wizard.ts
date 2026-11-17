// SPDX-License-Identifier: Apache-2.0
// `/widget/wizard` route handler — production wizard widget.
//
// Renders a host-injected schema (base64url `?schema=<...>`, optional
// `?value=<...>`) through the `buildUIWizards()` + `<UIWizardView>`
// pipeline. The runtime auto-detects the `ui-wizard` extension on the
// schema's root type and dispatches to `<UIWizardHarness>` accordingly.
//
// Final-step submit emits a `submit` postMessage with the merged
// accumulator value (all steps' fields rolled up).
//
// No schema injected (still streaming) → the shared loading screen; the
// host remounts with the real `?schema=` once it anchors. The route NEVER
// fabricates demo data — demo schemas live host-side in the dev demo pages.

import { renderSchemaForm } from './_lib/render-schema.ts'
import { renderLoadingResponse } from './_lib/render-loading.ts'
import { schemaFromURL, valueFromURL } from './_lib/schema-from-url.ts'

export function wizardWidget(req: Request): Response {
  const schema = schemaFromURL(req)
  if (!schema) {
    return renderLoadingResponse('wizard')
  }
  const value = valueFromURL(req)

  const html = renderSchemaForm({
    schema,
    value,
    pageTitle: 'Wizard widget',
    description: 'Host-injected schema rendered through the buildUIWizard() pipeline.',
    showSchemaPane: false,
    showOutcome: false,
    widgetName: 'wizard',
    mode: 'create',
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
