// SPDX-License-Identifier: Apache-2.0
// <DrawioEditor> — full editable drawio mode (gui-drawio-editor / B-Heavy).
//
// Mounts the vendored drawio webapp at
// `${bunBase}/widget/vendor/drawio/editor/index.html?embed=1&proto=json...`
// directly in an iframe, then bridges drawio's documented embed protocol
// (postMessage with `{ event, action, xml, ... }` envelopes) to host
// callbacks `onSave({ xml })` and `onExit()`.
//
// Why direct iframe (not IframeWidget):
//   - Drawio has its own postMessage protocol; our standard ready/error
//     envelope doesn't apply to its iframe.
//   - The editor is a static vendored asset (no token needed; served by
//     /widget/vendor/* which bypasses the auth gate).
//   - Theme bridge isn't useful — drawio has its own dark mode via the
//     `?dark=1` URL knob.
//
// Vendoring: the editor bundle lives at app/ktw-display/vendor/drawio/editor/
// (gitignored, ~50MB). Re-vendor via vendor/drawio/setup-editor.{sh,ps1}.
// See the NOTICE in that directory.

import { useCallback, useEffect, useRef, useState } from 'react'
import { Box, Center, Loader, Stack, Text } from '@mantine/core'
import { GetWidgetURL } from '../../../wailsjs/go/main/App'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

export interface DrawioEditorProps {
  /** Initial XML to load into the editor. */
  xml: string
  /** Universe whose bun server hosts the vendored editor. */
  universe?: string
  /** Fired when the user clicks Save in the drawio toolbar.
   *  Receives the current XML — caller persists / propagates. */
  onSave?: (xml: string) => void
  /** Fired when the user closes the editor (Exit / File / Close). */
  onExit?: () => void
  /** When set, overrides the host-theme-driven dark mode. Leave unset to
   *  follow the host's `appSettings.appearance.themeMode` (recommended). */
  dark?: boolean
}

/** Drawio embed-protocol message shapes (subset we actually handle). */
interface DrawioMessageInit { event: 'init' }
interface DrawioMessageSave { event: 'save'; xml: string }
interface DrawioMessageExit { event: 'exit' }
interface DrawioMessageAutosave { event: 'autosave'; xml: string }
type DrawioInboundMessage =
  | DrawioMessageInit
  | DrawioMessageSave
  | DrawioMessageExit
  | DrawioMessageAutosave
  | { event: string; [k: string]: unknown }

/** Build the editor URL from a representative widget URL.
 *  Strategy: ask for any kind ('drawio' is fine), then rewrite the path —
 *  same host:port, different path. Keeps token-handling unchanged for
 *  whatever future routes need it (the vendor path itself is unauthenticated). */
function deriveEditorURL(widgetURL: string, dark: boolean): string | null {
  try {
    const u = new URL(widgetURL)
    u.pathname = '/widget/vendor/drawio/editor/index.html'
    // Drawio embed protocol params:
    //   embed=1     → embed mode (no top toolbar / scratchpad / start screen)
    //   proto=json  → JSON envelope on postMessage (vs the legacy text)
    //   spin=1      → show drawio's own loading spinner
    //   ui=atlas    → modern UI theme
    //   noSaveBtn=0 → keep Save button visible (user clicks → triggers save event)
    //   saveAndExit=1 → clicking Save also exits the editor
    //   dark=1|0    → dark mode toggle
    //   modified=unsavedChanges → show unsaved indicator
    u.searchParams.set('embed', '1')
    u.searchParams.set('proto', 'json')
    u.searchParams.set('spin', '1')
    u.searchParams.set('ui', 'atlas')
    u.searchParams.set('saveAndExit', '0')
    u.searchParams.set('dark', dark ? '1' : '0')
    // configure=1 → drawio fires a `configure` event on load and waits for the
    // parent to post a config envelope before init. We use it to inject CSS
    // that forces a WHITE canvas backdrop (see the `configure` message handler).
    // `dark=0` only themes the chrome; the area outside the page (the
    // geDiagramContainer) still paints a grey/dark backdrop on its own.
    u.searchParams.set('configure', '1')
    return u.toString()
  } catch {
    return null
  }
}

export function DrawioEditor({ xml, universe: universeProp, onSave, onExit, dark }: DrawioEditorProps) {
  // Mirror IframeWidget's pattern: prefer an explicit prop, fall back to
  // the displayed universe in the store. Without this fallback, callers
  // that don't pass `universe` (DocumentTab → UIComponent → UIDrawio)
  // see "no displayed universe" even when one is active.
  const displayedUniverse = useAppStore((s) => s.displayedUniverseName)
  const universe = universeProp ?? displayedUniverse

  // Dark-mode resolution: an explicit `dark` prop wins; otherwise default to
  // LIGHT. Diagrams are authored on a WHITE canvas — DrawioView hardcodes a
  // white surface for the same reason (shape fills + edge labels are designed
  // against white and become invisible on a dark canvas). We do NOT follow the
  // host's dark theme here so the editor canvas matches the view.
  const isDark = typeof dark === 'boolean' ? dark : false

  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const t = useT('components')
  const [editorURL, setEditorURL] = useState<string | null>(null)
  const [resolveError, setResolveError] = useState<string | null>(null)
  const xmlRef = useRef(xml)
  // Keep ref in sync — the load is sent on init (one-shot), not on every
  // xml change. Re-mounting the editor with new xml is the way to swap.
  useEffect(() => { xmlRef.current = xml }, [xml])

  // Resolve the bun base URL (via GetWidgetURL with a sentinel kind),
  // then rewrite the path to point at the vendored editor.
  useEffect(() => {
    let cancelled = false
    if (!universe) {
      setResolveError('no displayed universe')
      return () => { cancelled = true }
    }
    GetWidgetURL(universe, 'drawio', '', '', '')
      .then((url) => {
        if (cancelled) return
        const editor = deriveEditorURL(url, isDark)
        if (!editor) {
          setResolveError('failed to derive editor URL from widget URL')
          return
        }
        setEditorURL(editor)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setResolveError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => { cancelled = true }
  }, [universe, isDark])

  const handleMessage = useCallback((evt: MessageEvent) => {
    if (!iframeRef.current || evt.source !== iframeRef.current.contentWindow) {
      return
    }
    let msg: DrawioInboundMessage
    try {
      msg = typeof evt.data === 'string' ? JSON.parse(evt.data) : (evt.data as DrawioInboundMessage)
    } catch {
      return // not a JSON envelope; ignore
    }
    if (!msg || typeof msg.event !== 'string') return

    switch (msg.event) {
      case 'configure': {
        // Respond to drawio's configure handshake (URL `configure=1`). Inject
        // CSS to force a WHITE editor surface — matching DrawioView's white
        // canvas convention (diagram shape fills + edge labels are designed
        // against white). drawio's atlas theme paints the area OUTSIDE the page
        // (the geDiagramContainer) a grey/dark backdrop; override it to white.
        // `enableCssDarkMode:false` stops CSS dark mode re-darkening it.
        iframeRef.current.contentWindow?.postMessage(
          JSON.stringify({
            action: 'configure',
            config: {
              enableCssDarkMode: false,
              css: [
                '.geDiagramContainer{background-color:#ffffff !important;}',
                '.geEditor,.geBackgroundContainer{background-color:#ffffff !important;}',
                'body{background-color:#ffffff !important;}',
              ].join(''),
            },
          }),
          '*',
        )
        return
      }
      case 'init': {
        // Drawio is ready — push the initial XML via the `load` action.
        iframeRef.current.contentWindow?.postMessage(
          JSON.stringify({ action: 'load', xml: xmlRef.current, autosave: 0 }),
          '*',
        )
        return
      }
      case 'save': {
        const saveMsg = msg as DrawioMessageSave
        if (typeof saveMsg.xml === 'string') onSave?.(saveMsg.xml)
        return
      }
      case 'exit': {
        onExit?.()
        return
      }
      case 'autosave': {
        // We pushed autosave=0 in `load`, so this should rarely fire —
        // but if drawio sends one anyway (safety), treat it like a save.
        const auto = msg as DrawioMessageAutosave
        if (typeof auto.xml === 'string') onSave?.(auto.xml)
        return
      }
      default:
        return
    }
  }, [onSave, onExit])

  useEffect(() => {
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [handleMessage])

  if (resolveError) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap="xs" p="md">
          <Text size="sm" c="red">{t('drawio.loadFailed')}</Text>
          <Text size="xs" c="dimmed">{resolveError}</Text>
        </Stack>
      </Center>
    )
  }

  if (!editorURL) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap="xs">
          <Loader size="sm" />
          <Text size="xs" c="dimmed">{t('drawio.loading')}</Text>
        </Stack>
      </Center>
    )
  }

  return (
    <Box style={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
      <iframe
        ref={iframeRef}
        src={editorURL}
        title={t('drawio.title')}
        // sandbox includes: scripts, same-origin (postMessage), forms,
        // popups (drawio sometimes opens link dialogs), modals.
        // Same-origin OK because vendor assets are served by our own bun.
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        style={{
          flex: 1,
          width: '100%',
          minHeight: 0,
          border: 'none',
          // White letterbox (the editor canvas is forced white via the
          // configure handshake) so the area around the iframe matches.
          background: '#ffffff',
        }}
      />
    </Box>
  )
}
