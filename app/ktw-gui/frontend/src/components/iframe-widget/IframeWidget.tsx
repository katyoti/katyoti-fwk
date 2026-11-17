// SPDX-License-Identifier: Apache-2.0
// <IframeWidget> — page-level iframe primitive for ktw-display-served widgets.
//
// One iframe per top-level composition (form / view / table / wizard / deck /
// dashboard / artifact) per the iframe-per-page invariant in
// modules/display/host-iframe-wire.md. Per-field
// rendering happens INSIDE the iframe via the in-page registry — the host
// does NOT round-trip per field.
//
// This file (Subtask 4 of gui-schema-runtime) ships the core: URL resolution
// via App.GetWidgetURL with a defensive dev fallback, widgetId correlation,
// origin-checked message listener, and the iframe element itself. Per-kind
// message handlers (resize → height sync, theme push, error states) land in
// Subtask 5 (lifecycle.ts).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Box, Skeleton, Stack, useMantineTheme } from '@mantine/core'
import { IconAlertCircle } from '@tabler/icons-react'
import { GetWidgetURL } from '../../../wailsjs/go/main/App'

// Diagnostic helper — writes through the Wails binding so iframe lifecycle
// events show up in ktw-gui's log file (production Wails builds have no
// DevTools). Safe to keep on in production; the volume is bounded by the
// per-mount lifecycle (mount, resolve, ready, error).
type GoBindings = {
  go?: { main?: { App?: { LogFromFrontend?: (tag: string, msg: string) => Promise<void> } } }
}
function dlog(tag: string, msg: string) {
  try {
    const fn = (window as unknown as GoBindings).go?.main?.App?.LogFromFrontend
    if (fn) void fn(tag, msg)
  } catch {
    // never throw from a diagnostic
  }
}
import {
  isIframeMessage,
  type IframeMessage,
} from '@display/postmessage'
import { useAppStore } from '../../store'
import {
  clampHeight,
  dispatchIframeMessage,
  postTheme,
  postValue,
  resolveTheme,
  type LifecycleCallbacks,
} from './lifecycle'

/** Dev-mode default — used by the localStorage-driven fallback. */
const DEV_PORT_KEY = 'display.devPort'
const DEV_PORT_DEFAULT = '9990'

/** Status surfaced to consumers via onStatusChange / debug panels. */
export type IframeWidgetStatus =
  | 'loading'  // initial mount, awaiting URL resolution
  | 'ok'       // URL resolved + iframe loaded
  | 'fallback' // dev fallback URL in use (App.GetWidgetURL failed)
  | 'down'     // unrecoverable error (no URL, no fallback usable)

export interface IframeWidgetProps extends LifecycleCallbacks {
  /** Widget kind — 'form' / 'view' / 'table' / 'hello' / etc. */
  kind: string
  /** Optional registered schema name. */
  schemaName?: string
  /** Optional value reference (e.g. record id). */
  value?: string
  /** Optional mode — 'view' / 'create' / 'update' / 'filter'. */
  mode?: string
  /**
   * Optional universe override. Defaults to the GUI's displayed universe
   * (useAppStore.displayedUniverseName). Cross-universe widget mounting
   * is supported but rare — debug/comparison views.
   */
  universe?: string
  /** Optional status callback for parent debug panels. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void
  /**
   * Optional callback for every well-formed iframe message — fires
   * BEFORE the per-kind callbacks (`onReady`, `onChange`, `onResize`,
   * etc. inherited from LifecycleCallbacks). Use this for raw debug
   * panels / event log; use the per-kind callbacks for typed handlers.
   */
  onIframeMessage?: (msg: IframeMessage) => void
  /** iframe sandbox flags. Default: scripts + same-origin + forms. */
  sandbox?: string
  /** Initial container height in px (until resize message arrives). */
  initialHeight?: number
  /** Maximum container height in px — clamps runaway resize messages.
   * Default 800. Set higher for full-page compositions. */
  maxHeight?: number
  /** Extra query params appended to the iframe URL after URL resolution.
   *  Used by typed wrappers (<UIForm>, <UIView>, ...) to pass per-
   *  composition selectors (e.g. `?form=compact` for UIForm, `?view=card`
   *  + `?empty=omit` for UIView). The bun-side runtime reads these via
   *  `window.location.search` at mount time. */
  extraParams?: Record<string, string>
  /** When set, pushes the value via postMessage `value` kind on every
   *  change (after initial mount). Used by the LLM-streaming-protocol's
   *  artifact mount path: the artifact's value updates as patches arrive
   *  in the host's store; the iframe's harness picks up the new value
   *  via `listenToHost` and re-renders. The initial value is delivered
   *  via the `?value=<base64>` URL injection at mount time; this push
   *  handles subsequent updates. */
  pushValue?: unknown

  /** When true, the iframe takes 100% of its parent container's height
   *  instead of sizing to content via resize messages. Use for tab-
   *  embedded mounts where the parent already provides bounded
   *  dimensions (artifact tabs in ChatPanel). When false (default), the
   *  iframe sizes itself based on the harness's resize messages, which
   *  is right for the inline /dev demo where the page should grow with
   *  content. Wired by display-dsl-framework polish. */
  fillHeight?: boolean

  /** Render a Mantine Skeleton placeholder while the URL is resolving and
   *  the iframe hasn't yet posted `ready`. Default: true. Set false to
   *  manage the loading state at a higher layer (e.g. a host-side
   *  spinner that wraps multiple widgets). */
  showLoadingOverlay?: boolean
  /** Render a Mantine Alert when status flips to `down` (URL resolution
   *  failed AND the dev fallback isn't usable, OR the iframe element
   *  itself failed to load). Default: true. Set false to manage error
   *  UX externally via the `onStatusChange` callback. */
  showDownAlert?: boolean
}

/** Build the dev-fallback URL when App.GetWidgetURL is unreachable. */
function buildDevFallbackURL(
  kind: string,
  schemaName: string | undefined,
  value: string | undefined,
  mode: string | undefined,
  widgetId: string,
  extraParams: Record<string, string> | undefined,
): string {
  const port = (typeof window !== 'undefined' &&
    window.localStorage?.getItem(DEV_PORT_KEY)) || DEV_PORT_DEFAULT
  const params = new URLSearchParams()
  if (schemaName) params.set('schemaName', schemaName)
  if (value) params.set('value', value)
  if (mode) params.set('mode', mode)
  params.set('widgetId', widgetId)
  if (extraParams) {
    for (const [k, v] of Object.entries(extraParams)) {
      if (v !== undefined && v !== null && v !== '') params.set(k, v)
    }
  }
  return `http://127.0.0.1:${port}/widget/${encodeURIComponent(kind)}?${params.toString()}`
}

/** Append widgetId (and any extraParams) to a URL returned by the bridge
 *  so the iframe can echo it + read per-composition selectors. */
function appendWidgetId(
  url: string,
  widgetId: string,
  extraParams: Record<string, string> | undefined,
): string {
  try {
    const u = new URL(url)
    u.searchParams.set('widgetId', widgetId)
    if (extraParams) {
      for (const [k, v] of Object.entries(extraParams)) {
        if (v !== undefined && v !== null && v !== '') u.searchParams.set(k, v)
      }
    }
    return u.toString()
  } catch {
    // If the bridge returned a malformed URL, fall through verbatim —
    // the load will fail and the listener's origin check will reject it.
    return url
  }
}

export function IframeWidget(props: IframeWidgetProps) {
  const {
    kind,
    schemaName,
    value,
    mode,
    universe: universeProp,
    onStatusChange,
    onIframeMessage,
    sandbox = 'allow-scripts allow-same-origin allow-forms',
    initialHeight = 320,
    maxHeight = 800,
    extraParams,
    pushValue,
    fillHeight = false,
    showLoadingOverlay = true,
    showDownAlert = true,
    onReady,
    onRequestValue,
    onChange,
    onSubmit,
    onAction,
    onError,
    onResize,
  } = props

  const displayedUniverse = useAppStore((s) => s.displayedUniverseName)
  const universe = universeProp ?? displayedUniverse
  const themeMode = useAppStore((s) => s.appSettings.appearance.themeMode)

  // widgetId is stable per mount — used for inbound message correlation
  // and embedded in the URL so the iframe knows its own id.
  const widgetId = useMemo(
    () =>
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `wgt-${Math.random().toString(36).slice(2)}-${Date.now().toString(36)}`,
    [],
  )

  const [url, setUrl] = useState<string | null>(null)
  const [status, setStatusState] = useState<IframeWidgetStatus>('loading')
  const [errorDetail, setErrorDetail] = useState<string | undefined>()
  const [containerHeight, setContainerHeight] = useState<number>(initialHeight)
  const [iframeReady, setIframeReady] = useState(false)

  const setStatus = useCallback(
    (s: IframeWidgetStatus, detail?: string) => {
      setStatusState(s)
      setErrorDetail(detail)
      onStatusChange?.(s, detail)
    },
    [onStatusChange],
  )

  // --- URL resolution ---
  // Re-resolves whenever input props change. The widgetId stays stable
  // (one per mount); it's appended to whatever URL we end up with.
  useEffect(() => {
    let cancelled = false
    setStatus('loading')
    ;(async () => {
      // Universe must be selected — surface a clear "down" status with a
      // hint rather than calling Wails with an empty arg.
      if (!universe) {
        if (!cancelled) setStatus('down', 'no displayed universe')
        return
      }
      dlog('iframe-widget', `resolve start kind=${kind} widgetId=${widgetId} universe=${universe} extraKeys=${Object.keys(extraParams ?? {}).join(',')}`)
      try {
        const bridgeURL = await GetWidgetURL(
          universe,
          kind,
          schemaName ?? '',
          value ?? '',
          mode ?? '',
        )
        if (cancelled) return
        if (!bridgeURL) {
          dlog('iframe-widget', `resolve EMPTY-bridge-URL kind=${kind} widgetId=${widgetId}`)
          // Empty string from Wails treated as "down" — fallback below
          // wouldn't help (the bridge IS reachable but returned nothing).
          setUrl(buildDevFallbackURL(kind, schemaName, value, mode, widgetId, extraParams))
          setStatus('fallback', 'bridge returned empty URL')
          return
        }
        const finalURL = appendWidgetId(bridgeURL, widgetId, extraParams)
        dlog('iframe-widget', `resolve OK kind=${kind} widgetId=${widgetId} url=${finalURL.split('?')[0]}?<${finalURL.split('?')[1]?.length ?? 0} chars>`)
        setUrl(finalURL)
        setStatus('ok')
      } catch (e) {
        if (cancelled) return
        // Defensive dev fallback: the bridge / Wails binding / universe
        // is unreachable. Build the URL client-side from the dev port
        // and let the iframe try to load. If it fails too, the iframe
        // onerror handler will downgrade to 'down'.
        const detail = e instanceof Error ? e.message : String(e)
        dlog('iframe-widget', `resolve FAILED kind=${kind} widgetId=${widgetId} err=${detail}`)
        setUrl(buildDevFallbackURL(kind, schemaName, value, mode, widgetId, extraParams))
        setStatus('fallback', detail)
      }
    })()
    return () => {
      cancelled = true
    }
    // extraParams: stringified for stable deps — reference identity flips
    // on every parent render and we don't want to re-resolve URL each time.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [universe, kind, schemaName, value, mode, widgetId, setStatus, JSON.stringify(extraParams)])

  // --- Inbound message listener ---
  // Origin-checked, widgetId-correlated. Per-kind dispatch is Subtask 5;
  // here we only validate + forward the raw stream via onIframeMessage.
  const expectedOrigin = useMemo(() => {
    if (!url) return null
    try {
      return new URL(url).origin
    } catch {
      return null
    }
  }, [url])

  // CRITICAL: the listener attaches AT MOUNT, before the iframe URL is
  // resolved. Previously it was guarded by `if (!expectedOrigin) return`
  // in the useEffect, which deferred attachment until AFTER setUrl. On
  // remount with a warm bun-side cache the iframe loaded and emitted
  // `ready` BEFORE that effect ran, so the message was missed and the
  // host stayed in `isLoading: true` forever — the iframe stayed
  // `display: none` and the form / page tab looked empty. Surfaced
  // 2026-05-20: form vanished after a tab-switch round-trip.
  //
  // Fix: attach the listener unconditionally at mount, gate the handler
  // on the LATEST `expectedOrigin` via a ref (so a fresh `ready` lands
  // correctly the moment the URL resolves, even mid-tick).
  const handlerStateRef = useRef({
    expectedOrigin: null as string | null,
    widgetId,
    maxHeight,
    onIframeMessage,
    onReady,
    onRequestValue,
    onChange,
    onSubmit,
    onAction,
    onError,
    onResize,
    setStatus,
  })
  // Keep the ref pointing at the LATEST values every render so the
  // listener handler (attached once at mount) reads fresh state.
  handlerStateRef.current = {
    expectedOrigin,
    widgetId,
    maxHeight,
    onIframeMessage,
    onReady,
    onRequestValue,
    onChange,
    onSubmit,
    onAction,
    onError,
    onResize,
    setStatus,
  }

  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const state = handlerStateRef.current
      // Origin gate — drop everything that's not from our resolved URL.
      // If `expectedOrigin` is still null (URL not yet resolved) we drop
      // EVERY message; the bun harness retries `ready` via its own
      // useEffect on mount, so a missed-on-first-tick message would
      // arrive again. (Today's harness emits `ready` only once but the
      // URL is reliably available before the iframe even starts loading
      // because the same render cycle that sets `url` also commits the
      // iframe element.)
      if (!state.expectedOrigin || event.origin !== state.expectedOrigin) {
        const dataPreview = typeof event.data === 'object' && event.data
          ? JSON.stringify(event.data).slice(0, 200)
          : String(event.data)
        if (dataPreview.includes('widgetId') || dataPreview.includes('"type"')) {
          dlog('iframe-widget', `DROP origin=${event.origin} expected=${state.expectedOrigin} kind=${kind} widgetId=${state.widgetId} data=${dataPreview}`)
        }
        return
      }
      const data = event.data as unknown
      if (!isIframeMessage(data)) return
      // widgetId correlation — drop messages from sibling widgets.
      if (data.widgetId !== state.widgetId) {
        dlog('iframe-widget', `WIDGETID-MISMATCH expected=${state.widgetId} got=${(data as { widgetId?: string }).widgetId} type=${(data as { type?: string }).type} kind=${kind}`)
        return
      }
      dlog('iframe-widget', `msg type=${data.type} kind=${kind} widgetId=${state.widgetId}`)
      // Raw stream first.
      state.onIframeMessage?.(data)
      // Built-in side effects.
      if (data.type === 'ready') {
        setIframeReady(true)
      } else if (data.type === 'resize') {
        setContainerHeight(clampHeight(data.height, { min: 0, max: state.maxHeight }))
      } else if (data.type === 'error') {
        state.setStatus('down', `widget error: ${data.error.name}: ${data.error.message}`)
      }
      // Per-kind user callbacks.
      dispatchIframeMessage(data, {
        onReady:        state.onReady,
        onRequestValue: state.onRequestValue,
        onChange:       state.onChange,
        onSubmit:       state.onSubmit,
        onAction:       state.onAction,
        onError:        state.onError,
        onResize:       state.onResize,
      })
    }
    window.addEventListener('message', handler)
    return () => {
      window.removeEventListener('message', handler)
    }
    // Intentionally empty deps: attach once at mount. The handler reads
    // every dynamic value via `handlerStateRef.current` which is updated
    // every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // --- Ready-timeout fallback ---
  // If the iframe loads but never sends `ready` within 1.5 s (bun-side
  // bug, harness crash, postMessage dropped despite the attach-at-mount
  // fix, or — observed on form-kind remounts — bun's React tree throws
  // before the ready useEffect runs), force-flip iframeReady so the
  // skeleton overlay drops and the user sees whatever bun rendered
  // (often a useful error from the harness itself) instead of an
  // indefinite Mantine skeleton.
  //
  // The iframe ITSELF is no longer hidden behind the skeleton (see
  // render block below) — even at iframeReady=false the iframe is
  // visible underneath, so a working bun-side render shows immediately
  // and the skeleton just covers the brief mount window.
  useEffect(() => {
    if (!url || iframeReady) return
    const t = window.setTimeout(() => {
      dlog('iframe-widget', `ready-timeout: bun harness never emitted ready within 1.5s — dropping skeleton overlay kind=${kind} widgetId=${widgetId}`)
      setIframeReady(true)
    }, 1500)
    return () => window.clearTimeout(t)
  }, [url, iframeReady, kind, widgetId])

  // --- Theme push ---
  // Push the current theme + theme tokens on iframe-ready and on every
  // theme switch. The iframe subscribes via listenToHost() and applies
  // these via its MantineProvider so the iframe visually mirrors the
  // host (no Light-while-host-is-Dark mismatch; no font-size mismatch).
  // 'system' is resolved to the concrete dark|light per matchMedia.
  const hostTheme = useMantineTheme()
  const themeTokens = useMemo(() => extractThemeTokens(hostTheme), [hostTheme])
  useEffect(() => {
    if (!iframeReady || !expectedOrigin) return
    postTheme(
      iframeRef.current,
      expectedOrigin,
      resolveTheme(themeMode),
      undefined,
      themeTokens,
    )
  }, [iframeReady, expectedOrigin, themeMode, themeTokens])

  // --- Value push (LLM-streaming-protocol patches) ---
  // When `pushValue` changes, push the new value to the iframe via
  // postMessage `value` kind. The iframe's harness subscribes via
  // `listenToHost` (in app/ktw-display/src/runtime/postmessage.ts) and
  // updates its local state, re-rendering with the new value.
  //
  // The initial value is delivered via the `?value=<base64>` URL
  // injection at mount time; this push handles SUBSEQUENT updates as
  // patches arrive in the host's artifacts store. Stringified-deps
  // gate avoids re-pushing on identity-only changes (parent re-renders).
  useEffect(() => {
    if (!iframeReady || !expectedOrigin || pushValue === undefined) return
    postValue(iframeRef.current, expectedOrigin, kind, widgetId, pushValue)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [iframeReady, expectedOrigin, kind, widgetId, JSON.stringify(pushValue)])

  // Listen for the OS-level prefers-color-scheme change so 'system' theme
  // re-pushes when the OS dark mode toggles. No-op if themeMode isn't
  // 'system' — the matchMedia listener stays attached but resolveTheme
  // returns the explicit value and the push is idempotent.
  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    if (themeMode !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      if (!iframeReady || !expectedOrigin) return
      postTheme(
        iframeRef.current,
        expectedOrigin,
        resolveTheme(themeMode),
        undefined,
        themeTokens,
      )
    }
    // Older browsers expose addListener; modern ones use addEventListener.
    if ('addEventListener' in mq) mq.addEventListener('change', onChange)
    else (mq as unknown as { addListener: (cb: () => void) => void }).addListener(onChange)
    return () => {
      if ('removeEventListener' in mq) mq.removeEventListener('change', onChange)
      else (mq as unknown as { removeListener: (cb: () => void) => void }).removeListener(onChange)
    }
  }, [themeMode, iframeReady, expectedOrigin])

  // --- iframe load failure handling ---
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const onIframeError = useCallback(() => {
    dlog('iframe-widget', `iframe ONERROR kind=${kind} widgetId=${widgetId} url=${url ?? '<none>'}`)
    setStatus('down', 'iframe failed to load')
  }, [setStatus, kind, widgetId, url])
  const onIframeLoad = useCallback(() => {
    dlog('iframe-widget', `iframe ONLOAD kind=${kind} widgetId=${widgetId}`)
  }, [kind, widgetId])

  // Loading state: URL still resolving OR iframe hasn't posted `ready`.
  // The skeleton placeholder keeps the slot's footprint stable so the
  // surrounding layout doesn't jump on mount.
  const isLoading = !url || (!iframeReady && status !== 'down')

  // Down state: URL resolution failed AND no fallback usable, OR the
  // iframe element itself failed to load. Renders an Alert in place of
  // the iframe (the iframe is hidden via display:none so its postMessage
  // listener stays attached for any late `error` events).
  const isDown = status === 'down'

  return (
    <Box
      data-iframe-widget-shell={kind}
      style={
        fillHeight
          ? {
              position: 'relative',
              width: '100%',
              height: '100%',
              display: 'flex',
              flexDirection: 'column',
            }
          : { position: 'relative', width: '100%', minHeight: initialHeight }
      }
    >
      {isDown && showDownAlert && (
        <Alert
          icon={<IconAlertCircle size={16} />}
          title={`${kind} widget unavailable`}
          color="red"
          variant="light"
          data-testid={`iframe-widget.down.${kind}`}
        >
          {errorDetail ?? 'The widget could not be loaded.'}
        </Alert>
      )}

      {url && (
        <iframe
          ref={iframeRef}
          data-iframe-widget={kind}
          data-widget-id={widgetId}
          data-status={status}
          data-error={errorDetail}
          data-ready={iframeReady ? 'true' : 'false'}
          src={url}
          sandbox={sandbox}
          // A widget may go full screen (a deck's ▶ Present).
          allow="fullscreen"
          allowFullScreen
          title={`${kind} widget`}
          onLoad={onIframeLoad}
          onError={onIframeError}
          style={{
            width: '100%',
            height: fillHeight ? '100%' : containerHeight,
            flex: fillHeight ? 1 : undefined,
            minHeight: fillHeight ? 0 : initialHeight,
            border: 'none',
            // The iframe is ALWAYS visible once url resolves (unless the
            // host explicitly marks the widget down). Previously this was
            // `display: isLoading || isDown ? 'none' : 'block'`, which
            // hid the iframe while waiting for the bun-side `ready`
            // postMessage. When `ready` never arrived (bun-side crash on
            // form remount, observed 2026-05-28), the user saw a blank
            // panel forever. Letting the iframe render naturally means
            // the user sees what bun produces — content, error, or
            // genuine blank — regardless of the postMessage handshake.
            display: isDown ? 'none' : 'block',
          }}
        />
      )}

      {isLoading && showLoadingOverlay && !isDown && (
        <Stack
          gap="xs"
          data-testid={`iframe-widget.loading.${kind}`}
          aria-label={`Loading widget ${kind}`}
          style={{
            // Overlay the iframe with the skeleton placeholder until
            // `ready` arrives (or the 1.5s timeout fires). Absolute
            // positioning + background color keeps a stable footprint
            // while the iframe is still mounting; once iframeReady flips
            // (real or timeout) the overlay unmounts and the iframe
            // shows through.
            position: 'absolute',
            inset: 0,
            background: 'var(--mantine-color-body)',
            padding: 8,
            zIndex: 1,
          }}
        >
          <Skeleton height={32} radius="sm" />
          <Skeleton height={initialHeight - 80} radius="sm" />
          <Skeleton height={32} width="60%" radius="sm" />
        </Stack>
      )}
    </Box>
  )
}

// --- Theme-token extraction ---
// Pull the JSON-serializable subset of the host's Mantine theme into the
// wire-shape. The iframe's MantineProvider rebuilds a theme from these
// tokens. We deliberately exclude functions / non-serializable fields
// (other.fn, components, etc.) — only static tokens travel.
function extractThemeTokens(theme: ReturnType<typeof useMantineTheme>) {
  return {
    primaryColor: theme.primaryColor,
    fontFamily: theme.fontFamily,
    fontFamilyMonospace: theme.fontFamilyMonospace,
    fontSizes: theme.fontSizes as Record<string, string>,
    spacing: theme.spacing as Record<string, string>,
    radius: theme.radius as Record<string, string>,
    defaultRadius: typeof theme.defaultRadius === 'string' ? theme.defaultRadius : undefined,
    headings: theme.headings
      ? {
          fontFamily: theme.headings.fontFamily,
          sizes: theme.headings.sizes as Record<
            string,
            { fontSize?: string; fontWeight?: string; lineHeight?: string }
          > | undefined,
        }
      : undefined,
  }
}
