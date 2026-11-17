// SPDX-License-Identifier: Apache-2.0
// Iframe runtime entry — bundled by Bun.build into /widget/runtime.js.
//
// Bootstraps the React tree:
//   1. Reads window.__KTW_RUNTIME_CONFIG__ (set by the HTML shell)
//   2. Initializes i18next (see ../ui/i18n)
//   3. Wires the static renderer registry (see ./registry-wire)
//   4. Mounts <MantineProvider><Harness/></MantineProvider> into #ktw-root
//
// All Mantine CSS imports are extracted by Bun.build into a separate CSS
// asset, served at /widget/runtime.css (see ./build.ts + ./serve.ts).

// Core MUST be first; the per-package styles layer on top of it.
import '@mantine/core/styles.css'
import '@mantine/dates/styles.css'   // date picker / calendar — without this the date field renders unstyled
import '@mantine/charts/styles.css'

import { Component, type ErrorInfo, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { initI18n } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { Harness, type HarnessConfig } from './harness.tsx'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import { pickViewKind } from './dispatch.ts'
import { HostThemeBridge } from './host-theme-bridge.tsx'
import { UniversalHarness } from './universal-harness.tsx'
import { renderByKind } from './render-inner.tsx'

// Top-level error boundary. Without this, a render-time throw inside the
// iframe just unmounts the React tree silently → blank screen. With this,
// the error + stack are painted into the iframe so the user (and DevTools)
// can see exactly what failed. Painted via direct DOM mutation so we never
// re-enter the failing React render path.
class RuntimeErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }
  static getDerivedStateFromError(error: Error) { return { error } }
  componentDidCatch(error: Error, info: ErrorInfo) {
    // eslint-disable-next-line no-console
    console.error('[ktw-display runtime] render error', error, info.componentStack)
    const root = document.getElementById('ktw-root')
    const pre = document.createElement('pre')
    pre.style.cssText = 'padding:24px;color:#c00;font:12px/1.4 monospace;white-space:pre-wrap;overflow:auto;max-height:100vh'
    pre.textContent =
      `[ktw-display runtime] render error: ${error.message}\n\n` +
      `Stack:\n${error.stack ?? '(no stack)'}\n\n` +
      `Component stack:${info.componentStack ?? '(none)'}`
    if (root) { root.innerHTML = ''; root.appendChild(pre) }
    else document.body.appendChild(pre)
  }
  render() { return this.state.error ? null : this.props.children }
}

declare global {
  interface Window {
    __KTW_RUNTIME_CONFIG__?: HarnessConfig
  }
}

async function bootstrap(): Promise<void> {
  const config = window.__KTW_RUNTIME_CONFIG__
  if (!config) {
    document.body.innerHTML =
      '<pre style="padding:24px;color:#c00;font:14px/1.4 monospace">' +
      'ktw-display runtime: __KTW_RUNTIME_CONFIG__ is not set. Did the HTML shell forget the configuration script tag?' +
      '</pre>'
    return
  }

  await initI18n({ language: 'en' })
  wireRegistries()

  const root = document.getElementById('ktw-root')
  if (!root) {
    document.body.innerHTML =
      '<pre style="padding:24px;color:#c00;font:14px/1.4 monospace">' +
      'ktw-display runtime: #ktw-root not found in the DOM.' +
      '</pre>'
    return
  }

  // Dispatch: pick the view kind based on URL kind hint (`/widget/<kind>`)
  // and/or schema-extension auto-detect. See `dispatch.ts` for precedence.
  const kind = pickViewKind(config.schema)

  // For typed kinds, mount <UniversalHarness> with the matching renderInner
  // function. For 'proto' (no extension found), fall back to the legacy
  // <Harness> proto-walker (bare-field test cases like string-basic).
  const harnessNode =
    kind === 'proto' ? (
      <Harness config={config} />
    ) : (
      <UniversalHarness config={config} kind={kind} renderInner={renderByKind[kind]} />
    )

  // HostThemeBridge subscribes to the host's `theme` postMessage and
  // applies received colorScheme + theme tokens. When the iframe is
  // embedded as a host pane (config.embedded, or URL ?display=ui|schema),
  // it also applies a compact density scale (smaller fonts / headings /
  // spacing) so the iframe visually matches ktw-gui's denser chrome.
  // Without this, Mantine's default heading sizes look oversized vs.
  // the host's small-text UI.
  const urlParams = new URLSearchParams(window.location.search)
  const displayParam = urlParams.get('display')
  const isHostEmbedded = !!config.embedded || displayParam === 'ui' || displayParam === 'schema'

  // Default to DARK so widgets never flash/stick on a white background when
  // the host's `theme` postMessage is slow or missing (ktw-gui is dark). The
  // host bridge still switches to whatever the host later pushes.
  createRoot(root).render(
    <RuntimeErrorBoundary>
      <HostThemeBridge compact={isHostEmbedded} initialColorScheme="dark">{harnessNode}</HostThemeBridge>
    </RuntimeErrorBoundary>,
  )
}

// Catch errors that escape outside React's lifecycle (raw module evaluation,
// async errors during bootstrap that bypass the bootstrap().catch). Same
// rendering style as the boundary.
function paintGlobalError(label: string, err: unknown) {
  // eslint-disable-next-line no-console
  console.error(`[ktw-display runtime] ${label}`, err)
  const root = document.getElementById('ktw-root') ?? document.body
  const pre = document.createElement('pre')
  pre.style.cssText = 'padding:24px;color:#c00;font:12px/1.4 monospace;white-space:pre-wrap;overflow:auto;max-height:100vh'
  const e = err as Error
  pre.textContent = `[ktw-display runtime] ${label}: ${e?.message ?? String(err)}\n\n${e?.stack ?? ''}`
  root.appendChild(pre)
}
window.addEventListener('error', (ev) => paintGlobalError('window error', ev.error ?? ev.message))
window.addEventListener('unhandledrejection', (ev) => paintGlobalError('unhandledrejection', ev.reason))

bootstrap().catch((err) => {
  console.error('[ktw-display runtime] bootstrap failed', err)
  document.body.innerHTML =
    '<pre style="padding:24px;color:#c00;font:14px/1.4 monospace">' +
    'ktw-display runtime: ' +
    String((err as Error).message) +
    '</pre>'
})
