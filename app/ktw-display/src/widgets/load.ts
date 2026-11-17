// SPDX-License-Identifier: Apache-2.0
// `/widget/load` route handler — the standalone loading-screen widget.
//
// Renders the shared "assistant is working" loading screen on its own,
// independent of any content kind. Mount `<IframeWidget kind="load" />`
// (or open `/widget/load` directly) wherever a placeholder loader is
// wanted — a tab whose content hasn't been requested yet, a long-running
// action's holding screen, or a smoke test of the loader itself.

import { renderLoadingResponse } from './_lib/render-loading.ts'

export function loadWidget(_req: Request): Response {
  return renderLoadingResponse('load', {
    title: 'KaTyoti is working',
    subtitle: 'Hang tight — your content appears here live as it arrives.',
  })
}
