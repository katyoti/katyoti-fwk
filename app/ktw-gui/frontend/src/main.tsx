// SPDX-License-Identifier: Apache-2.0
import '@mantine/core/styles.css'
import '@mantine/dates/styles.css'
import '@mantine/code-highlight/styles.css'
import '@gfazioli/mantine-split-pane/styles.css'
import React from 'react'
import ReactDOM from 'react-dom/client'
import { MantineProvider, createTheme } from '@mantine/core'
import { App } from './App'
import { ErrorBoundary } from './components/ErrorBoundary'
import { installDevCapture } from './sections/dev/installDevCapture'
import { installBusyCount } from './ktsp/busy'
import { initAppI18n } from './i18n'
import { namePanelPage } from './lib/displayPanels'

// Patch console / errors / Wails IPC into the in-app DevPanel buffer before
// anything renders, so the developer console captures the full session.
installDevCapture()

// Every call to the Go side counts in window.__ktwBusy while it is in flight:
// a script driving the page (browse/wait-idle) waits until it is 0.
installBusyCount()

// This page names itself to the GUI at once: a webview reload ends the
// display-panel follows the page before it started.
void namePanelPage()

const theme = createTheme({
  primaryColor: 'blue',
})

// The GUI's language is set up once, before the first render: every text is
// read in the person's language from the first frame (i18n/index.ts). A
// failure leaves the keys' English fallback rather than no window.
const ready = initAppI18n().catch((err) => {
  // eslint-disable-next-line no-console
  console.warn('[i18n] init failed; texts fall back to their keys:', err)
})

void ready.then(() => ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    {/* ErrorBoundary INSIDE MantineProvider so its fallback UI can use Mantine components */}
    <MantineProvider theme={theme} defaultColorScheme="dark">
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </MantineProvider>
  </React.StrictMode>,
))
