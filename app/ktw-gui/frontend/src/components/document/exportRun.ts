// SPDX-License-Identifier: Apache-2.0
/**
 * The document export run through the action surface: the
 * host's conversion (ktw-docs Render or Diagram, a long method) answers at
 * once with its run and the display panel it reports each step into - drawn
 * where the export was asked - and the file is read once that panel completes
 * ok (DocumentExportRead); a failed or cancelled one is discarded.
 */
import type { Document } from './engines'

/** DocumentExportRun's answer (app/ktw-gui/app_export.go). */
export interface ExportRunResult {
  exportId?: string
  taskId?: string
  panel?: string
  document?: Document
  error?: string
}

/** What an export run shows: the run's panel (the file read when it ends), a
 *  document a conversion answered at once, or the error. */
export type ExportRunOutcome =
  | { panel: string; exportId: string }
  | { document: Document }
  | { error: string }

export function exportRunOutcome(r: ExportRunResult): ExportRunOutcome {
  if (r.error) return { error: r.error }
  if (r.panel && r.exportId) return { panel: r.panel, exportId: r.exportId }
  if (r.document) return { document: r.document }
  return { error: 'the export answered neither a run nor a document' }
}

/** The document an export hands the host: the content as shown, with the
 *  file it was read from - the host converts a copy, and the file's folder is
 *  where its relative images and diagrams resolve. A document read from no
 *  file goes as it is. */
export function withExportSource(doc: Document, source: Document['source'] | undefined): Document {
  return source?.path ? { ...doc, source: { path: source.path } } : doc
}

/** Once the export's panel ends: its file is read when it completed ok, else
 *  the export is discarded. */
export function afterExportPanel(status: string): 'read' | 'discard' {
  return status === 'ok' ? 'read' : 'discard'
}

/** An export's panel from the moment it is drawn: `ended` once the run's panel
 *  completes (read when ok, else discarded); `enter` / `leave` as the panel is
 *  mounted and goes away - closed, replaced by another export or unmounted
 *  with the document view. An export left before its run ended is discarded,
 *  so its job and its temp folder do not stay in the host until the GUI
 *  exits; one already read or discarded is left alone. The discard of a leave
 *  waits for `later` (the next tick): a panel mounted again at once - React's
 *  StrictMode runs an effect's cleanup and setup twice - keeps its export. */
export interface ExportPanelLife {
  ended(status: string): 'read' | 'discard' | 'settled'
  enter(): void
  leave(): void
}

export function exportPanelLife(
  exportId: string,
  discard: (exportId: string) => void,
  later: (f: () => void) => void = (f) => { setTimeout(f, 0) },
): ExportPanelLife {
  let settled = false
  let present = false
  return {
    ended(status) {
      if (settled) return 'settled'
      settled = true
      const next = afterExportPanel(status)
      if (next === 'discard') discard(exportId)
      return next
    },
    enter() { present = true },
    leave() {
      present = false
      later(() => {
        if (present || settled) return
        settled = true
        discard(exportId)
      })
    },
  }
}
