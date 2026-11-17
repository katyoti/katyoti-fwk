// SPDX-License-Identifier: Apache-2.0
// Sidecar-contributed display engines: every `display-engine` contribution of
// a universe (render.EngineDecl) registers as an iframe engine on the one
// registry. A built-in engine wins a tie (the registry's rule).
//
// The widget page is the contribution's `widget` layer — one self-contained
// HTML page, fetched once through the contribution lane (DisplayEngineWidget)
// and loaded by srcdoc in a sandboxed iframe: no URL, no port.
//
// Widget protocol: the host posts `{type: 'value', value: <Document>, mode}`
// (mode 'view' | 'edit') once the page has loaded and again when the document
// or the mode changes; in edit mode the page commits with
// `{type: 'change', value: <content>}` — the host keeps it and leaves editing.
// For a format the engine
// declares in `capabilities.export`, the host posts `{type: 'export', id,
// mediaType}` and the page answers `{type: 'exported', id, value: <Document>}`
// (or `{type: 'exported', id, error}`).
import { Center, Loader, Text } from '@mantine/core'
import { useEffect, useRef, useState, type ComponentType } from 'react'
import { DisplayEngines, DisplayEngineWidget } from '../../../wailsjs/go/main/App'
import type { Document } from '../../generated/document'
import { displayEngines, type EngineProps } from './engines'

/** A widget page by engine URI, fetched once per universe. */
const widgetPages = new Map<string, Promise<string>>()

function widgetPage(universe: string, uri: string): Promise<string> {
  const key = `${universe}|${uri}`
  let page = widgetPages.get(key)
  if (!page) {
    page = DisplayEngineWidget(universe, uri).then((r) => {
      if (r.error || !r.html) throw new Error(r.error || `${uri} has no widget page`)
      return r.html
    })
    page.catch(() => widgetPages.delete(key))
    widgetPages.set(key, page)
  }
  return page
}

/** The open frames of an engine, so an export request reaches the one showing
 *  the document. */
const frames = new Map<string, Set<HTMLIFrameElement>>()

let exportSeq = 0

/** Ask the engine's widget to convert the document; the widget showing it
 *  answers. */
function exportThroughWidget(engineId: string) {
  return (doc: Document, mediaType: string): Promise<Document> => {
    const frame = [...(frames.get(engineId) ?? [])][0]
    const target = frame?.contentWindow
    if (!target) return Promise.reject(new Error('the engine is not showing a document'))
    const id = `export-${++exportSeq}`
    return new Promise<Document>((resolve, reject) => {
      const timer = window.setTimeout(() => { done(); reject(new Error('the engine did not answer the export')) }, 30_000)
      const onMessage = (e: MessageEvent) => {
        if (e.source !== target) return
        const m = e.data as { type?: string; id?: string; value?: Document; error?: string }
        if (m?.type !== 'exported' || m.id !== id) return
        done()
        if (m.error || !m.value) reject(new Error(m.error || 'the engine exported nothing'))
        else resolve(m.value)
      }
      const done = () => { window.clearTimeout(timer); window.removeEventListener('message', onMessage) }
      window.addEventListener('message', onMessage)
      target.postMessage({ type: 'export', id, mediaType, value: doc }, '*')
    })
  }
}

function widgetComponent(engineId: string, uri: string): ComponentType<EngineProps> {
  return function ContributedEngineView({ doc, mode, onChange, host }: EngineProps) {
    const frame = useRef<HTMLIFrameElement>(null)
    const loaded = useRef(false)
    const [html, setHtml] = useState<string>()
    const [error, setError] = useState<string>()
    useEffect(() => {
      if (!host.universe) { setError('no displayed universe'); return }
      let cancelled = false
      widgetPage(host.universe, uri).then((h) => { if (!cancelled) setHtml(h) }, (e) => { if (!cancelled) setError(String(e?.message ?? e)) })
      return () => { cancelled = true }
    }, [host.universe])
    const post = () => {
      if (loaded.current) frame.current?.contentWindow?.postMessage({ type: 'value', value: doc, mode }, '*')
    }
    useEffect(post, [doc.content, doc.mediaType, mode])
    useEffect(() => {
      const onMessage = (e: MessageEvent) => {
        if (e.source !== frame.current?.contentWindow) return
        const m = e.data as { type?: string; value?: unknown }
        if (m?.type === 'change' && onChange) onChange(m.value)
      }
      window.addEventListener('message', onMessage)
      return () => window.removeEventListener('message', onMessage)
    }, [onChange])
    useEffect(() => {
      const el = frame.current
      if (!el) return
      const set = frames.get(engineId) ?? new Set()
      set.add(el)
      frames.set(engineId, set)
      return () => { set.delete(el) }
    }, [html])
    if (error) return <Center h="100%"><Text c="red" size="sm">{error}</Text></Center>
    if (html === undefined) return <Center h="100%"><Loader size="sm" /></Center>
    return (
      <iframe
        ref={frame}
        srcDoc={html}
        sandbox="allow-scripts"
        title={doc.title ?? uri}
        onLoad={() => { loaded.current = true; post() }}
        style={{ width: '100%', height: '100%', border: 'none' }}
      />
    )
  }
}

const loadedFor = new Map<string, string[]>()

/** Registers the universe's contributed engines (once per universe; a new
 *  call replaces what the previous one registered). */
export async function loadContributedEngines(universe: string): Promise<void> {
  const r = await DisplayEngines(universe)
  if (r.error) {
    // eslint-disable-next-line no-console
    console.warn(`[display-engines] ${universe}: ${r.error}`)
  }
  for (const id of loadedFor.get(universe) ?? []) displayEngines.unregister(id)
  const ids: string[] = []
  for (const e of r.engines ?? []) {
    const id = `${e.provider}/${e.id}`
    ids.push(id)
    const exportFormats = e.capabilities?.export ?? []
    displayEngines.register({
      id,
      label: e.label || e.id,
      accepts: e.accepts ?? [],
      capabilities: {
        view: true, stream: !!e.capabilities?.stream, edit: !!e.capabilities?.edit, save: !!e.capabilities?.save,
        export: exportFormats.length ? exportFormats.map((f: { mediaType: string; label: string }) => ({ mediaType: f.mediaType, label: f.label })) : undefined,
      },
      builtin: false,
      component: widgetComponent(id, e.uri),
      exportDocument: exportFormats.length ? exportThroughWidget(id) : undefined,
    })
  }
  loadedFor.set(universe, ids)
}

/** Loads a universe's contributed engines the first time a document is shown
 *  in it. */
export function useContributedEngines(universe: string | undefined): void {
  useEffect(() => {
    if (!universe || loadedFor.has(universe)) return
    loadedFor.set(universe, [])
    void loadContributedEngines(universe)
  }, [universe])
}
