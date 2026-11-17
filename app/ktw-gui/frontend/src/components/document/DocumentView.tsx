// SPDX-License-Identifier: Apache-2.0
/**
 * DocumentView — THE entry point for showing a document
 * (modules/display/document-display.md). Every surface hands it a document
 * envelope; the registry picks the engine by media type. A document given by
 * its source, or without a media type, is read and typed in Go first, through
 * DocumentOpen.
 *
 * With `header`, it draws the tab chrome: the engine's label, the title, the
 * stream status, the size, and Edit / Save when the engine's capabilities say
 * so — no surface special-cases an engine.
 */
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ActionIcon, Badge, Button, Center, Group, Loader, Menu, Stack, Text, Tooltip } from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconDownload, IconEdit, IconFileExport, IconX } from '@tabler/icons-react'
import { DocumentOpen, SaveDocument } from '../../../wailsjs/go/main/App'
import { render } from '../../../wailsjs/go/models'
import './builtin'
import { useContributedEngines } from './contributed'
import { documentSize, documentText, needsResolution } from './content'
import { MEDIA_TYPE_UI } from '../../generated/media-types'
import { useEngine, type DisplayEngine, type Document, type EngineHost } from './engines'
import { SaveToWorkspaceButton, savableToWorkspace } from './SaveToWorkspace'
import { useAppStore } from '../../store'
import { DisplayPanel } from '../../sections/panels/DisplayPanel'
import { exportPanelLife, exportRunOutcome, withExportSource, type ExportPanelLife, type ExportRunResult } from './exportRun'
import { ExportFailure } from './ExportFailure'
import { useT } from '../../i18n'

export interface DocumentViewProps {
  doc: Document
  /** The universe the document belongs to (sources, contributed engines, iframes). */
  universe?: string
  /** The node a relative source path belongs to. */
  nodeId?: string
  /** The producer is still writing the document. */
  streaming?: boolean
  /** Draw the tab chrome (engine label, title, status, size, Edit / Save). */
  header?: boolean
  /** Extra header items, left of Edit / Save. */
  headerExtra?: ReactNode
  /** Drawn over the engine (e.g. an in-flight action). */
  overlay?: ReactNode
  host?: Omit<EngineHost, 'universe'>
  /** Edit mode: the engine committed edited content. */
  onContentChange?: (content: unknown) => void
  /** A relative link inside a document read from an object's file opens its
   *  target here (the Files tab selects it); without it the link is plain. */
  onOpenFile?: (path: string) => void
}

/** Reads a document given by its source, types one given without a media
 *  type (in Go: title as file name, then bytes), and resolves a ui-component
 *  naming a schema by id or carrying a ui block; any other inline document is
 *  returned as is. */
function useResolvedDocument(doc: Document, universe?: string, nodeId?: string): { doc?: Document; error?: string; loading?: boolean } {
  const [state, setState] = useState<{ doc?: Document; error?: string; key?: string }>({})
  const needsGo = !!doc.source || !doc.mediaType || (doc.mediaType === MEDIA_TYPE_UI && needsResolution(doc.content))
  const key = needsGo ? JSON.stringify([universe, nodeId, doc.source, doc.mediaType, doc.title, doc.source ? null : doc.content]) : undefined
  useEffect(() => {
    if (!key) return
    let cancelled = false
    DocumentOpen(universe ?? '', nodeId ?? '', render.Document.createFrom(doc))
      .then((r) => {
        if (!cancelled) setState({ doc: r.error ? undefined : (r.document as Document), error: r.error || undefined, key })
      })
      .catch((e: unknown) => { if (!cancelled) setState({ error: String(e), key }) })
    return () => { cancelled = true }
    // doc is keyed by `key` (its source); its other fields do not re-read it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  if (!key) return { doc }
  if (state.key !== key) return { loading: true }
  return state
}

/** The file a document given by a path of an object was read from: its
 *  neighbours (a diagram an image tag names, a sibling a link names) are
 *  shown through this same entry point, read from the same object's module. */
function fileHost(doc: Document, universe: string | undefined, nodeId: string | undefined, open?: (path: string) => void): EngineHost['file'] {
  const path = doc.source?.path
  if (!path || !nodeId) return undefined
  return {
    path,
    open,
    embed: (p, title) => (
      <DocumentView doc={{ source: { path: p }, title } as Document} universe={universe} nodeId={nodeId}
        host={{ fillHeight: true }} onOpenFile={open} />
    ),
  }
}

export function DocumentView({
  doc, universe, nodeId, streaming = false, header = false, headerExtra, overlay, host, onContentChange, onOpenFile,
}: DocumentViewProps) {
  useContributedEngines(universe)
  const t = useT('workspace')
  const tc = useT('components')
  // Where « Save to workspace » keeps a document: the host's universe, else
  // the one on screen (a chat's document tab names none).
  const displayedUniverse = useAppStore((s) => s.displayedUniverseName)
  const saveUniverse = universe || displayedUniverse || ''
  const resolved = useResolvedDocument(doc, universe, nodeId)
  const current = resolved.doc
  const engine = useEngine(current?.mediaType ?? '')
  const [mode, setMode] = useState<'view' | 'edit'>('view')
  // The export run last through the action surface: its panel drawn under the
  // header while it runs and kept once it ended.
  const [exportRun, setExportRun] = useState<ExportPanel | null>(null)

  // The content when editing started: a document updated meanwhile (a new
  // agent turn) is flagged, because saving the edit would overwrite it.
  const editSnapshot = useRef<unknown>(undefined)
  const remoteUpdate = mode === 'edit' && editSnapshot.current !== current?.content
  const startEdit = () => { editSnapshot.current = current?.content; setMode('edit') }
  const exitEdit = () => { editSnapshot.current = undefined; setMode('view') }

  let body: ReactNode
  if (resolved.loading) {
    body = <Center p="md"><Loader size="sm" /></Center>
  } else if (resolved.error || !current) {
    body = <Text size="xs" c="red" p="md">{resolved.error ?? tc('documentView.noDocument')}</Text>
  } else if (!engine) {
    body = <Text size="xs" c="red" p="md">{tc('documentView.noEngine', { mediaType: current.mediaType })}</Text>
  } else {
    const Engine = engine.component
    body = (
      <Engine
        doc={current}
        mode={mode}
        streaming={streaming}
        // Only a host that saves gets the engine's edits: an engine without
        // onChange knows its document cannot be saved here.
        onChange={engine.capabilities.edit && onContentChange ? (content) => { onContentChange(content); exitEdit() } : undefined}
        onExitEdit={exitEdit}
        host={{ ...host, universe, file: fileHost(doc, universe, nodeId, onOpenFile) ?? host?.file }}
      />
    )
  }

  if (!header) {
    return <div style={{ height: '100%', position: 'relative' }}>{overlay}{body}</div>
  }

  const canEdit = !!engine?.capabilities.edit && !streaming && !!current && !!onContentChange
  const editButtons = canEdit ? (
    <Group gap={6} wrap="nowrap">
      {remoteUpdate && (
        <Tooltip label={tc('documentView.remoteUpdateHint')} withArrow multiline w={260}>
          <Badge size="xs" color="yellow" variant="light" leftSection={<IconAlertTriangle size={10} style={{ marginTop: 2 }} />}>
            {tc('documentView.remoteUpdate')}
          </Badge>
        </Tooltip>
      )}
      {mode === 'edit' ? (
        <Tooltip label={tc('documentView.cancelEdit')} withArrow>
          <Button size="compact-xs" variant="default" leftSection={<IconX size={12} />} onClick={exitEdit}>{t('common.cancel')}</Button>
        </Tooltip>
      ) : (
        <Tooltip label={tc('documentView.edit')} withArrow>
          <Button size="compact-xs" variant="light" leftSection={<IconEdit size={12} />} onClick={startEdit}>{tc('documentView.edit')}</Button>
        </Tooltip>
      )}
    </Group>
  ) : null

  return (
    <Stack gap={0} style={{ height: '100%' }}>
      <Group gap="xs" px="sm" py={6} style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}>
        <Badge size="xs" variant="light" color="gray">{engine?.label ?? current?.mediaType ?? 'document'}</Badge>
        <Text size="xs" fw={500}>{current?.title ?? doc.title}</Text>
        <div style={{ flex: 1 }} />
        {streaming ? (
          <Group gap={4}><Loader size={10} color="blue" /><Text size="xs" c="dimmed">{tc('documentView.streaming')}</Text></Group>
        ) : (
          current && <Badge size="xs" color="green" variant="light">{tc('documentView.complete')}</Badge>
        )}
        {current && <Text size="xs" c="dimmed">{tc('documentView.sizeKb', { size: (documentSize(current) / 1024).toFixed(1) })}</Text>}
        {headerExtra}
        {editButtons}
        {/* A document that is not yet an object can become one (th25). */}
        {saveUniverse && current && !streaming && !host?.context?.objectId && savableToWorkspace(current) && (
          <SaveToWorkspaceButton doc={current} universe={saveUniverse} />
        )}
        {engine?.capabilities.export?.length && current ? (
          <ExportMenu doc={current} source={doc.source} engine={engine} universe={universe} nodeId={nodeId} onPanel={setExportRun} />
        ) : null}
        {engine?.capabilities.save && current && <SaveButton doc={current} />}
      </Group>
      {exportRun && universe && (
        <ExportRunPanel key={exportRun.panel} run={exportRun} universe={universe} onClose={() => setExportRun(null)} />
      )}
      <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', position: 'relative' }}>
        {overlay}
        {body}
      </div>
    </Stack>
  )
}

/** An export run through the action surface: its run's panel, the format it
 *  converts to. */
interface ExportPanel { panel: string; exportId: string; label: string }

type ExportBindings = {
  go?: { main?: { App?: {
    DocumentExportRun?: (universeName: string, nodeId: string, doc: render.Document, to: string) => Promise<ExportRunResult>
    DocumentExportRead?: (exportId: string) => Promise<{ document?: Document; error?: string }>
    DocumentExportDiscard?: (exportId: string) => Promise<void>
  } } }
}
const exportApp = () => (window as unknown as ExportBindings).go?.main?.App

/** The export's display panel, drawn where it was asked: each step it reports
 *  live; once it completes ok the file is read and goes to the save dialog,
 *  a failed or cancelled export is discarded. */
function ExportRunPanel({ run, universe, onClose }: { run: ExportPanel; universe: string; onClose: () => void }) {
  const t = useT('components')
  const [error, setError] = useState<string | null>(null)
  // The export is discarded when its panel goes away (closed, replaced by
  // another export, the view unmounted) before its run ended.
  const life = useRef<ExportPanelLife | null>(null)
  if (!life.current) life.current = exportPanelLife(run.exportId, (id) => { void exportApp()?.DocumentExportDiscard?.(id) })
  useEffect(() => { life.current?.enter(); return () => life.current?.leave() }, [])
  const ended = async (status: string) => {
    if (life.current?.ended(status) !== 'read') return
    try {
      const r = await exportApp()?.DocumentExportRead?.(run.exportId)
      if (!r || r.error || !r.document) throw new Error(r?.error || t('documentView.exportFailed'))
      const out = r.document
      const content = typeof out.content === 'string' ? out.content : documentText(out)
      await SaveDocument(out.title || 'export', out.mediaType, out.encoding ?? '', content)
    } catch (e) {
      setError(`${run.label}: ${reason(e)}`)
    }
  }
  return (
    <Stack gap={4} px="sm" py={6} style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }} data-element={`document-export:${run.exportId}`}>
      <Group justify="flex-end" gap={4}>
        {error && <ExportFailure label={t('documentView.exportFailed')} reason={error} />}
        <Tooltip label={t('documentView.closeExport')} withArrow>
          <ActionIcon size="xs" variant="subtle" color="gray" onClick={onClose} aria-label={t('documentView.closeExport')}><IconX size={12} /></ActionIcon>
        </Tooltip>
      </Group>
      <DisplayPanel universeName={universe} panelId={run.panel} variant="inline" title={t('documentView.exporting', { format: run.label })} onComplete={(status) => void ended(status)} />
    </Stack>
  )
}

/** Export: the formats the engine declares; the engine converts in the
 *  browser when it can, else Go converts through ktw-docs; the result goes to
 *  the save dialog. */
function ExportMenu({ doc, source, engine, universe, nodeId, onPanel }: { doc: Document; source?: Document['source']; engine: DisplayEngine; universe?: string; nodeId?: string; onPanel: (p: ExportPanel) => void }) {
  const t = useT('components')
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  // The host's conversion runs through the action surface: its panel drawn
  // under the header, the file saved when it ends.
  const viaGo = (d: Document, mediaType: string, label: string): Promise<Document | null> =>
    viaRun(withExportSource(d, source), mediaType, label, universe ?? '', nodeId, onPanel)
  const exportTo = async (mediaType: string, label: string) => {
    setBusy(label)
    setError(null)
    try {
      let out: Document | null
      const engineOnly = engine.capabilities.export?.find((f) => f.mediaType === mediaType)?.engineOnly
      if (engine.exportDocument && engineOnly) {
        out = await engine.exportDocument(doc, mediaType, { universe })
      } else if (engine.exportDocument) {
        try {
          out = await engine.exportDocument(doc, mediaType, { universe })
        } catch (first) {
          // The host converts what the engine does not; when it cannot either,
          // both reasons are shown.
          out = await viaGo(doc, mediaType, label).catch((second) => {
            throw new Error(`${reason(first)} — ${reason(second)}`)
          })
        }
      } else {
        out = await viaGo(doc, mediaType, label)
      }
      // A run's panel took the export over: it saves the file when it ends.
      if (!out) return
      const content = typeof out.content === 'string' ? out.content : documentText(out)
      await SaveDocument(out.title || 'export', out.mediaType, out.encoding ?? '', content)
    } catch (e) {
      setError(`${label}: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setBusy(null)
    }
  }
  return (
    <>
      {error && <ExportFailure label={t('documentView.exportFailed')} reason={error} />}
      <Menu position="bottom-end" withArrow>
        <Menu.Target>
          <Tooltip label={busy ? t('documentView.exporting', { format: busy }) : t('documentView.export')} position="bottom" withArrow>
            <ActionIcon size="sm" variant="subtle" color="gray" loading={!!busy} aria-label={t('documentView.exportDocument')}>
              <IconFileExport size={14} />
            </ActionIcon>
          </Tooltip>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Label>{t('documentView.exportAs')}</Menu.Label>
          {engine.capabilities.export!.map((f) => (
            <Menu.Item key={f.mediaType} onClick={() => void exportTo(f.mediaType, f.label)}>{f.label}</Menu.Item>
          ))}
        </Menu.Dropdown>
      </Menu>
    </>
  )
}

function reason(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** The export run through the action surface: a run's panel handed to the
 *  document view (null: nothing to save yet), or the document answered. */
async function viaRun(doc: Document, mediaType: string, label: string, universe: string, nodeId: string | undefined, onPanel: (p: ExportPanel) => void): Promise<Document | null> {
  const run = exportApp()?.DocumentExportRun
  if (!run) throw new Error('the export binding is missing: rebuild the GUI')
  const r = await run(universe, nodeId ?? '', render.Document.createFrom(doc), mediaType)
  const o = exportRunOutcome(r)
  if ('error' in o) throw new Error(o.error)
  if ('panel' in o) { onPanel({ panel: o.panel, exportId: o.exportId, label }); return null }
  return o.document
}

function SaveButton({ doc }: { doc: Document }) {
  const t = useT('components')
  const [saving, setSaving] = useState(false)
  const [savedFlash, setSavedFlash] = useState(false)
  const content = typeof doc.content === 'string' ? doc.content : documentText(doc)
  const save = async () => {
    if (saving) return
    setSaving(true)
    try {
      const path = await SaveDocument(doc.title || 'document', doc.mediaType, doc.encoding ?? '', content)
      if (path) {
        setSavedFlash(true)
        setTimeout(() => setSavedFlash(false), 1800)
      }
    } catch (err) {
      console.error('SaveDocument failed:', err)
    } finally {
      setSaving(false)
    }
  }
  return (
    <Tooltip label={savedFlash ? t('documentView.saved') : t('documentView.saveToFile')} position="bottom" withArrow>
      <ActionIcon size="sm" variant="subtle" color={savedFlash ? 'green' : 'gray'} onClick={save} loading={saving} disabled={!content || saving} aria-label={t('documentView.saveDocument')}>
        {savedFlash ? <IconCheck size={14} /> : <IconDownload size={14} />}
      </ActionIcon>
    </Tooltip>
  )
}
