// SPDX-License-Identifier: Apache-2.0
// <UIDeck> — typed host wrapper for the bun-side slide-deck widget
// (`/widget/deck`, modules/slidedeck/).
//
// The deck is markdown rendered live by reveal.js inside the iframe. The
// markdown and the streaming flag reach it by postMessage — never the URL —
// on mount and on every change, so a deck being written by an agent grows
// slide by slide and an edited deck re-renders on the slide it was showing.
//
// "Present in a window" posts the deck to the display server's deck
// store and opens it in the system browser, where it can go to a second screen
// with reveal.js's speaker view, or be shared as a window in Teams.
// While that window is open, every change of the markdown is posted again and
// the presentation follows it.
//
// A deck naming a theme other than katyoti: the host publishes that theme to
// the display server (DeckThemePublish — resolved through the universe's
// catalog) and tells the page its version; the page loads it by name, in the
// embedded view and in the presenting window alike.
//
// Editing (DeckSettingsPanel): the deck's settings and the shown slide's, in a
// panel docked right of the deck. The deck shows the draft as it changes;
// Save hands it to the host (`onSave`, a document or an artifact) or, for a
// slide-deck object, writes the object itself: its `deck` fields and its body.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Menu } from '@mantine/core'
import { IframeWidget, type IframeWidgetStatus } from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import {
  DeckAssetsPublish, DeckEditorGrammar, DeckSlideGroups, DeckThemePublish, GetWidgetURL, SaveDocument, WorkspaceObjectNamedFileRead, WorkspaceObjectNamedFileWrite,
  WorkspaceObjectUpdateWithBody,
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { useAppStore } from '../../store'
import { displayName, useProfileStore } from '../../store/profileStore'
// parse.ts and theme.ts only — the deck package's index pulls in reveal.js.
import { parseDeck } from '@ktw/display-lib/ui-builders/deck/parse.ts'
import { DEFAULT_THEME } from '@ktw/display-lib/ui-builders/deck/theme.ts'
import { insertSlideAfter, slideIndexAt } from '@ktw/display-lib/ui-builders/deck/settings.ts'
import { inlineGroups } from '@ktw/display-lib/ui-builders/deck/groups.ts'
import { DeckSettingsPanel } from './DeckSettingsPanel'
import { DeckSlideGroupPicker } from './DeckSlideGroupPicker'
import { noteRevision, revisionOf } from '../../sections/workspace/objectRevision'
import { useT } from '../../i18n'

export interface UIDeckProps {
  /** The deck's markdown. */
  markdown: string
  /** The producer is still writing the deck. */
  streaming?: boolean
  /** Names the deck across presentations (a document title or path); the
   *  drawings are kept per name. */
  deckKey?: string
  /** The slide-deck object shown, when the deck is one (its pane's Deck
   *  tab): its drawings are read from, and saved to, its `drawings` file. */
  objectId?: string
  /** The deck's file on disk (absolute), when it is one: the images it names
   *  by a relative path are read beside it. */
  sourcePath?: string
  universe?: string
  onReady?: () => void
  onError?: (error: { name: string; message: string; stack?: string }) => void
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void
  initialHeight?: number
  maxHeight?: number
  fillHeight?: boolean
  /** The host's edit mode (a document's Edit): the settings panel is open. */
  editing?: boolean
  /** Saves the edited markdown (a document or an artifact). Without it, and
   *  without an objectId, the deck is not editable. */
  onSave?: (markdown: string) => void
  /** The panel closed without saving. */
  onEditEnd?: () => void
}

/** Where the deck store is, and the token that opens it, from the widget URL
 *  the bridge hands out for this universe. */
async function displayBase(universe: string): Promise<{ origin: string; token: string }> {
  const u = new URL(await GetWidgetURL(universe, 'deck', '', '', ''))
  return { origin: u.origin, token: u.searchParams.get('token') ?? '' }
}

/** Where the files beside a deck are on the display server (DeckAssetsPublish). */
export interface DeckAssets { scope: string; version: string }

async function storeDeck(base: { origin: string; token: string }, id: string, markdown: string, assets?: DeckAssets) {
  const r = await fetch(`${base.origin}/widget/deck/store?token=${encodeURIComponent(base.token)}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ id, markdown, assets }),
  })
  if (!r.ok) throw new Error(`deck store: ${r.status}`)
}

/** The deck as a PowerPoint file, with its speaker notes: real slides
 *  (`editable`: text boxes, pictures and shapes measured from the deck page)
 *  or one picture per slide. The display server shows each slide in a
 *  headless browser. The deck's theme is published first, as to present.
 *  Returns the .pptx as base64. */
/** Publishes the files beside a deck — a deck object's (objectId), or a deck
 *  file's (an absolute path) — and returns where the page finds them; none
 *  when the deck names no file beside it. */
export async function publishDeckAssets(markdown: string, universe: string, objectId?: string, sourcePath?: string): Promise<DeckAssets | undefined> {
  const r = await DeckAssetsPublish(universe, objectId ?? '', sourcePath ?? '', markdown)
  if (r.error) throw new Error(r.error)
  return r.scope && r.version ? { scope: r.scope, version: r.version } : undefined
}

export async function exportDeckPptx(markdown: string, universe: string, editable = true, assets?: DeckAssets): Promise<string> {
  const t = parseDeck(markdown).settings?.theme
  if (typeof t === 'string' && t && t !== DEFAULT_THEME) await DeckThemePublish(universe, t)
  const base = await displayBase(universe)
  const r = await fetch(`${base.origin}/widget/deck/export/pptx?token=${encodeURIComponent(base.token)}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ markdown, editable, assets }),
  })
  if (!r.ok) {
    const e = await r.json().catch(() => ({})) as { error?: string }
    throw new Error(e.error || `PowerPoint export: ${r.status}`)
  }
  const bytes = new Uint8Array(await r.arrayBuffer())
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(bin)
}

/** The deck as one standalone HTML page: presents with no server and no
 *  network (theme, fonts and pictograms inside). The theme is published first. */
export async function exportDeckHtml(markdown: string, universe: string, assets?: DeckAssets): Promise<string> {
  const t = parseDeck(markdown).settings?.theme
  if (typeof t === 'string' && t && t !== DEFAULT_THEME) await DeckThemePublish(universe, t)
  const base = await displayBase(universe)
  const r = await fetch(`${base.origin}/widget/deck/export/html?token=${encodeURIComponent(base.token)}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ markdown, assets }),
  })
  if (!r.ok) {
    const e = await r.json().catch(() => ({})) as { error?: string }
    throw new Error(e.error || `HTML export: ${r.status}`)
  }
  return r.text()
}

// Both colour schemes: no light button on the dark theme.
const hostButton = {
  font: '12px system-ui, sans-serif', color: 'light-dark(#4f46e5, #a5b4fc)',
  background: 'light-dark(#f3f4f6, var(--mantine-color-dark-6))',
  border: '1px solid light-dark(#e5e7eb, var(--mantine-color-dark-4))', borderRadius: 999, padding: '2px 10px', cursor: 'pointer',
} as const

// A host button at work: dimmed, no pointer, until its job is done.
const busyButton = { ...hostButton, opacity: 0.55, cursor: 'progress' } as const

const storeId = (key: string) =>
  key.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 48) || 'deck'

export function UIDeck({
  markdown,
  streaming = false,
  deckKey,
  objectId,
  sourcePath,
  universe: universeProp,
  onReady,
  onError,
  onStatusChange,
  initialHeight = 540,
  maxHeight = 2000,
  fillHeight,
  editing = false,
  onSave,
  onEditEnd,
}: UIDeckProps) {
  const tr = useT('components')
  const displayed = useAppStore((s) => s.displayedUniverseName)
  const universe = universeProp ?? displayed ?? undefined

  // Editing: the panel's draft is what the deck shows while it is open; a
  // saved draft stays shown until the host's markdown catches up.
  const selfSaving = !!objectId && !!universe
  const canEdit = (selfSaving || !!onSave) && !streaming
  const [localEdit, setLocalEdit] = useState(false)
  const open = canEdit && (editing || localEdit)
  const [draft, setDraft] = useState<string | null>(null)
  const [saved, setSaved] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [at, setAt] = useState<{ h: number; v: number } | null>(null)
  const [grammar, setGrammar] = useState<main.DeckEditorGrammarResult | null>(null)
  const [grammarError, setGrammarError] = useState<string | null>(null)
  // « Insérer un groupe de slides »: the picker over the deck being edited.
  const [pickGroup, setPickGroup] = useState(false)
  useEffect(() => {
    setDraft(open ? markdown : null)
    setSaveError(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
  useEffect(() => { setSaved(null) }, [markdown])
  useEffect(() => {
    if (!open || grammar) return
    let cancelled = false
    DeckEditorGrammar(universe ?? '')
      .then((g) => { if (!cancelled) { setGrammar(g); setGrammarError(g.error || null) } })
      .catch((e) => { if (!cancelled) setGrammarError(e instanceof Error ? e.message : String(e)) })
    return () => { cancelled = true }
  }, [open, grammar, universe])
  const shown = draft ?? saved ?? markdown

  // The slide groups the deck includes (th22): read from the universe — the
  // project's first — and shown in place of their include slides. Where the
  // deck leaves the editor (the window, the exports) it goes with them
  // written in (inlineGroups).
  const [groups, setGroups] = useState<Record<string, string> | undefined>(undefined)
  useEffect(() => {
    if (!universe || streaming || !/include=/.test(shown)) { setGroups(undefined); return }
    let cancelled = false
    const t = setTimeout(() => {
      DeckSlideGroups(universe, shown)
        .then((g) => { if (!cancelled) setGroups(g ?? {}) })
        .catch(() => {})
    }, 400)
    return () => { cancelled = true; clearTimeout(t) }
  }, [shown, universe, streaming])
  const outgoing = useMemo(() => inlineGroups(shown, groups), [shown, groups])

  // The files beside the deck (its relative images and backgrounds): read
  // where the deck lives and published to the display server.
  const [assets, setAssets] = useState<DeckAssets | undefined>(undefined)
  const [assetsMissing, setAssetsMissing] = useState<string[]>([])
  useEffect(() => {
    if (!universe || streaming) return
    let cancelled = false
    const t = setTimeout(() => {
      DeckAssetsPublish(universe, objectId ?? '', sourcePath ?? '', shown)
        .then((r) => {
          if (cancelled) return
          setAssets(r.scope && r.version ? { scope: r.scope, version: r.version } : undefined)
          setAssetsMissing(r.missing ?? [])
        })
        .catch(() => {})
    }, 400)
    return () => { cancelled = true; clearTimeout(t) }
  }, [shown, universe, objectId, sourcePath, streaming])
  // A deck object's drawings: read once before the deck is first pushed (the
  // board starts from them), written back — debounced by the view — on every
  // change, through the CWM named-file method.
  const [drawings, setDrawings] = useState<string | undefined>(undefined)
  const [drawingsLoaded, setDrawingsLoaded] = useState(!objectId)
  useEffect(() => {
    if (!objectId || !universe) { setDrawingsLoaded(true); return }
    let cancelled = false
    WorkspaceObjectNamedFileRead(universe, objectId, '', 'drawings')
      .then((f) => { if (!cancelled && f.exists && f.content) setDrawings(f.content) })
      .catch(() => { /* no drawings yet */ })
      .finally(() => { if (!cancelled) setDrawingsLoaded(true) })
    return () => { cancelled = true }
  }, [objectId, universe])

  // A deck object's review: its `review` named file — comment
  // threads per slide and the reviews marked done — read with the drawings,
  // written on every change the view reports.
  const [review, setReview] = useState<string | undefined>(undefined)
  useEffect(() => {
    if (!objectId || !universe) return
    let cancelled = false
    WorkspaceObjectNamedFileRead(universe, objectId, '', 'review')
      .then((f) => { if (!cancelled && f.exists && f.content) setReview(f.content) })
      .catch(() => { /* no review yet */ })
    return () => { cancelled = true }
  }, [objectId, universe])
  const reviewer = displayName(useProfileStore((s) => s.profile))

  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      if (msg.type === 'ready') onReady?.()
      else if (msg.type === 'error') onError?.(msg.error)
      else if (msg.type === 'change' && (msg.value as { slide?: unknown } | undefined)?.slide) {
        const sl = (msg.value as { slide: { h?: unknown; v?: unknown } }).slide
        if (typeof sl.h === 'number' && typeof sl.v === 'number') setAt({ h: sl.h, v: sl.v })
      } else if (msg.type === 'change' && objectId && universe && typeof (msg.value as { review?: unknown } | undefined)?.review === 'string') {
        const r = (msg.value as { review: string }).review
        setReview(r)
        WorkspaceObjectNamedFileWrite(universe, objectId, '', 'review', r)
          .catch(() => { /* a failed save leaves the session's review */ })
      } else if (msg.type === 'change' && objectId && universe) {
        const d = (msg.value as { drawings?: unknown } | undefined)?.drawings
        if (typeof d !== 'string') return
        WorkspaceObjectNamedFileWrite(universe, objectId, '', 'drawings', d)
          .catch(() => { /* a failed save leaves the session's drawings */ })
      }
    },
    [onReady, onError, objectId, universe],
  )
  const scheme = typeof document !== 'undefined' &&
    document.documentElement.getAttribute('data-mantine-color-scheme') === 'dark' ? 'dark' : 'light'

  // A stable id per deck for the store: the deck's name when it has one.
  const id = useMemo(
    () => storeId(objectId ?? deckKey ?? `deck-${Math.random().toString(36).slice(2, 10)}`),
    [objectId, deckKey],
  )
  const presented = useRef<{ origin: string; token: string } | null>(null)
  const [presentError, setPresentError] = useState<string | null>(null)
  // A host button at work: which one, since when; a second click is ignored.
  const [busy, setBusy] = useState<{ what: 'window' | 'pdf' | 'pptx' | 'html'; since: number } | null>(null)
  const busyRef = useRef(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [, tick] = useState(0)
  useEffect(() => {
    if (!busy) return
    const t = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(t)
  }, [busy])
  useEffect(() => {
    if (!notice) return
    const t = setTimeout(() => setNotice(null), 6000)
    return () => clearTimeout(t)
  }, [notice])
  const run = async (what: 'window' | 'pdf' | 'pptx' | 'html', fn: () => Promise<string | void>) => {
    if (busyRef.current) return
    busyRef.current = true
    setBusy({ what, since: Date.now() })
    setPresentError(null)
    setNotice(null)
    try {
      const done = await fn()
      if (done) setNotice(done)
    } catch (e) {
      setPresentError(e instanceof Error ? e.message : String(e))
    } finally {
      busyRef.current = false
      setBusy(null)
    }
  }
  const seconds = busy ? Math.max(0, Math.round((Date.now() - busy.since) / 1000)) : 0

  // The deck's theme, published to the display server; its version reaches
  // the page with the value, and a new one makes the page read it again.
  const themeName = useMemo(() => {
    const t = parseDeck(shown ?? '').settings?.theme
    return typeof t === 'string' && t && t !== DEFAULT_THEME ? t : ''
  }, [shown])
  const [themeVersion, setThemeVersion] = useState<string | undefined>(undefined)
  const [themeError, setThemeError] = useState<string | null>(null)
  const publishTheme = useCallback(
    () => (themeName && universe ? DeckThemePublish(universe, themeName) : Promise.resolve('')),
    [themeName, universe],
  )
  useEffect(() => {
    setThemeError(null)
    if (!themeName) { setThemeVersion(undefined); return }
    let cancelled = false
    publishTheme()
      .then((v) => { if (!cancelled) setThemeVersion(v || undefined) })
      .catch((e) => { if (!cancelled) setThemeError(`theme ${themeName}: ${e instanceof Error ? e.message : String(e)}`) })
    return () => { cancelled = true }
  }, [themeName, publishTheme])

  // Opens the deck in the system browser. `pdf` opens reveal.js's print
  // layout (one page per slide and per reveal step) and the page runs the
  // browser's print dialog — choose "Save as PDF".
  const openInBrowser = (pdf: boolean) => run(pdf ? 'pdf' : 'window', async () => {
      const base = await displayBase(universe ?? '')
      // The window loads the deck's theme by name: it must be there first.
      await publishTheme().catch(() => {})
      await storeDeck(base, id, outgoing, assets)
      presented.current = base
      const open = new URL(`${base.origin}/widget/deck`)
      open.searchParams.set('deck', id)
      open.searchParams.set('present', '1')
      if (pdf) {
        open.searchParams.set('print-pdf', '')
        open.searchParams.set('autoprint', '1')
      }
      if (base.token) open.searchParams.set('token', base.token)
      BrowserOpenURL(open.toString())
      return pdf
        ? tr('uiDeck.openedPdf')
        : tr('uiDeck.openedWindow')
  })

  // Keep a presented deck current: the window follows the store.
  useEffect(() => {
    const base = presented.current
    if (!base) return
    const t = setTimeout(() => {
      storeDeck(base, id, outgoing, assets).catch(() => {})
    }, 250)
    return () => clearTimeout(t)
  }, [outgoing, id, assets])

  const closeEditor = () => {
    setLocalEdit(false)
    setDraft(null)
    onEditEnd?.()
  }
  const save = async () => {
    if (draft === null) return
    setSaveError(null)
    if (!selfSaving) {
      setSaved(draft)
      onSave?.(draft)
      closeEditor()
      return
    }
    // A slide-deck object: its settings are its `deck` fields (the title is
    // its name), its slides are its body (the panel's composition, inverted).
    setSaving(true)
    try {
      const settings = parseDeck(draft).settings as Record<string, unknown>
      const group: Record<string, string> = {}
      for (const f of grammar?.object ?? []) {
        const v = settings[f.name]
        if (v != null && String(v) !== '') group[f.name] = String(v)
      }
      const r = await WorkspaceObjectUpdateWithBody(universe!, objectId!, JSON.stringify({ deck: group }), deckBody(draft), revisionOf(objectId!))
      noteRevision(objectId!, r.revision)
      if (r.error) throw new Error(r.error)
      setSaved(draft)
      closeEditor()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }
  const slide = at ? slideIndexAt(shown, at.h, at.v, groups) : -1
  const deckLook = useMemo(() => parseDeck(shown).settings as Record<string, unknown>, [shown])


  // The file name of an export: the deck's title, else its key.
  const exportName = () => {
    const t = parseDeck(shown).settings?.title
    return (typeof t === 'string' && t ? t : deckKey || 'presentation').replace(/[\\/:*?"<>|]+/g, '-').replace(/\.deck\.md$|\.md$/i, '')
  }
  // PowerPoint: real slides, or one picture per slide; saved where the user picks.
  const exportPptx = (editable: boolean) => run('pptx', async () => {
    const content = await exportDeckPptx(outgoing, universe ?? '', editable, assets).catch((e) => {
      throw new Error(`PowerPoint: ${e instanceof Error ? e.message : String(e)}`)
    })
    const saved = await SaveDocument(`${exportName()}.pptx`, 'application/vnd.openxmlformats-officedocument.presentationml.presentation', 'base64', content)
    return saved ? tr('uiDeck.pptxSaved', { path: saved }) : undefined
  })
  // A standalone HTML page: presents offline, by a double click.
  const exportHtml = () => run('html', async () => {
    const html = await exportDeckHtml(outgoing, universe ?? '', assets).catch((e) => {
      throw new Error(`HTML: ${e instanceof Error ? e.message : String(e)}`)
    })
    const saved = await SaveDocument(`${exportName()}.html`, 'text/html', '', html)
    return saved ? tr('uiDeck.htmlSaved', { path: saved }) : undefined
  })
  const slideCount = parseDeck(shown).slides.length

  return (
    <div style={{ display: 'flex', height: fillHeight ? '100%' : undefined }}>
    <div style={{ position: 'relative', flex: 1, minWidth: 0, height: fillHeight ? '100%' : undefined }}>
      <div style={{ position: 'absolute', top: 8, right: 12, zIndex: 5, display: 'flex', gap: 6 }}>
        {selfSaving && canEdit && !open ? (
          <button
            type="button"
            onClick={() => setLocalEdit(true)}
            title={tr('uiDeck.editHint')}
            style={hostButton}
          >
            {tr('uiDeck.edit')}
          </button>
        ) : null}
        <button
          type="button"
          onClick={() => openInBrowser(false)}
          disabled={!!busy}
          title={tr('uiDeck.presentHint')}
          style={busy ? busyButton : hostButton}
        >
          {busy?.what === 'window' ? tr('uiDeck.opening', { icon: '⧉' }) : tr('uiDeck.present')}
        </button>
        <Menu position="bottom-end" withArrow disabled={!!busy || streaming}>
          <Menu.Target>
            <button
              type="button"
              data-guide="deck.export"
              disabled={!!busy || streaming}
              title={tr('uiDeck.exportHint')}
              style={busy || streaming ? busyButton : hostButton}
            >
              {busy?.what === 'pdf' ? tr('uiDeck.opening', { icon: '⎙' })
                : busy?.what === 'pptx' ? tr('uiDeck.pptxProgress', { seconds, count: slideCount })
                : busy?.what === 'html' ? '⇩ HTML…'
                : tr('uiDeck.export')}
            </button>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item data-guide="deck.export-pdf" onClick={() => openInBrowser(true)}>
              {tr('uiDeck.exportPdf')}
            </Menu.Item>
            <Menu.Item data-guide="deck.export-pptx" onClick={() => exportPptx(true)}>
              {tr('uiDeck.exportPptx')}
            </Menu.Item>
            <Menu.Item data-guide="deck.export-pptx-pictures" onClick={() => exportPptx(false)}>
              {tr('uiDeck.exportPptxPictures')}
            </Menu.Item>
            <Menu.Item data-guide="deck.export-html" onClick={exportHtml}>
              {tr('uiDeck.exportHtml')}
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
      </div>
      {!presentError && !themeError && !notice && assetsMissing.length ? (
        <div style={{ position: 'absolute', top: 36, right: 12, zIndex: 5, font: '12px system-ui', color: '#b45309', maxWidth: 420, textAlign: 'right' }}>
          {tr('uiDeck.assetsMissing', { files: assetsMissing.join(', ') })}
        </div>
      ) : null}
      {presentError || themeError || notice ? (
        <div style={{ position: 'absolute', top: 36, right: 12, zIndex: 5, font: '12px system-ui', color: presentError || themeError ? '#dc2626' : '#16a34a', maxWidth: 420, textAlign: 'right' }}>
          {presentError ?? themeError ?? notice}
        </div>
      ) : null}
      <IframeWidget
        kind="deck"
        universe={universe}
        extraParams={{ display: 'ui' }}
        onIframeMessage={onIframeMessage}
        onStatusChange={onStatusChange}
        initialHeight={initialHeight}
        maxHeight={maxHeight}
        pushValue={shown && drawingsLoaded ? { markdown: shown, streaming, scheme, annotationsKey: id, drawings, themeVersion, assets, groups, ...(objectId ? { review, reviewer, reviewable: true } : {}) } : undefined}
        fillHeight={fillHeight}
      />
    </div>
    {open && draft !== null ? (
      <div style={{ width: 300, flexShrink: 0, borderLeft: '1px solid var(--mantine-color-default-border)', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
        <DeckSettingsPanel
          grammar={grammar}
          grammarError={grammarError}
          markdown={draft}
          onMarkdownChange={setDraft}
          slide={slide}
          readOnly={selfSaving ? { title: tr('uiDeck.titleReadOnly') } : undefined}
          saving={saving}
          error={saveError}
          onSave={save}
          onCancel={closeEditor}
          onInsertGroup={universe ? () => setPickGroup(true) : undefined}
        />
        {universe ? (
          <DeckSlideGroupPicker
            opened={pickGroup}
            universe={universe}
            slide={slide}
            look={{ theme: typeof deckLook.theme === 'string' ? deckLook.theme : undefined, aspect: typeof deckLook.aspect === 'string' ? deckLook.aspect : undefined }}
            preview={(md) => <UIDeck markdown={md} universe={universe} deckKey={`group-preview:${id}`} fillHeight />}
            onPick={(name) => {
              setDraft((d) => (d === null ? d : insertSlideAfter(d, slide, `<!-- slide: include=${name} -->`)))
              setPickGroup(false)
            }}
            onClose={() => setPickGroup(false)}
          />
        ) : null}
      </div>
    ) : null}
    </div>
  )
}

/** A deck object's body: the slides, after the frontmatter its Deck panel
 *  composes (and the blank line it puts after it). */
function deckBody(markdown: string): string {
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  if (lines[0]?.trimEnd() !== '---') return markdown
  const end = lines.findIndex((l, i) => i > 0 && l.trimEnd() === '---')
  if (end < 0) return markdown
  const rest = lines.slice(end + 1)
  if (rest[0]?.trim() === '') rest.shift()
  return rest.join('\n')
}
