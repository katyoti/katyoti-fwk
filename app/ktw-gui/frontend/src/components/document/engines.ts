// SPDX-License-Identifier: Apache-2.0
// The display-engine contract and THE registry the GUI resolves documents
// through (modules/display/document-display.md). Built-in engines register in
// ./builtin; a sidecar's `display-engine` contributions join the same registry
// through ./contributed.
import { useSyncExternalStore, type ComponentType, type ReactNode } from 'react'
import type { Document } from '../../generated/document'
import { EngineRegistry, type RegisteredEngine } from './engineRegistry'

export type { Document } from '../../generated/document'
export type { EngineCapabilities, ExportFormat } from './engineRegistry'

/** What the host gives an engine besides the document: where it is mounted
 *  and how an interaction flows back. `context` is the object an extension
 *  view is shown on (a ui-component of type `extension`). */
export interface EngineHost {
  universe?: string
  fillHeight?: boolean
  /** The producer finished writing (or the stream settled). */
  streamingComplete?: boolean
  /** A form or wizard submitted a value. */
  onSubmit?: (value: unknown) => void
  /** A component emitted an action (a row click, a button). */
  onAction?: (action: string, args: unknown) => void
  /** Live value pushed into an already-mounted component (cross-turn patches). */
  pushValue?: unknown
  context?: EngineObjectContext
  /** The file the document was read from (a path of the host's object's
   *  module): a relative image or link inside it resolves against its folder. */
  file?: EngineFile
}

/** The file a document was read from, and how a neighbour is shown: `embed`
 *  draws another file of the module in place (by its media type), `open`
 *  makes it the document shown (a link to a sibling). */
export interface EngineFile {
  path: string
  embed: (path: string, title: string) => ReactNode
  open?: (path: string) => void
}

/** The object an extension view is shown on. */
export interface EngineObjectContext {
  universeName: string
  objectId: string
  objectName: string
  detail?: unknown
  /** Re-read the panel's source (after the view ran an action that changed it). */
  refresh?: () => void
}

export interface EngineProps {
  doc: Document
  mode: 'view' | 'edit'
  /** The producer is still writing the document. */
  streaming: boolean
  /** Edit mode: the engine reports the edited content (same shape as doc.content). */
  onChange?: (content: unknown) => void
  /** Edit mode: the engine leaves editing (its own Save or Cancel). */
  onExitEdit?: () => void
  host: EngineHost
}

/** A display engine. Its `exportDocument`, when present, converts in the
 *  browser; without it the host converts through Go (DocumentExportRun). */
export type DisplayEngine = RegisteredEngine<ComponentType<EngineProps>, Document>

export const DOWNLOAD_ENGINE_ID = 'download'
export const TEXT_ENGINE_ID = 'text'

/** THE registry: every surface resolves a document's engine here. */
export const displayEngines = new EngineRegistry<ComponentType<EngineProps>, Document>(DOWNLOAD_ENGINE_ID)

/** Registers engines; a module-level call, so importing ./builtin is enough. */
export function registerEngines(...engines: DisplayEngine[]): void {
  for (const e of engines) displayEngines.register(e)
}

/** The engine for a media type, re-resolved when engines are (un)registered. */
export function useEngine(mediaType: string): DisplayEngine | undefined {
  useSyncExternalStore(
    (fn) => displayEngines.subscribe(fn),
    () => displayEngines.getVersion(),
  )
  return displayEngines.resolve(mediaType)
}
